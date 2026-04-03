package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/dlc-01/replicast/internal/apperr"
	"github.com/dlc-01/replicast/internal/config"
	"github.com/dlc-01/replicast/internal/logger"
	"github.com/dlc-01/replicast/internal/port"
)

// authRepository — локальный интерфейс.
// Определяется на стороне потребителя, не реализации.
type authRepository interface {
	CreateUser(ctx context.Context, u port.User) error
	GetByGlobalID(ctx context.Context, globalID string) (*port.User, error)
	UsernameExists(ctx context.Context, username string) (bool, error)
	GetPasswordHash(ctx context.Context, globalID string) (string, error)
}

type Service struct {
	repo authRepository
	log  logger.Logger
	cfg  *config.Config
}

func NewService(repo authRepository, log logger.Logger, cfg *config.Config) *Service {
	return &Service{repo: repo, log: log, cfg: cfg}
}

type RegisterResult struct {
	Token string
	User  *port.User
}

func (s *Service) Register(ctx context.Context, username, password string) (*RegisterResult, error) {
	exists, err := s.repo.UsernameExists(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("auth.Register check username: %w", err)
	}
	if exists {
		return nil, apperr.ErrUserExists
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("auth.Register bcrypt: %w", err)
	}

	u := port.User{
		GlobalID:      fmt.Sprintf("%s@%s", username, s.cfg.NodeName),
		LocalUsername: username,
		HomeNode:      s.cfg.NodeName,
		PasswordHash:  string(hash),
	}
	if err := s.repo.CreateUser(ctx, u); err != nil {
		return nil, fmt.Errorf("auth.Register create: %w", err)
	}

	created, err := s.repo.GetByGlobalID(ctx, u.GlobalID)
	if err != nil {
		return nil, fmt.Errorf("auth.Register get created: %w", err)
	}

	token, err := s.makeToken(created.ID, u.GlobalID)
	if err != nil {
		return nil, err
	}

	s.log.Info("user registered", "global_id", u.GlobalID)
	return &RegisterResult{Token: token, User: created}, nil
}

func (s *Service) Login(ctx context.Context, username, password string) (string, error) {
	globalID := fmt.Sprintf("%s@%s", username, s.cfg.NodeName)

	hash, err := s.repo.GetPasswordHash(ctx, globalID)
	if err != nil {
		return "", apperr.ErrInvalidPassword
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return "", apperr.ErrInvalidPassword
	}

	user, err := s.repo.GetByGlobalID(ctx, globalID)
	if err != nil {
		return "", apperr.ErrInvalidPassword
	}

	token, err := s.makeToken(user.ID, globalID)
	if err != nil {
		return "", err
	}

	s.log.Info("user logged in", "global_id", globalID)
	return token, nil
}

// makeToken выпускает JWT с полями которые ожидает middleware:
// sub (UUID), global_id (alice@node-a), exp, iat.
func (s *Service) makeToken(userID, globalID string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":       userID,
		"global_id": globalID,
		"exp":       time.Now().Add(24 * time.Hour).Unix(),
		"iat":       time.Now().Unix(),
	})
	signed, err := token.SignedString([]byte(s.cfg.JWTSecret))
	if err != nil {
		return "", fmt.Errorf("auth: sign token: %w", err)
	}
	return signed, nil
}

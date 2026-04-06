package dms

import (
	"context"
	"fmt"
	"strings"

	"github.com/oklog/ulid/v2"

	"github.com/dlc-01/replicast/internal/apperr"
	"github.com/dlc-01/replicast/internal/config"
	"github.com/dlc-01/replicast/internal/logger"
	"github.com/dlc-01/replicast/internal/port"
)

type dmRepo interface {
	GetOrCreateConversation(ctx context.Context, a, b, sessionKeyA, sessionKeyB string) (*port.Conversation, error)
	GetConversation(ctx context.Context, id string) (*port.Conversation, error)
	ListConversations(ctx context.Context, userGlobalID string) ([]port.Conversation, error)
	CreateMessage(ctx context.Context, m port.Message) error
	GetMessages(ctx context.Context, conversationID string, limit int) ([]port.Message, error)
	GetUUIDByGlobalID(ctx context.Context, globalID string) (string, error)
	GetNodeByGlobalID(ctx context.Context, globalID string) (string, error)
}

type fedEnqueuer interface {
	EnqueueEvent(ctx context.Context, e port.OutboxEvent) error
}

type Service struct {
	repo    dmRepo
	fedRepo fedEnqueuer
	log     logger.Logger
	cfg     *config.Config
}

func NewService(repo dmRepo, fedRepo fedEnqueuer, log logger.Logger, cfg *config.Config) *Service {
	return &Service{repo: repo, fedRepo: fedRepo, log: log, cfg: cfg}
}

func (s *Service) StartConversation(ctx context.Context, senderGlobalID, recipientGlobalID, sessionKeyForMe, sessionKeyForThem string) (*port.Conversation, error) {
	if senderGlobalID == recipientGlobalID {
		return nil, apperr.BadRequest("self_message", "cannot message yourself")
	}
	conv, err := s.repo.GetOrCreateConversation(ctx, senderGlobalID, recipientGlobalID, sessionKeyForMe, sessionKeyForThem)
	if err != nil {
		return nil, fmt.Errorf("dms.StartConversation: %w", err)
	}
	return conv, nil
}

func (s *Service) ListConversations(ctx context.Context, userGlobalID string) ([]port.Conversation, error) {
	return s.repo.ListConversations(ctx, userGlobalID)
}

func (s *Service) SendMessage(ctx context.Context, senderGlobalID, conversationID, content string) (*port.Message, error) {
	conv, err := s.repo.GetConversation(ctx, conversationID)
	if err != nil {
		return nil, err
	}

	// Проверяем что отправитель участник диалога
	if conv.ParticipantA != senderGlobalID && conv.ParticipantB != senderGlobalID {
		return nil, apperr.Forbidden("not_participant", "not a participant of this conversation")
	}

	senderUUID, err := s.repo.GetUUIDByGlobalID(ctx, senderGlobalID)
	if err != nil {
		return nil, fmt.Errorf("dms.SendMessage resolve sender: %w", err)
	}

	globalID := fmt.Sprintf("msg:%s:%s", senderGlobalID, ulid.Make().String())
	m := port.Message{
		GlobalID:       globalID,
		ConversationID: conversationID,
		SenderID:       senderUUID,
		Content:        content,
	}

	if err := s.repo.CreateMessage(ctx, m); err != nil {
		return nil, fmt.Errorf("dms.SendMessage: %w", err)
	}

	// Определяем получателя
	recipientGlobalID := conv.ParticipantA
	if conv.ParticipantA == senderGlobalID {
		recipientGlobalID = conv.ParticipantB
	}

	// Если получатель на другом узле — доставляем через federation
	recipientNode := nodeFromGlobalID(recipientGlobalID)
	if recipientNode != s.cfg.NodeName {
		_ = s.fedRepo.EnqueueEvent(ctx, port.OutboxEvent{
			TargetNode: recipientNode,
			EventType:  "message.sent",
			Payload: map[string]any{
				"global_id":           globalID,
				"conversation_id":     conversationID,
				"sender_global_id":    senderGlobalID,
				"recipient_global_id": recipientGlobalID,
				"content":             content,
				"participant_a":       conv.ParticipantA,
				"participant_b":       conv.ParticipantB,
			},
		})
	}

	s.log.Info("message sent", "global_id", globalID, "conversation", conversationID)

	// Возвращаем сообщение с данными отправителя
	msgs, _ := s.repo.GetMessages(ctx, conversationID, 1)
	if len(msgs) > 0 {
		return &msgs[0], nil
	}
	m.SenderGlobalID = senderGlobalID
	return &m, nil
}

func (s *Service) GetMessages(ctx context.Context, userGlobalID, conversationID string, limit int) ([]port.Message, error) {
	conv, err := s.repo.GetConversation(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	if conv.ParticipantA != userGlobalID && conv.ParticipantB != userGlobalID {
		return nil, apperr.Forbidden("not_participant", "not a participant")
	}
	return s.repo.GetMessages(ctx, conversationID, limit)
}

func nodeFromGlobalID(globalID string) string {
	if i := strings.LastIndex(globalID, "@"); i != -1 {
		return globalID[i+1:]
	}
	return ""
}

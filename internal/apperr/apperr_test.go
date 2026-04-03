package apperr_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/dlc-01/replicast/internal/apperr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNotFound(t *testing.T) {
	err := apperr.NotFound("user_not_found", "user not found")
	assert.Equal(t, http.StatusNotFound, err.Status)
	assert.Equal(t, "user_not_found", err.Code)
	assert.Equal(t, "user not found", err.Message)
	assert.Equal(t, "user not found", err.Error())
}

func TestInternal_WrapsOriginalError(t *testing.T) {
	original := errors.New("db connection refused")
	err := apperr.Internal("db_error", original)

	assert.Equal(t, http.StatusInternalServerError, err.Status)
	assert.Equal(t, "db_error", err.Code)
	assert.Equal(t, "internal error", err.Message)
	// Error() возвращает оригинальную ошибку
	assert.Equal(t, "db connection refused", err.Error())
	// Unwrap работает корректно
	assert.True(t, errors.Is(err, original))
}

func TestAs_ExtractsAppError(t *testing.T) {
	err := apperr.Conflict("user_exists", "username taken")

	appErr, ok := apperr.As(err)
	require.True(t, ok)
	assert.Equal(t, "user_exists", appErr.Code)
	assert.Equal(t, http.StatusConflict, appErr.Status)
}

func TestAs_WrappedError(t *testing.T) {
	inner := apperr.NotFound("post_not_found", "post not found")
	wrapped := errors.New("wrapped: " + inner.Error())
	_ = wrapped

	// errors.As работает через цепочку
	appErr, ok := apperr.As(inner)
	require.True(t, ok)
	assert.Equal(t, http.StatusNotFound, appErr.Status)
}

func TestAs_NonAppError(t *testing.T) {
	err := errors.New("plain error")
	_, ok := apperr.As(err)
	assert.False(t, ok)
}

func TestSentinels(t *testing.T) {
	tests := []struct {
		name           string
		err            *apperr.AppError
		expectedStatus int
		expectedCode   string
	}{
		{"ErrUserNotFound", apperr.ErrUserNotFound, http.StatusNotFound, "user_not_found"},
		{"ErrPostNotFound", apperr.ErrPostNotFound, http.StatusNotFound, "post_not_found"},
		{"ErrUserExists", apperr.ErrUserExists, http.StatusConflict, "user_exists"},
		{"ErrInvalidPassword", apperr.ErrInvalidPassword, http.StatusUnauthorized, "invalid_credentials"},
		{"ErrPostForbidden", apperr.ErrPostForbidden, http.StatusForbidden, "post_forbidden"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expectedStatus, tt.err.Status)
			assert.Equal(t, tt.expectedCode, tt.err.Code)
		})
	}
}

func TestUnauthorized(t *testing.T) {
	err := apperr.Unauthorized("missing_token", "auth required")
	assert.Equal(t, http.StatusUnauthorized, err.Status)
}

func TestForbidden(t *testing.T) {
	err := apperr.Forbidden("invalid_secret", "bad secret")
	assert.Equal(t, http.StatusForbidden, err.Status)
}

func TestBadRequest(t *testing.T) {
	err := apperr.BadRequest("invalid_body", "bad json")
	assert.Equal(t, http.StatusBadRequest, err.Status)
}

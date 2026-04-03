package respond_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dlc-01/replicast/internal/apperr"
	"github.com/dlc-01/replicast/internal/respond"
)

func TestJSON_SetsContentType(t *testing.T) {
	w := httptest.NewRecorder()
	respond.JSON(w, http.StatusOK, map[string]string{"key": "value"})

	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

func TestJSON_SetsStatus(t *testing.T) {
	tests := []int{
		http.StatusOK, http.StatusCreated, http.StatusNoContent,
		http.StatusBadRequest, http.StatusNotFound,
	}
	for _, status := range tests {
		w := httptest.NewRecorder()
		respond.JSON(w, status, nil)
		if w.Code != status {
			t.Errorf("status = %d, want %d", w.Code, status)
		}
	}
}

func TestJSON_EncodesBody(t *testing.T) {
	w := httptest.NewRecorder()
	respond.JSON(w, http.StatusOK, map[string]string{"hello": "world"})

	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["hello"] != "world" {
		t.Errorf("body[hello] = %q, want world", body["hello"])
	}
}

func TestError_AppError_UsesStatus(t *testing.T) {
	tests := []struct {
		name       string
		err        *apperr.AppError
		wantStatus int
		wantCode   string
	}{
		{"not found", apperr.ErrUserNotFound, http.StatusNotFound, "user_not_found"},
		{"conflict", apperr.ErrUserExists, http.StatusConflict, "user_exists"},
		{"unauthorized", apperr.ErrInvalidPassword, http.StatusUnauthorized, "invalid_credentials"},
		{"forbidden", apperr.ErrPostForbidden, http.StatusForbidden, "post_forbidden"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			respond.Error(w, r, tt.err)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
			var body map[string]string
			json.NewDecoder(w.Body).Decode(&body)
			if body["code"] != tt.wantCode {
				t.Errorf("code = %q, want %q", body["code"], tt.wantCode)
			}
		})
	}
}

func TestError_UnknownError_Returns500(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	respond.Error(w, r, errors.New("something unexpected"))

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
	var body map[string]string
	json.NewDecoder(w.Body).Decode(&body)
	if body["code"] != "internal_error" {
		t.Errorf("code = %q, want internal_error", body["code"])
	}
}

func TestError_InternalAppError_HidesDetails(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	respond.Error(w, r, apperr.Internal("db_error", errors.New("connection refused")))

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
	var body map[string]string
	json.NewDecoder(w.Body).Decode(&body)
	// Детали внутренней ошибки клиенту не уходят
	if body["message"] != "internal error" {
		t.Errorf("message = %q, want 'internal error'", body["message"])
	}
	if body["code"] != "db_error" {
		t.Errorf("code = %q, want db_error", body["code"])
	}
}

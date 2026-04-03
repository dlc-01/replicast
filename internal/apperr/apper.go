package apperr

import (
	"errors"
	"net/http"
)

// AppError — доменная ошибка с HTTP статусом и машиночитаемым кодом.
// Все ошибки домена возвращаются через этот тип —
// хендлеры не знают про HTTP статусы, только про коды.
type AppError struct {
	Code    string // машиночитаемый код: "user_not_found"
	Message string // сообщение для клиента
	Status  int    // HTTP статус
	Err     error  // оригинальная ошибка (только для логов, клиенту не уходит)
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return e.Message
}

func (e *AppError) Unwrap() error { return e.Err }

// Конструкторы

func NotFound(code, msg string) *AppError {
	return &AppError{Code: code, Message: msg, Status: http.StatusNotFound}
}

func Conflict(code, msg string) *AppError {
	return &AppError{Code: code, Message: msg, Status: http.StatusConflict}
}

func Forbidden(code, msg string) *AppError {
	return &AppError{Code: code, Message: msg, Status: http.StatusForbidden}
}

func BadRequest(code, msg string) *AppError {
	return &AppError{Code: code, Message: msg, Status: http.StatusBadRequest}
}

func Unauthorized(code, msg string) *AppError {
	return &AppError{Code: code, Message: msg, Status: http.StatusUnauthorized}
}

func Internal(code string, err error) *AppError {
	return &AppError{
		Code:    code,
		Message: "internal error",
		Status:  http.StatusInternalServerError,
		Err:     err,
	}
}

// Sentinel ошибки домена
var (
	ErrUserNotFound    = NotFound("user_not_found", "user not found")
	ErrPostNotFound    = NotFound("post_not_found", "post not found")
	ErrUserExists      = Conflict("user_exists", "username already taken")
	ErrInvalidPassword = Unauthorized("invalid_credentials", "invalid username or password")
	ErrPostForbidden   = Forbidden("post_forbidden", "not your post")
)

// As извлекает AppError из цепочки ошибок.
func As(err error) (*AppError, bool) {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr, true
	}
	return nil, false
}

package ctxkey

// Key — тип для ключей контекста.
// Собственный тип предотвращает коллизии с ключами из других пакетов.
type Key string

const (
	UserGlobalID Key = "user_global_id"
	RequestID    Key = "request_id"
)

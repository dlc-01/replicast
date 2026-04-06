package signing

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

const (
	HeaderNodeName  = "X-Replicast-Node"
	HeaderTimestamp = "X-Replicast-Timestamp"
	HeaderSignature = "X-Replicast-Signature"
	MaxClockSkew    = 5 * time.Minute
)

// SignRequest добавляет HMAC-SHA256 подпись к исходящему запросу.
// Подпись = HMAC-SHA256(secret, "nodeName:timestamp:sha256(body)")
func SignRequest(req *http.Request, nodeName, secret string) error {
	var body []byte
	if req.Body != nil {
		var err error
		body, err = io.ReadAll(req.Body)
		if err != nil {
			return fmt.Errorf("signing: read body: %w", err)
		}
		req.Body = io.NopCloser(bytes.NewReader(body))
	}

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	message := nodeName + ":" + timestamp + ":" + SHA256Hex(body)
	sig := HMACSHA256(secret, message)

	req.Header.Set(HeaderNodeName, nodeName)
	req.Header.Set(HeaderTimestamp, timestamp)
	req.Header.Set(HeaderSignature, sig)
	return nil
}

// Verify проверяет HMAC подпись входящего запроса.
// Возвращает тело запроса (для повторного чтения) и ошибку если подпись неверна.
func Verify(r *http.Request, secret string) ([]byte, error) {
	nodeName := r.Header.Get(HeaderNodeName)
	timestampStr := r.Header.Get(HeaderTimestamp)
	sig := r.Header.Get(HeaderSignature)

	if nodeName == "" || timestampStr == "" || sig == "" {
		return nil, nil // заголовки отсутствуют — пропускаем проверку
	}

	ts, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("signing: invalid timestamp")
	}
	if time.Since(time.Unix(ts, 0)).Abs() > MaxClockSkew {
		return nil, fmt.Errorf("signing: timestamp too old or too new")
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("signing: read body: %w", err)
	}

	message := nodeName + ":" + timestampStr + ":" + SHA256Hex(body)
	expected := HMACSHA256(secret, message)

	if !hmac.Equal([]byte(sig), []byte(expected)) {
		return nil, fmt.Errorf("signing: signature mismatch")
	}

	return body, nil
}

func HMACSHA256(secret, message string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

func SHA256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

const HeaderRSASignature = "X-Replicast-RSA-Signature"

// SignRequestWithRSA добавляет RSA подпись поверх HMAC.
// Вызывается после SignRequest — добавляет один дополнительный заголовок.
func SignRequestWithRSA(req *http.Request, signer RSASigner) error {
	// Подписываем: метод + path + timestamp + HMAC подпись
	// Это связывает RSA подпись с конкретным запросом
	msg := req.Method + req.URL.Path +
		req.Header.Get(HeaderTimestamp) +
		req.Header.Get(HeaderSignature)

	sig, err := signer.Sign([]byte(msg))
	if err != nil {
		return fmt.Errorf("signing: rsa sign: %w", err)
	}
	req.Header.Set(HeaderRSASignature, sig)
	return nil
}

// VerifyRSA проверяет RSA подпись запроса.
// pubKeyPEM — публичный ключ отправителя из таблицы nodes.
// Если заголовок RSA подписи отсутствует — пропускаем (обратная совместимость).
func VerifyRSA(r *http.Request, pubKeyPEM string) error {
	rsaSig := r.Header.Get(HeaderRSASignature)
	if rsaSig == "" {
		return nil // нет RSA подписи — пропускаем
	}

	msg := r.Method + r.URL.Path +
		r.Header.Get(HeaderTimestamp) +
		r.Header.Get(HeaderSignature)

	return VerifyNodeSignatureFunc(pubKeyPEM, []byte(msg), rsaSig)
}

// RSASigner — интерфейс для подписи (реализуется NodeKeyPair из e2e пакета).
type RSASigner interface {
	Sign(data []byte) (string, error)
}

// VerifyNodeSignatureFunc — функция верификации (инжектируется из e2e пакета).
var VerifyNodeSignatureFunc func(pubKeyPEM string, data []byte, sigBase64 string) error

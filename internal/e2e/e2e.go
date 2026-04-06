// Package e2e реализует end-to-end шифрование для личных сообщений.
//
// Схема:
//
//  1. При регистрации сервер генерирует RSA-2048 пару ключей.
//     Публичный ключ хранится в БД и отдаётся через API.
//     Приватный ключ возвращается клиенту ОДИН РАЗ и больше не хранится.
//
//  2. При создании диалога клиент:
//     a. Генерирует случайный AES-256 session key
//     b. Шифрует его публичным ключом получателя → session_key_b
//     c. Шифрует его своим публичным ключом → session_key_a
//     d. Передаёт оба на сервер
//
//  3. Все сообщения в диалоге шифруются AES-256-GCM session key'ем на клиенте.
//     Сервер хранит и доставляет только зашифрованный blob.
//
// Сервер НИКОГДА не видит:
//   - Приватные ключи пользователей
//   - Session keys в открытом виде
//   - Содержимое сообщений
package e2e

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
)

const rsaKeyBits = 2048

// KeyPair содержит RSA пару ключей в PEM формате.
type KeyPair struct {
	PublicKey  string // PEM — хранится на сервере, публичный
	PrivateKey string // PEM — отдаётся клиенту один раз, НЕ хранится
}

// GenerateKeyPair генерирует RSA-2048 пару ключей.
// Вызывается при регистрации пользователя.
func GenerateKeyPair() (*KeyPair, error) {
	priv, err := rsa.GenerateKey(rand.Reader, rsaKeyBits)
	if err != nil {
		return nil, fmt.Errorf("e2e: generate key: %w", err)
	}

	privPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(priv),
	})

	pubDER, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("e2e: marshal public key: %w", err)
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubDER,
	})

	return &KeyPair{
		PublicKey:  string(pubPEM),
		PrivateKey: string(privPEM),
	}, nil
}

// ParsePublicKey разбирает PEM публичный ключ.
func ParsePublicKey(pemStr string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("e2e: invalid PEM")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("e2e: parse public key: %w", err)
	}
	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("e2e: not an RSA key")
	}
	return rsaPub, nil
}

// ParsePrivateKey разбирает PEM приватный ключ.
// Поддерживает PKCS#1 (openssl genrsa) и PKCS#8 форматы.
func ParsePrivateKey(pemStr string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("e2e: invalid PEM")
	}

	// Пробуем PKCS#1 (RSA PRIVATE KEY) — генерируется openssl genrsa
	if priv, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return priv, nil
	}

	// Пробуем PKCS#8 (PRIVATE KEY) — генерируется Go стандартной библиотекой
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("e2e: parse private key: %w", err)
	}
	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("e2e: not an RSA private key")
	}
	return rsaKey, nil
}

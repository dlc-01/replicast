package e2e

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
)

// NodeKeyPair — RSA пара ключей узла для подписи межузловых запросов.
type NodeKeyPair struct {
	PrivateKey   *rsa.PrivateKey
	PublicKeyPEM string
}

// GenerateNodeKeyPair генерирует RSA-2048 пару ключей для узла.
// Вызывается один раз при старте если ключи не сохранены.
func GenerateNodeKeyPair() (*NodeKeyPair, error) {
	priv, err := rsa.GenerateKey(rand.Reader, rsaKeyBits)
	if err != nil {
		return nil, fmt.Errorf("e2e: generate node key: %w", err)
	}

	pubDER, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("e2e: marshal node public key: %w", err)
	}
	pubPEM := string(pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubDER,
	}))

	return &NodeKeyPair{
		PrivateKey:   priv,
		PublicKeyPEM: pubPEM,
	}, nil
}

// Sign подписывает данные приватным ключом узла.
// Возвращает base64-encoded подпись.
func (kp *NodeKeyPair) Sign(data []byte) (string, error) {
	hash := sha256.Sum256(data)
	sig, err := rsa.SignPKCS1v15(rand.Reader, kp.PrivateKey, crypto.SHA256, hash[:])
	if err != nil {
		return "", fmt.Errorf("e2e: sign: %w", err)
	}
	return base64.StdEncoding.EncodeToString(sig), nil
}

// VerifyNodeSignature проверяет подпись публичным ключом узла.
func VerifyNodeSignature(pubKeyPEM string, data []byte, sigBase64 string) error {
	pub, err := ParsePublicKey(pubKeyPEM)
	if err != nil {
		return fmt.Errorf("e2e: parse pub key: %w", err)
	}

	sig, err := base64.StdEncoding.DecodeString(sigBase64)
	if err != nil {
		return fmt.Errorf("e2e: decode signature: %w", err)
	}

	hash := sha256.Sum256(data)
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, hash[:], sig); err != nil {
		return fmt.Errorf("e2e: invalid signature: %w", err)
	}
	return nil
}

// NodeKeyPairFromPrivate создаёт NodeKeyPair из существующего приватного ключа.
func NodeKeyPairFromPrivate(priv *rsa.PrivateKey) (*NodeKeyPair, error) {
	pubDER, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("e2e: marshal public key: %w", err)
	}
	pubPEM := string(pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubDER,
	}))
	return &NodeKeyPair{
		PrivateKey:   priv,
		PublicKeyPEM: pubPEM,
	}, nil
}

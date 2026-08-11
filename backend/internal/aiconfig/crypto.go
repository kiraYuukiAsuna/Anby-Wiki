package aiconfig

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
)

const credentialAAD = "anby-wiki/ai-runtime/v1"

type credentialCipher struct {
	aead cipher.AEAD
}

func newCredentialCipher(encodedKey string) (*credentialCipher, error) {
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encodedKey))
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("%w: AI_CONFIG_MASTER_KEY 必须是 32 字节密钥的 base64", ErrMasterKeyInvalid)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMasterKeyInvalid, err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMasterKeyInvalid, err)
	}
	return &credentialCipher{aead: aead}, nil
}

func (c *credentialCipher) encrypt(plaintext string) (string, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("aiconfig: 生成加密 nonce 失败: %w", err)
	}
	sealed := c.aead.Seal(nil, nonce, []byte(plaintext), []byte(credentialAAD))
	payload := append(nonce, sealed...)
	return base64.StdEncoding.EncodeToString(payload), nil
}

func (c *credentialCipher) decrypt(encoded string) (string, error) {
	payload, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(payload) <= c.aead.NonceSize() {
		return "", fmt.Errorf("aiconfig: API Key 密文损坏")
	}
	nonce, ciphertext := payload[:c.aead.NonceSize()], payload[c.aead.NonceSize():]
	plaintext, err := c.aead.Open(nil, nonce, ciphertext, []byte(credentialAAD))
	if err != nil {
		return "", fmt.Errorf("aiconfig: API Key 解密失败: %w", err)
	}
	return string(plaintext), nil
}

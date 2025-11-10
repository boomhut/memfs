package memfs

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"io"
)

// encryptor handles encryption and decryption of file data at rest
type encryptor struct {
	key    []byte
	gcm    cipher.AEAD
	enable bool
}

// newEncryptor creates a new encryptor with the given key
// The key can be of any length and will be hashed to 32 bytes for AES-256
func newEncryptor(key []byte) (*encryptor, error) {
	if len(key) == 0 {
		return &encryptor{enable: false}, nil
	}

	// Hash the key to ensure it's the correct length for AES-256 (32 bytes)
	hash := sha256.Sum256(key)

	block, err := aes.NewCipher(hash[:])
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	return &encryptor{
		key:    hash[:],
		gcm:    gcm,
		enable: true,
	}, nil
}

// encrypt encrypts the plaintext data using AES-GCM
// Returns the encrypted data with the nonce prepended
func (e *encryptor) encrypt(plaintext []byte) ([]byte, error) {
	if !e.enable || len(plaintext) == 0 {
		return plaintext, nil
	}

	// Generate a random nonce
	nonce := make([]byte, e.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	// Encrypt the data
	// The nonce is prepended to the ciphertext
	ciphertext := e.gcm.Seal(nonce, nonce, plaintext, nil)

	return ciphertext, nil
}

// decrypt decrypts the ciphertext data using AES-GCM
// Expects the nonce to be prepended to the ciphertext
func (e *encryptor) decrypt(ciphertext []byte) ([]byte, error) {
	if !e.enable || len(ciphertext) == 0 {
		return ciphertext, nil
	}

	nonceSize := e.gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}

	// Extract the nonce and ciphertext
	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]

	// Decrypt the data
	plaintext, err := e.gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}

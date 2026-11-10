package security

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	argonMemory      uint32 = 64 * 1024
	argonIterations  uint32 = 3
	argonParallelism uint8  = 2
	saltLength              = 16
	keyLength               = 32
)

var ErrInvalidPasswordHash = errors.New("invalid password hash")

// HashPassword создаёт Argon2id-хеш с уникальной случайной солью.
func HashPassword(password string) (string, error) {
	salt := make([]byte, saltLength)

	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}

	hash := argon2.IDKey(
		[]byte(password),
		salt,
		argonIterations,
		argonMemory,
		argonParallelism,
		keyLength,
	)

	encodedSalt := base64.RawStdEncoding.EncodeToString(salt)
	encodedHash := base64.RawStdEncoding.EncodeToString(hash)

	return fmt.Sprintf(
		"$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		argonMemory,
		argonIterations,
		argonParallelism,
		encodedSalt,
		encodedHash,
	), nil
}

// VerifyPassword сравнивает пароль с сохранённым Argon2id-хешем.
func VerifyPassword(password, encodedHash string) (bool, error) {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false, ErrInvalidPasswordHash
	}

	params := strings.Split(parts[3], ",")
	if len(params) != 3 {
		return false, ErrInvalidPasswordHash
	}

	var memory, iterations uint64
	var parallelism uint64

	for _, param := range params {
		key, value, ok := strings.Cut(param, "=")
		if !ok {
			return false, ErrInvalidPasswordHash
		}

		number, err := strconv.ParseUint(value, 10, 32)
		if err != nil {
			return false, ErrInvalidPasswordHash
		}

		switch key {
		case "m":
			memory = number
		case "t":
			iterations = number
		case "p":
			parallelism = number
		default:
			return false, ErrInvalidPasswordHash
		}
	}

	// Не принимаем произвольные параметры из хранимой строки:
	// это предотвращает неконтролируемые затраты памяти и CPU.
	if memory != uint64(argonMemory) ||
		iterations != uint64(argonIterations) ||
		parallelism != uint64(argonParallelism) {
		return false, ErrInvalidPasswordHash
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) != saltLength {
		return false, ErrInvalidPasswordHash
	}

	expectedHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(expectedHash) != keyLength {
		return false, ErrInvalidPasswordHash
	}

	actualHash := argon2.IDKey(
		[]byte(password),
		salt,
		argonIterations,
		argonMemory,
		argonParallelism,
		keyLength,
	)

	return subtle.ConstantTimeCompare(actualHash, expectedHash) == 1, nil
}

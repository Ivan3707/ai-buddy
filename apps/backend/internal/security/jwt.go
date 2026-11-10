package security

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const jwtIssuer = "ai-buddy"

var (
	ErrInvalidToken = errors.New("invalid access token")
)

type JWTService struct {
	secret []byte
	ttl    time.Duration
}

func NewJWTService(secret string, ttl time.Duration) (*JWTService, error) {
	if len(secret) < 32 {
		return nil, errors.New("JWT secret must contain at least 32 bytes")
	}

	if ttl <= 0 {
		return nil, errors.New("JWT TTL must be positive")
	}

	return &JWTService{
		secret: []byte(secret),
		ttl:    ttl,
	}, nil
}

func (s *JWTService) GenerateAccessToken(userID uuid.UUID) (string, error) {
	now := time.Now()

	claims := jwt.RegisteredClaims{
		Issuer:    jwtIssuer,
		Subject:   userID.String(),
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(s.ttl)),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	signed, err := token.SignedString(s.secret)
	if err != nil {
		return "", fmt.Errorf("sign access token: %w", err)
	}

	return signed, nil
}

func (s *JWTService) ValidateAccessToken(tokenString string) (uuid.UUID, error) {
	claims := &jwt.RegisteredClaims{}

	token, err := jwt.ParseWithClaims(
		tokenString,
		claims,
		func(token *jwt.Token) (any, error) {
			if token.Method != jwt.SigningMethodHS256 {
				return nil, ErrInvalidToken
			}
			return s.secret, nil
		},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(jwtIssuer),
		jwt.WithExpirationRequired(),
	)
	if err != nil || !token.Valid {
		return uuid.Nil, ErrInvalidToken
	}

	userID, err := uuid.Parse(claims.Subject)
	if err != nil || userID == uuid.Nil {
		return uuid.Nil, ErrInvalidToken
	}

	return userID, nil
}

func (s *JWTService) TTL() time.Duration {
	return s.ttl
}

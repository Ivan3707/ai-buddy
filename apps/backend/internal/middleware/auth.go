package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
	"github.com/nvkmaksim/ai-buddy/backend/internal/security"
)

type userRepository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*model.User, error)
}

type contextKey string

const userContextKey contextKey = "authenticated_user"

func Authenticate(
	jwtService *security.JWTService,
	users userRepository,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authorization := r.Header.Get("Authorization")
			parts := strings.Fields(authorization)

			if len(parts) != 2 ||
				!strings.EqualFold(parts[0], "Bearer") ||
				len(parts[1]) > 4096 {
				unauthorized(w)
				return
			}

			userID, err := jwtService.ValidateAccessToken(parts[1])
			if err != nil {
				unauthorized(w)
				return
			}

			user, err := users.GetByID(r.Context(), userID)
			if errors.Is(err, repository.ErrNotFound) {
				unauthorized(w)
				return
			}
			if err != nil {
				http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
				return
			}
			if user == nil {
				unauthorized(w)
				return
			}

			ctx := context.WithValue(r.Context(), userContextKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func RequireRole(roles ...model.UserRole) func(http.Handler) http.Handler {
	allowed := make(map[model.UserRole]struct{}, len(roles))
	for _, role := range roles {
		allowed[role] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := CurrentUser(r.Context())
			if !ok {
				unauthorized(w)
				return
			}

			if _, ok := allowed[user.Role]; !ok {
				http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func CurrentUser(ctx context.Context) (*model.User, bool) {
	user, ok := ctx.Value(userContextKey).(*model.User)
	return user, ok
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
}

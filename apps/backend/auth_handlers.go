package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/middleware"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
	"github.com/nvkmaksim/ai-buddy/backend/internal/security"
	"github.com/nvkmaksim/ai-buddy/backend/internal/service"
)

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type publicUser struct {
	ID         string            `json:"id"`
	Email      string            `json:"email"`
	Role       model.UserRole    `json:"role"`
	Profession *model.Profession `json:"profession"`
	CreatedAt  string            `json:"created_at"`
}

type loginResponse struct {
	AccessToken string     `json:"access_token"`
	TokenType   string     `json:"token_type"`
	ExpiresIn   int64      `json:"expires_in"`
	User        publicUser `json:"user"`
}

type authService interface {
	Login(ctx context.Context, email, password string) (*service.LoginResult, error)
}

type handlerUserRepository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*model.User, error)
}

type handlerDependencies struct {
	auth     authService
	users    handlerUserRepository
	jwt      *security.JWTService
	interns  internCreator
	progress progressReviewer
}

type createInternRequest struct {
	Email      string           `json:"email"`
	Password   string           `json:"password"`
	Profession model.Profession `json:"profession"`
}

type internCreator interface {
	CreateIntern(
		ctx context.Context,
		email string,
		passwordHash string,
		profession model.Profession,
		buddyID uuid.UUID,
	) (*model.User, error)
}

type progressReviewer interface {
	ReviewItem(
		ctx context.Context,
		buddyID uuid.UUID,
		internID uuid.UUID,
		itemID uuid.UUID,
		approved bool,
	) error
}

type reviewItemRequest struct {
	Approved *bool `json:"approved"`
}

func toPublicUser(user *model.User) publicUser {
	return publicUser{
		ID:         user.ID.String(),
		Email:      user.Email,
		Role:       user.Role,
		Profession: user.Profession,
		CreatedAt:  user.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func (d handlerDependencies) loginHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 4096)

		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()

		var body loginRequest
		if err := decoder.Decode(&body); err != nil {
			send(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		if decoder.Decode(&struct{}{}) != io.EOF {
			send(w, http.StatusBadRequest, map[string]string{"error": "expected one JSON object"})
			return
		}

		body.Email = strings.TrimSpace(body.Email)
		if body.Email == "" || len(body.Email) > 255 ||
			body.Password == "" || len(body.Password) > 256 {
			send(w, http.StatusBadRequest, map[string]string{"error": "invalid email or password"})
			return
		}

		result, err := d.auth.Login(r.Context(), body.Email, body.Password)
		if errors.Is(err, service.ErrInvalidCredentials) {
			send(w, http.StatusUnauthorized, map[string]string{"error": "invalid email or password"})
			return
		}
		if err != nil {
			send(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			return
		}

		send(w, http.StatusOK, loginResponse{
			AccessToken: result.AccessToken,
			TokenType:   "Bearer",
			ExpiresIn:   result.ExpiresIn,
			User:        toPublicUser(result.User),
		})
	}
}

func (d handlerDependencies) meHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := middleware.CurrentUser(r.Context())
		if !ok {
			send(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}

		send(w, http.StatusOK, toPublicUser(user))
	}
}

func (d handlerDependencies) createInternHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 4096)

		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()

		var body createInternRequest
		if err := decoder.Decode(&body); err != nil {
			send(w, http.StatusBadRequest, map[string]string{
				"error": "invalid JSON",
			})
			return
		}

		if decoder.Decode(&struct{}{}) != io.EOF {
			send(w, http.StatusBadRequest, map[string]string{
				"error": "expected one JSON object",
			})
			return
		}

		body.Email = strings.TrimSpace(body.Email)

		parsedEmail, err := mail.ParseAddress(body.Email)
		if err != nil || parsedEmail.Address != body.Email ||
			len(body.Email) > 255 {
			send(w, http.StatusBadRequest, map[string]string{
				"error": "invalid email",
			})
			return
		}

		if len(body.Password) > 256 ||
			utf8.RuneCountInString(body.Password) < 12 {
			send(w, http.StatusBadRequest, map[string]string{
				"error": "password must contain at least 12 characters",
			})
			return
		}

		passwordHash, err := security.HashPassword(body.Password)
		if err != nil {
			send(w, http.StatusInternalServerError, map[string]string{
				"error": "internal server error",
			})
			return
		}

		currentUser, ok := middleware.CurrentUser(r.Context())
		if !ok || currentUser.Role != model.RoleBuddy {
			send(w, http.StatusUnauthorized, map[string]string{
				"error": "unauthorized",
			})
			return
		}

		user, err := d.interns.CreateIntern(
			r.Context(),
			body.Email,
			passwordHash,
			body.Profession,
			currentUser.ID,
		)

		switch {
		case errors.Is(err, repository.ErrEmailAlreadyExists):
			send(w, http.StatusConflict, map[string]string{
				"error": "email already exists",
			})
			return

		case errors.Is(err, service.ErrInvalidProfession):
			send(w, http.StatusBadRequest, map[string]string{
				"error": "invalid profession",
			})
			return

		case errors.Is(err, service.ErrUserNotBuddy):
			send(w, http.StatusForbidden, map[string]string{
				"error": "user is not a buddy",
			})
			return

		case err != nil:
			send(w, http.StatusInternalServerError, map[string]string{
				"error": "internal server error",
			})
			return
		}

		send(w, http.StatusCreated, toPublicUser(user))
	}
}
func (d handlerDependencies) reviewItemHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		currentUser, ok := middleware.CurrentUser(r.Context())
		if !ok || currentUser.Role != model.RoleBuddy {
			send(w, http.StatusUnauthorized, map[string]string{
				"error": "unauthorized",
			})
			return
		}

		internID, err := uuid.Parse(r.PathValue("internID"))
		if err != nil {
			send(w, http.StatusBadRequest, map[string]string{
				"error": "invalid intern ID",
			})
			return
		}

		itemID, err := uuid.Parse(r.PathValue("itemID"))
		if err != nil {
			send(w, http.StatusBadRequest, map[string]string{
				"error": "invalid checklist item ID",
			})
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, 4096)

		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()

		var body reviewItemRequest
		if err := decoder.Decode(&body); err != nil {
			send(w, http.StatusBadRequest, map[string]string{
				"error": "invalid JSON",
			})
			return
		}

		if decoder.Decode(&struct{}{}) != io.EOF {
			send(w, http.StatusBadRequest, map[string]string{
				"error": "expected one JSON object",
			})
			return
		}

		if body.Approved == nil {
			send(w, http.StatusBadRequest, map[string]string{
				"error": "approved is required",
			})
			return
		}

		err = d.progress.ReviewItem(
			r.Context(),
			currentUser.ID,
			internID,
			itemID,
			*body.Approved,
		)

		switch {
		case errors.Is(err, repository.ErrNotFound):
			send(w, http.StatusNotFound, map[string]string{
				"error": "user or checklist item not found",
			})
			return

		case errors.Is(err, service.ErrUserNotBuddy),
			errors.Is(err, service.ErrInternNotAssigned),
			errors.Is(err, service.ErrCannotReviewSelf):
			send(w, http.StatusForbidden, map[string]string{
				"error": "forbidden",
			})
			return

		case errors.Is(err, service.ErrUserNotIntern):
			send(w, http.StatusNotFound, map[string]string{
				"error": "intern not found",
			})
			return

		case errors.Is(err, service.ErrReviewNotPending):
			send(w, http.StatusConflict, map[string]string{
				"error": "checklist item is not awaiting review",
			})
			return

		case err != nil:
			send(w, http.StatusInternalServerError, map[string]string{
				"error": "internal server error",
			})
			return
		}

		send(w, http.StatusOK, map[string]string{
			"status": "ok",
		})
	}
}

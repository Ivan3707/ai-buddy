package handler

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
	ID               string            `json:"id"`
	Email            string            `json:"email"`
	Role             model.UserRole    `json:"role"`
	Profession       *model.Profession `json:"profession"`
	CreatedAt        string            `json:"created_at"`
	FirstName        *string           `json:"first_name"`
	LastName         *string           `json:"last_name"`
	TelegramUsername *string           `json:"telegram_username"`
	Department       *string           `json:"department"`
	Team             *string           `json:"team"`
	Buddy            *buddyProfile     `json:"buddy,omitempty"`
	Teamlead         *buddyProfile     `json:"teamlead,omitempty"`
	AgeYears         *int              `json:"age_years"`
	TenureMonths     *int              `json:"tenure_months"`
}

type buddyProfile struct {
	ID               string  `json:"id"`
	FirstName        *string `json:"first_name"`
	LastName         *string `json:"last_name"`
	TelegramUsername *string `json:"telegram_username"`
}

type loginResponse struct {
	AccessToken  string     `json:"access_token"`
	TokenType    string     `json:"token_type"`
	ExpiresIn    int64      `json:"expires_in"`
	IsFirstLogin bool       `json:"is_first_login"`
	User         publicUser `json:"user"`
	RefreshToken string     `json:"refresh_token,omitempty"`
}

type createInternRequest struct {
	Email            string           `json:"email"`
	Password         string           `json:"password"`
	Profession       model.Profession `json:"profession"`
	FirstName        string           `json:"first_name"`
	LastName         string           `json:"last_name"`
	TelegramUsername string           `json:"telegram_username"`
	Department       string           `json:"department"`
	Team             string           `json:"team"`
	TeamleadID       string           `json:"teamlead_id"`
	BuddyID          string           `json:"buddy_id"`
	BirthDate        string           `json:"birth_date"`
	EmployedSince    string           `json:"employed_since"`
}

func toPublicUser(user *model.User) publicUser {
	profile := publicUser{
		ID:         user.ID.String(),
		Email:      user.Email,
		Role:       user.Role,
		Profession: user.Profession,
		CreatedAt:  user.CreatedAt.UTC().Format(time.RFC3339),
		FirstName:  user.FirstName, LastName: user.LastName, TelegramUsername: user.TelegramUsername,
		Department: user.Department, Team: user.Team,
	}
	if user.BirthDate != nil {
		age := fullMonths(*user.BirthDate, time.Now()) / 12
		profile.AgeYears = &age
	}
	if user.EmployedSince != nil {
		months := fullMonths(*user.EmployedSince, time.Now())
		profile.TenureMonths = &months
	}
	return profile
}

func fullMonths(from, to time.Time) int {
	months := (to.Year()-from.Year())*12 + int(to.Month()-from.Month())
	if to.Day() < from.Day() {
		months--
	}
	if months < 0 {
		return 0
	}
	return months
}

func (d Dependencies) publicProfile(ctx context.Context, user *model.User) (publicUser, error) {
	profile := toPublicUser(user)
	if user.BuddyID != nil {
		buddy, err := d.Users.GetByID(ctx, *user.BuddyID)
		if err != nil && !errors.Is(err, repository.ErrNotFound) {
			return profile, err
		}
		if err == nil && buddy != nil {
			profile.Buddy = &buddyProfile{ID: buddy.ID.String(), FirstName: buddy.FirstName, LastName: buddy.LastName, TelegramUsername: buddy.TelegramUsername}
		}
	}
	if user.TeamleadID != nil {
		lead, err := d.Users.GetByID(ctx, *user.TeamleadID)
		if err != nil && !errors.Is(err, repository.ErrNotFound) {
			return profile, err
		}
		if err == nil && lead != nil {
			profile.Teamlead = &buddyProfile{ID: lead.ID.String(), FirstName: lead.FirstName, LastName: lead.LastName, TelegramUsername: lead.TelegramUsername}
		}
	}
	return profile, nil
}

func (d Dependencies) loginHandler() http.HandlerFunc {
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

		result, err := d.Auth.Login(r.Context(), body.Email, body.Password)
		if errors.Is(err, service.ErrInvalidCredentials) {
			send(w, http.StatusUnauthorized, map[string]string{"error": "invalid email or password"})
			return
		}
		if err != nil {
			send(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			return
		}

		profile, err := d.publicProfile(r.Context(), result.User)
		if err != nil {
			send(w, 500, map[string]string{"error": "internal server error"})
			return
		}
		send(w, http.StatusOK, loginResponse{
			AccessToken:  result.AccessToken,
			TokenType:    "Bearer",
			ExpiresIn:    result.ExpiresIn,
			IsFirstLogin: result.IsFirstLogin,
			User:         profile,
			RefreshToken: result.RefreshToken,
		})
	}
}

func (d Dependencies) meHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := middleware.CurrentUser(r.Context())
		if !ok {
			send(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}

		profile, err := d.publicProfile(r.Context(), user)
		if err != nil {
			send(w, 500, map[string]string{"error": "internal server error"})
			return
		}
		send(w, http.StatusOK, profile)
	}
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

func (d Dependencies) refreshHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body refreshRequest
		if !decodeJSONBody(w, r, &body) {
			return
		}
		if d.Sessions == nil {
			send(w, 500, map[string]string{"error": "sessions unavailable"})
			return
		}
		access, refresh, ttl, err := d.Sessions.Refresh(r.Context(), body.RefreshToken)
		if errors.Is(err, service.ErrInvalidRefresh) {
			send(w, 401, map[string]string{"error": "invalid refresh token"})
			return
		}
		if err != nil {
			send(w, 500, map[string]string{"error": "internal server error"})
			return
		}
		send(w, 200, map[string]any{"access_token": access, "refresh_token": refresh, "token_type": "Bearer", "expires_in": ttl})
	}
}
func (d Dependencies) logoutHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body refreshRequest
		if !decodeJSONBody(w, r, &body) {
			return
		}
		user, _ := middleware.CurrentUser(r.Context())
		if d.Sessions == nil {
			send(w, 500, map[string]string{"error": "sessions unavailable"})
			return
		}
		if err := d.Sessions.Logout(r.Context(), user.ID, body.RefreshToken); err != nil {
			send(w, 400, map[string]string{"error": "invalid refresh token"})
			return
		}
		w.WriteHeader(204)
	}
}

func (d Dependencies) createInternHandler() http.HandlerFunc {
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

		body.Email = strings.ToLower(strings.TrimSpace(body.Email))
		buddyID, err := uuid.Parse(body.BuddyID)
		if err != nil {
			send(w, 400, map[string]string{"error": "invalid buddy_id"})
			return
		}
		birthDate, err := time.Parse("2006-01-02", body.BirthDate)
		if err != nil || birthDate.After(time.Now().AddDate(-16, 0, 0)) {
			send(w, 400, map[string]string{"error": "invalid birth_date"})
			return
		}
		employedSince, err := time.Parse("2006-01-02", body.EmployedSince)
		if err != nil || employedSince.After(time.Now()) {
			send(w, 400, map[string]string{"error": "invalid employed_since"})
			return
		}
		if strings.TrimSpace(body.FirstName) == "" || strings.TrimSpace(body.LastName) == "" || strings.TrimSpace(body.Department) == "" || strings.TrimSpace(body.Team) == "" {
			send(w, http.StatusBadRequest, map[string]string{"error": "first_name, last_name, department and team are required"})
			return
		}

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
		if !ok || currentUser.Role != model.RoleTeamlead {
			send(w, http.StatusForbidden, map[string]string{
				"error": "forbidden",
			})
			return
		}

		var user *model.User
		var errCreate error
		if creator, ok := d.Interns.(interface {
			CreateInternWithAssignments(context.Context, string, string, model.Profession, uuid.UUID, uuid.UUID, time.Time, time.Time, string, string, string, string, string) (*model.User, error)
		}); ok {
			user, errCreate = creator.CreateInternWithAssignments(r.Context(), body.Email, passwordHash, body.Profession, buddyID, currentUser.ID, birthDate, employedSince, strings.TrimSpace(body.FirstName), strings.TrimSpace(body.LastName), strings.TrimSpace(body.TelegramUsername), strings.TrimSpace(body.Department), strings.TrimSpace(body.Team))
		} else {
			send(w, http.StatusInternalServerError, map[string]string{"error": "intern creation unavailable"})
			return
		}
		err = errCreate

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
		case errors.Is(err, service.ErrUserNotTeamlead):
			send(w, 403, map[string]string{"error": "user is not a teamlead"})
			return
		case errors.Is(err, service.ErrInvalidProfile):
			send(w, 400, map[string]string{"error": "invalid profile"})
			return
		case errors.Is(err, repository.ErrNotFound):
			send(w, 400, map[string]string{"error": "assigned user not found"})
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

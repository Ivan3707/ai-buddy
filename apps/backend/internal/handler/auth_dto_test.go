package handler_test

import "github.com/nvkmaksim/ai-buddy/backend/internal/model"

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

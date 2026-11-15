package model

import "time"

type AdminAccount struct {
	ID             string     `json:"id"`
	Email          string     `json:"email"`
	FirstName      string     `json:"first_name"`
	LastName       string     `json:"last_name"`
	Role           string     `json:"role"`
	Profession     string     `json:"profession"`
	Team           string     `json:"team"`
	Department     string     `json:"department"`
	BuddyName      string     `json:"buddy_name"`
	TeamleadName   string     `json:"teamlead_name"`
	CreatedAt      time.Time  `json:"created_at"`
	LastLoginAt    *time.Time `json:"last_login_at"`
	ActiveSessions int        `json:"active_sessions"`
}

type AnswerFeedbackItem struct {
	MessageID string    `json:"message_id"`
	Comment   string    `json:"comment"`
	CreatedAt time.Time `json:"created_at"`
}

type AnswerFeedback struct {
	Up    int                  `json:"up"`
	Down  int                  `json:"down"`
	Items []AnswerFeedbackItem `json:"items"`
}

type AdminCounters struct {
	UsersByRole     map[string]int `json:"users_by_role"`
	OpenQuestions   int            `json:"open_questions"`
	PendingRequests int            `json:"pending_requests"`
	MessagesToday   int            `json:"messages_today"`
	ActiveSessions  int            `json:"active_sessions"`
}

type StuckTask struct {
	Task      string `json:"task"`
	UserEmail string `json:"user_email"`
	Hint      string `json:"hint_status"`
	HintText  string `json:"hint_text"`
}

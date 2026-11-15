package service

import "github.com/nvkmaksim/ai-buddy/backend/internal/model"

// ---------------------------------------------------------------- DTOs (contracts/backend.openapi.json)
//
// Types of the assistant API, serialized as-is (ARCH-01). PendingAction, ServiceRequest and ChatMessage stay here:
// they are read and written only by AssistantService and mirror the API shape. MentorQuestion is a domain entity
// shared with internal/repository, so it lives in internal/model; the alias keeps the service API unchanged.

type PendingAction = model.PendingAction
type ServiceRequest = model.ServiceRequest

type MentorQuestion = model.MentorQuestion

type ActionResult struct {
	Action         PendingAction   `json:"action"`
	Request        *ServiceRequest `json:"request,omitempty"`
	MentorQuestion *MentorQuestion `json:"mentor_question,omitempty"`
}

type ChatMessage = model.ChatMessage
type ChatMetrics = model.ChatMetrics

type PageContext struct {
	URL       string `json:"url"`
	Title     string `json:"title,omitempty"`
	Selection string `json:"selection,omitempty"`
	// Text is the readable text of the page (not HTML), at most MaxPageTextRunes; untrusted data.
	Text string `json:"text,omitempty"`
}

// MaxPageTextRunes limits PageContext.Text (also in the AI service schema and in the contract).
const MaxPageTextRunes = 6000

type taskContext struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	Description    string `json:"description,omitempty"`
	Status         string `json:"status"`
	Checklist      string `json:"checklist"`
	StageTitle     string `json:"stage_title,omitempty"`
	RequiresResult bool   `json:"requires_result,omitempty"`
	ReviewComment  string `json:"review_comment,omitempty"`
}

type stageContext struct {
	Title    string `json:"title"`
	Position int    `json:"position"`
	Total    int    `json:"total"`
}

type UserContext struct {
	UserID         string           `json:"user_id"`
	DisplayName    string           `json:"display_name,omitempty"`
	Role           string           `json:"role"`
	Profession     *string          `json:"profession"`
	Team           string           `json:"team,omitempty"`
	BuddyName      string           `json:"buddy_name,omitempty"`
	Today          string           `json:"today"`
	CurrentStage   *stageContext    `json:"current_stage,omitempty"`
	Tasks          []taskContext    `json:"tasks"`
	OpenRequests   []ServiceRequest `json:"open_requests"`
	AllowedActions []string         `json:"allowed_actions"`
	// Staff only (buddy, teamlead): the interns this user is responsible for, as the backend sees them.
	MyInterns        []teamInternContext `json:"my_interns"`
	OpenQuestions    []openQuestionInfo  `json:"open_mentor_questions,omitempty"`
	PendingApprovals []pendingApproval   `json:"pending_approvals,omitempty"`
}

// teamInternContext is a short progress summary of one intern for a mentor or team lead.
type teamInternContext struct {
	Name         string              `json:"name"`
	Profession   *string             `json:"profession,omitempty"`
	Team         string              `json:"team,omitempty"`
	Buddy        string              `json:"buddy,omitempty"`
	CurrentStage string              `json:"current_stage,omitempty"`
	Checklists   []checklistProgress `json:"checklists"`
	CurrentTasks []string            `json:"current_tasks,omitempty"`
	OnReview     int                 `json:"on_review"`
	Returned     int                 `json:"returned"`
	LastLogin    string              `json:"last_login,omitempty"`
}

type checklistProgress struct {
	Checklist string `json:"checklist"`
	Done      int    `json:"done"`
	Total     int    `json:"total"`
}

type openQuestionInfo struct {
	Intern   string `json:"intern"`
	Question string `json:"question"`
	Since    string `json:"since"`
}

type pendingApproval struct {
	Intern string `json:"intern"`
	Number string `json:"number"`
	Type   string `json:"type"`
	Status string `json:"status"`
}

type ChatTurn = model.ChatTurn

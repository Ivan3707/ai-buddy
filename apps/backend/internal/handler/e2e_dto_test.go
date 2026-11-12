package handler_test

import "github.com/nvkmaksim/ai-buddy/backend/internal/model"

type checklistItemDTO struct {
	ID               string                `json:"id"`
	Title            string                `json:"title"`
	Description      string                `json:"description"`
	ExpectedResult   *string               `json:"expected_result"`
	RequiresReview   bool                  `json:"requires_review"`
	EstimatedMinutes *int                  `json:"estimated_minutes"`
	Position         int                   `json:"position"`
	Status           model.ChecklistStatus `json:"status"`
	CompletedAt      *string               `json:"completed_at"`
}

type checklistStageDTO struct {
	ID             string             `json:"id"`
	Title          string             `json:"title"`
	WeekStart      int                `json:"week_start"`
	WeekEnd        int                `json:"week_end"`
	Goal           string             `json:"goal"`
	ExpectedResult string             `json:"expected_result"`
	Position       int                `json:"position"`
	Items          []checklistItemDTO `json:"items"`
}

type checklistDTO struct {
	ID                string              `json:"id"`
	Name              string              `json:"name"`
	Type              model.ChecklistType `json:"type"`
	Profession        *model.Profession   `json:"profession"`
	Stages            []checklistStageDTO `json:"stages"`
	TotalItems        int                 `json:"total_items"`
	CompletedItems    int                 `json:"completed_items"`
	CompletionPercent float64             `json:"completion_percent"`
}

type checklistsResponse struct {
	Common       checklistDTO `json:"common"`
	Professional checklistDTO `json:"professional"`
}

type submitItemRequest struct {
	ResultText *string `json:"result_text"`
	ResultURL  *string `json:"result_url"`
}

type submissionDTO struct {
	ID            string                 `json:"id"`
	ProgressID    string                 `json:"progress_id"`
	AttemptNumber int                    `json:"attempt_number"`
	ResultText    *string                `json:"result_text"`
	ResultURL     *string                `json:"result_url"`
	Status        model.SubmissionStatus `json:"status"`
	SubmittedAt   string                 `json:"submitted_at"`
}

type submissionHistoryDTO struct {
	submissionDTO
	ReviewedAt    *string `json:"reviewed_at"`
	ReviewComment *string `json:"review_comment"`
}

type pendingSubmissionDTO struct {
	ID              string  `json:"id"`
	InternID        string  `json:"intern_id"`
	InternFirstName *string `json:"first_name"`
	InternLastName  *string `json:"last_name"`
	ItemID          string  `json:"item_id"`
	ItemTitle       string  `json:"item_title"`
	AttemptNumber   int     `json:"attempt_number"`
	ResultText      *string `json:"result_text"`
	ResultURL       *string `json:"result_url"`
	SubmittedAt     string  `json:"submitted_at"`
}

type reviewSubmissionRequest struct {
	Approved *bool   `json:"approved"`
	Comment  *string `json:"comment"`
}

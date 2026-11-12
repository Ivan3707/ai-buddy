package handler

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/nvkmaksim/ai-buddy/backend/internal/middleware"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/service"
)

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

func (d Dependencies) checklistsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, _ := middleware.CurrentUser(r.Context())
		result, err := d.Checklists.GetByUserID(r.Context(), user.ID)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		send(w, http.StatusOK, checklistsResponse{
			Common:       toChecklistDTO(result.Common),
			Professional: toChecklistDTO(result.Professional),
		})
	})
}

func (d Dependencies) completeItemHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		itemID, ok := parsePathUUID(w, r, "itemID", "invalid checklist item ID")
		if !ok {
			return
		}
		user, _ := middleware.CurrentUser(r.Context())
		if err := d.Items.CompleteItem(r.Context(), user.ID, itemID); err != nil {
			writeServiceError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func (d Dependencies) submitItemHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		itemID, ok := parsePathUUID(w, r, "itemID", "invalid checklist item ID")
		if !ok {
			return
		}
		var body submitItemRequest
		if !decodeJSONBody(w, r, &body) {
			return
		}
		user, _ := middleware.CurrentUser(r.Context())
		submission, err := d.Items.SubmitItem(r.Context(), user.ID, itemID, body.ResultText, body.ResultURL)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		send(w, http.StatusCreated, toSubmissionDTO(submission))
	})
}

func (d Dependencies) submissionHistoryHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		itemID, ok := parsePathUUID(w, r, "itemID", "invalid checklist item ID")
		if !ok {
			return
		}
		user, _ := middleware.CurrentUser(r.Context())
		history, err := d.Items.ListSubmissions(r.Context(), user.ID, itemID)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		response := make([]submissionHistoryDTO, 0, len(history))
		for _, submission := range history {
			response = append(response, submissionHistoryDTO{
				submissionDTO: toSubmissionDTO(submission),
				ReviewedAt:    formatOptionalTime(submission.ReviewedAt),
				ReviewComment: submission.ReviewComment,
			})
		}
		send(w, http.StatusOK, response)
	})
}

func (d Dependencies) pendingSubmissionsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, _ := middleware.CurrentUser(r.Context())
		pending, err := d.Submissions.ListPendingSubmissions(r.Context(), user.ID)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		response := make([]pendingSubmissionDTO, 0, len(pending))
		for _, item := range pending {
			response = append(response, pendingSubmissionDTO{
				ID:              item.Submission.ID.String(),
				InternID:        item.InternID.String(),
				InternFirstName: item.InternFirstName,
				InternLastName:  item.InternLastName,
				ItemID:          item.ItemID.String(),
				ItemTitle:       item.ItemTitle,
				AttemptNumber:   item.Submission.AttemptNumber,
				ResultText:      item.Submission.ResultText,
				ResultURL:       item.Submission.ResultURL,
				SubmittedAt:     formatTime(item.Submission.SubmittedAt),
			})
		}
		send(w, http.StatusOK, response)
	})
}

func (d Dependencies) reviewSubmissionHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		submissionID, ok := parsePathUUID(w, r, "submissionID", "invalid submission ID")
		if !ok {
			return
		}
		var body reviewSubmissionRequest
		if !decodeJSONBody(w, r, &body) {
			return
		}
		if body.Approved == nil {
			send(w, http.StatusBadRequest, map[string]string{"error": "approved is required"})
			return
		}
		if !*body.Approved && (body.Comment == nil || strings.TrimSpace(*body.Comment) == "") {
			send(w, http.StatusBadRequest, map[string]string{"error": "comment is required for rejection"})
			return
		}
		user, _ := middleware.CurrentUser(r.Context())
		err := d.Submissions.ReviewSubmission(r.Context(), user.ID, submissionID, *body.Approved, body.Comment)
		if errors.Is(err, service.ErrInternNotAssigned) {
			send(w, http.StatusNotFound, map[string]string{"error": "submission not found"})
			return
		}
		if err != nil {
			writeServiceError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func toChecklistDTO(value service.ChecklistWithProgress) checklistDTO {
	stages := make([]checklistStageDTO, 0, len(value.Stages))
	for _, stage := range value.Stages {
		items := make([]checklistItemDTO, 0, len(stage.Items))
		for _, item := range stage.Items {
			items = append(items, checklistItemDTO{
				ID:               item.Item.ID.String(),
				Title:            item.Item.Title,
				Description:      item.Item.Description,
				ExpectedResult:   item.Item.ExpectedResult,
				RequiresReview:   item.Item.RequiresReview,
				EstimatedMinutes: item.Item.EstimatedMinutes,
				Position:         item.Item.Position,
				Status:           item.Status,
				CompletedAt:      formatOptionalTime(item.CompletedAt),
			})
		}
		stages = append(stages, checklistStageDTO{
			ID:             stage.Stage.ID.String(),
			Title:          stage.Stage.Title,
			WeekStart:      stage.Stage.WeekStart,
			WeekEnd:        stage.Stage.WeekEnd,
			Goal:           stage.Stage.Goal,
			ExpectedResult: stage.Stage.ExpectedResult,
			Position:       stage.Stage.Position,
			Items:          items,
		})
	}
	return checklistDTO{
		ID:                value.Checklist.ID.String(),
		Name:              value.Checklist.Name,
		Type:              value.Checklist.Type,
		Profession:        value.Checklist.Profession,
		Stages:            stages,
		TotalItems:        value.TotalItems,
		CompletedItems:    value.CompletedItems,
		CompletionPercent: value.CompletionPercent,
	}
}

func toSubmissionDTO(value model.ChecklistItemSubmission) submissionDTO {
	return submissionDTO{
		ID:            value.ID.String(),
		ProgressID:    value.ProgressID.String(),
		AttemptNumber: value.AttemptNumber,
		ResultText:    value.ResultText,
		ResultURL:     value.ResultURL,
		Status:        value.Status,
		SubmittedAt:   formatTime(value.SubmittedAt),
	}
}

func formatTime(value time.Time) string { return value.UTC().Format(time.RFC3339) }

func formatOptionalTime(value *time.Time) *string {
	if value == nil {
		return nil
	}
	formatted := formatTime(*value)
	return &formatted
}

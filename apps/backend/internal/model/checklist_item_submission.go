package model

import (
	"time"

	"github.com/google/uuid"
)

type SubmissionStatus string

const (
	SubmissionPending  SubmissionStatus = "pending"
	SubmissionApproved SubmissionStatus = "approved"
	SubmissionRejected SubmissionStatus = "rejected"
)

type ChecklistItemSubmission struct {
	ID            uuid.UUID
	ProgressID    uuid.UUID
	AttemptNumber int
	ResultText    *string
	ResultURL     *string
	Status        SubmissionStatus
	SubmittedAt   time.Time
	ReviewedAt    *time.Time
	ReviewedBy    *uuid.UUID
	ReviewComment *string
}

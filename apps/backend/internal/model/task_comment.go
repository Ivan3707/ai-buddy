package model

import (
	"time"

	"github.com/google/uuid"
)

type TaskAttachment struct {
	ID          uuid.UUID
	Filename    string
	ContentType string
	Size        int
}

type TaskComment struct {
	ID          uuid.UUID
	AuthorID    uuid.UUID
	AuthorName  string
	AuthorRole  UserRole
	Body        string
	CreatedAt   time.Time
	Attachments []TaskAttachment
}

type AttachmentFile struct {
	Filename    string
	ContentType string
	Data        []byte
}

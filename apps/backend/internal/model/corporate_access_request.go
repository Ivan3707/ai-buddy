package model

import (
	"github.com/google/uuid"
	"time"
)

type CorporateAccessRequest struct {
	ID           uuid.UUID `json:"id"`
	Tool         string    `json:"tool"`
	TicketNumber string    `json:"ticket_number"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
}

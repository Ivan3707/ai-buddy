package service

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
	"strings"
)

var ErrInvalidPersonal = errors.New("invalid personal record")

type PersonalService struct {
	repo *repository.PersonalRepository
}

func NewPersonalService(repo *repository.PersonalRepository) *PersonalService {
	return &PersonalService{repo}
}
func (s *PersonalService) CreateNote(ctx context.Context, userID uuid.UUID, n *repository.Note) error {
	if strings.TrimSpace(n.Title) == "" {
		return ErrInvalidPersonal
	}
	return s.repo.CreateNote(ctx, userID, n)
}
func (s *PersonalService) ListNotes(ctx context.Context, userID uuid.UUID) ([]repository.Note, error) {
	return s.repo.ListNotes(ctx, userID)
}
func (s *PersonalService) GetNote(ctx context.Context, userID, id uuid.UUID) (*repository.Note, error) {
	return s.repo.GetNote(ctx, userID, id)
}
func (s *PersonalService) UpdateNote(ctx context.Context, userID uuid.UUID, n *repository.Note) error {
	if strings.TrimSpace(n.Title) == "" {
		return ErrInvalidPersonal
	}
	return s.repo.UpdateNote(ctx, userID, n)
}
func (s *PersonalService) DeleteNote(ctx context.Context, userID, id uuid.UUID) error {
	return s.repo.DeleteNote(ctx, userID, id)
}
func (s *PersonalService) CreateEvent(ctx context.Context, userID uuid.UUID, e *repository.CalendarEvent) error {
	if err := validateEvent(e); err != nil {
		return err
	}
	return s.repo.CreateEvent(ctx, userID, e)
}
func (s *PersonalService) ListEvents(ctx context.Context, userID uuid.UUID) ([]repository.CalendarEvent, error) {
	return s.repo.ListEvents(ctx, userID)
}
func (s *PersonalService) GetEvent(ctx context.Context, userID, id uuid.UUID) (*repository.CalendarEvent, error) {
	return s.repo.GetEvent(ctx, userID, id)
}
func (s *PersonalService) UpdateEvent(ctx context.Context, userID uuid.UUID, e *repository.CalendarEvent) error {
	if err := validateEvent(e); err != nil {
		return err
	}
	return s.repo.UpdateEvent(ctx, userID, e)
}
func (s *PersonalService) DeleteEvent(ctx context.Context, userID, id uuid.UUID) error {
	return s.repo.DeleteEvent(ctx, userID, id)
}
func validateEvent(e *repository.CalendarEvent) error {
	if strings.TrimSpace(e.Title) == "" || e.StartsAt.IsZero() || !e.EndsAt.After(e.StartsAt) {
		return ErrInvalidPersonal
	}
	return nil
}

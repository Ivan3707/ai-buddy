package repository

import (
	"context"
	"database/sql"
	"errors"
	"github.com/google/uuid"
	"time"
)

type Note struct {
	ID        uuid.UUID `json:"id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	UpdatedAt time.Time `json:"updated_at"`
}
type CalendarEvent struct {
	ID          uuid.UUID `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	StartsAt    time.Time `json:"starts_at"`
	EndsAt      time.Time `json:"ends_at"`
}
type PersonalRepository struct{ db *sql.DB }

func NewPersonalRepository(db *sql.DB) *PersonalRepository { return &PersonalRepository{db} }
func (r *PersonalRepository) CreateNote(ctx context.Context, userID uuid.UUID, n *Note) error {
	return r.db.QueryRowContext(ctx, `INSERT INTO notes(user_id,title,content) VALUES($1,$2,$3) RETURNING id,updated_at`, userID, n.Title, n.Content).Scan(&n.ID, &n.UpdatedAt)
}
func (r *PersonalRepository) ListNotes(ctx context.Context, userID uuid.UUID) ([]Note, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,title,content,updated_at FROM notes WHERE user_id=$1 ORDER BY updated_at DESC,id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Note, 0)
	for rows.Next() {
		var n Note
		if err = rows.Scan(&n.ID, &n.Title, &n.Content, &n.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}
func (r *PersonalRepository) GetNote(ctx context.Context, userID, id uuid.UUID) (*Note, error) {
	var n Note
	err := r.db.QueryRowContext(ctx, `SELECT id,title,content,updated_at FROM notes WHERE user_id=$1 AND id=$2`, userID, id).Scan(&n.ID, &n.Title, &n.Content, &n.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &n, err
}
func (r *PersonalRepository) UpdateNote(ctx context.Context, userID uuid.UUID, n *Note) error {
	err := r.db.QueryRowContext(ctx, `UPDATE notes SET title=$1,content=$2,updated_at=now() WHERE id=$3 AND user_id=$4 RETURNING updated_at`, n.Title, n.Content, n.ID, userID).Scan(&n.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
func (r *PersonalRepository) DeleteNote(ctx context.Context, userID, id uuid.UUID) error {
	return affected(r.db.ExecContext(ctx, `DELETE FROM notes WHERE id=$1 AND user_id=$2`, id, userID))
}
func (r *PersonalRepository) CreateEvent(ctx context.Context, userID uuid.UUID, e *CalendarEvent) error {
	return r.db.QueryRowContext(ctx, `INSERT INTO calendar_events(user_id,title,description,starts_at,ends_at) VALUES($1,$2,$3,$4,$5) RETURNING id`, userID, e.Title, e.Description, e.StartsAt, e.EndsAt).Scan(&e.ID)
}
func (r *PersonalRepository) ListEvents(ctx context.Context, userID uuid.UUID) ([]CalendarEvent, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,title,description,starts_at,ends_at FROM calendar_events WHERE user_id=$1 ORDER BY starts_at,id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]CalendarEvent, 0)
	for rows.Next() {
		var e CalendarEvent
		if err = rows.Scan(&e.ID, &e.Title, &e.Description, &e.StartsAt, &e.EndsAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
func (r *PersonalRepository) GetEvent(ctx context.Context, userID, id uuid.UUID) (*CalendarEvent, error) {
	var e CalendarEvent
	err := r.db.QueryRowContext(ctx, `SELECT id,title,description,starts_at,ends_at FROM calendar_events WHERE user_id=$1 AND id=$2`, userID, id).Scan(&e.ID, &e.Title, &e.Description, &e.StartsAt, &e.EndsAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &e, err
}
func (r *PersonalRepository) UpdateEvent(ctx context.Context, userID uuid.UUID, e *CalendarEvent) error {
	return affected(r.db.ExecContext(ctx, `UPDATE calendar_events SET title=$1,description=$2,starts_at=$3,ends_at=$4 WHERE id=$5 AND user_id=$6`, e.Title, e.Description, e.StartsAt, e.EndsAt, e.ID, userID))
}
func (r *PersonalRepository) DeleteEvent(ctx context.Context, userID, id uuid.UUID) error {
	return affected(r.db.ExecContext(ctx, `DELETE FROM calendar_events WHERE id=$1 AND user_id=$2`, id, userID))
}
func affected(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

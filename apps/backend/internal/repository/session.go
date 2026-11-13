package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type SessionRepository struct{ db *sql.DB }

func NewSessionRepository(db *sql.DB) *SessionRepository { return &SessionRepository{db} }

func (r *SessionRepository) Create(ctx context.Context, userID uuid.UUID, secret string, expires time.Time) (uuid.UUID, error) {
	h := sha256.Sum256([]byte(secret))
	var id uuid.UUID
	err := r.db.QueryRowContext(ctx, `INSERT INTO auth_sessions(user_id,token_hash,expires_at) VALUES($1,$2,$3) RETURNING id`, userID, h[:], expires).Scan(&id)
	return id, err
}

// Rotate locks the session and revokes it if an old or invalid token is replayed.
func (r *SessionRepository) Rotate(ctx context.Context, id uuid.UUID, secret, next string, expires time.Time) (uuid.UUID, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return uuid.Nil, err
	}
	defer tx.Rollback()
	var userID uuid.UUID
	var hash []byte
	var oldExpiry time.Time
	var revoked sql.NullTime
	err = tx.QueryRowContext(ctx, `SELECT user_id,token_hash,expires_at,revoked_at FROM auth_sessions WHERE id=$1 FOR UPDATE`, id).Scan(&userID, &hash, &oldExpiry, &revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, ErrNotFound
	}
	if err != nil {
		return uuid.Nil, err
	}
	if revoked.Valid || !oldExpiry.After(time.Now()) {
		return uuid.Nil, ErrNotFound
	}
	got := sha256.Sum256([]byte(secret))
	if !equalHash(hash, got[:]) {
		if _, err = tx.ExecContext(ctx, `UPDATE auth_sessions SET revoked_at=now() WHERE id=$1`, id); err != nil {
			return uuid.Nil, err
		}
		if err = tx.Commit(); err != nil {
			return uuid.Nil, err
		}
		return uuid.Nil, ErrNotFound
	}
	nextHash := sha256.Sum256([]byte(next))
	if _, err = tx.ExecContext(ctx, `UPDATE auth_sessions SET token_hash=$1,expires_at=$2 WHERE id=$3`, nextHash[:], expires, id); err != nil {
		return uuid.Nil, err
	}
	if err = tx.Commit(); err != nil {
		return uuid.Nil, fmt.Errorf("rotate session: %w", err)
	}
	return userID, nil
}
func equalHash(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := range a {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}
func (r *SessionRepository) Revoke(ctx context.Context, id, userID uuid.UUID, secret string) error {
	hash := sha256.Sum256([]byte(secret))
	result, err := r.db.ExecContext(ctx, `UPDATE auth_sessions SET revoked_at=now()
		WHERE id=$1 AND user_id=$2 AND token_hash=$3
		AND revoked_at IS NULL AND expires_at>now()`, id, userID, hash[:])
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return ErrNotFound
	}
	return nil
}

func (r *SessionRepository) IsActive(ctx context.Context, id, userID uuid.UUID) (bool, error) {
	var active bool
	err := r.db.QueryRowContext(ctx, `SELECT revoked_at IS NULL AND expires_at>now() FROM auth_sessions WHERE id=$1 AND user_id=$2`, id, userID).Scan(&active)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return active, err
}

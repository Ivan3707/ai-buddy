package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
)

type CorporateRepository struct{ db DBTX }

func NewCorporateRepository(db DBTX) *CorporateRepository { return &CorporateRepository{db: db} }
func (r *CorporateRepository) CreateRequest(ctx context.Context, userID, id uuid.UUID, tool, key, ticket string) (model.CorporateAccessRequest, error) {
	var out model.CorporateAccessRequest
	err := r.db.QueryRowContext(ctx, `INSERT INTO corporate_access_requests(id,user_id,tool,idempotency_key,ticket_number)
 VALUES($1,$2,$3,$4,$5) ON CONFLICT(user_id,idempotency_key) DO UPDATE SET id=corporate_access_requests.id
 RETURNING id,tool,ticket_number,status,created_at`, id, userID, tool, key, ticket).Scan(&out.ID, &out.Tool, &out.TicketNumber, &out.Status, &out.CreatedAt)
	return out, err
}
func (r *CorporateRepository) GetRequest(ctx context.Context, userID, id uuid.UUID) (model.CorporateAccessRequest, error) {
	var out model.CorporateAccessRequest
	err := r.db.QueryRowContext(ctx, `SELECT id,tool,ticket_number,status,created_at FROM corporate_access_requests WHERE id=$1 AND user_id=$2`, id, userID).Scan(&out.ID, &out.Tool, &out.TicketNumber, &out.Status, &out.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return out, ErrNotFound
	}
	return out, err
}
func (r *CorporateRepository) ListRequests(ctx context.Context, userID uuid.UUID) ([]model.CorporateAccessRequest, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,tool,ticket_number,status,created_at FROM corporate_access_requests WHERE user_id=$1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.CorporateAccessRequest{}
	for rows.Next() {
		var x model.CorporateAccessRequest
		if err := rows.Scan(&x.ID, &x.Tool, &x.TicketNumber, &x.Status, &x.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

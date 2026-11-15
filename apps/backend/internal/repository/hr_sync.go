package repository

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
)

type HRIdentity struct {
	ID                 uuid.UUID
	CorporateID, Email string
	Role               model.UserRole
}
type HRProfile struct {
	ID                                                      uuid.UUID
	CorporateID, Email, PasswordHash                        string
	Role                                                    model.UserRole
	Profession                                              any
	Department, Team                                        string
	BuddyID, TeamleadID                                     any
	FirstName, LastName, Telegram, BirthDate, EmployedSince *string
}
type HRSyncRepository struct{ db DBTX }

func NewHRSyncRepository(db DBTX) *HRSyncRepository { return &HRSyncRepository{db: db} }
func (r *HRSyncRepository) LockRun(ctx context.Context) error {
	_, e := r.db.ExecContext(ctx, `SELECT pg_advisory_xact_lock(80520263)`)
	return e
}
func (r *HRSyncRepository) LockedIdentities(ctx context.Context) ([]HRIdentity, error) {
	rows, e := r.db.QueryContext(ctx, `SELECT id,COALESCE(corporate_id,''),email,role FROM users FOR UPDATE`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []HRIdentity{}
	for rows.Next() {
		var v HRIdentity
		if e := rows.Scan(&v.ID, &v.CorporateID, &v.Email, &v.Role); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (r *HRSyncRepository) ParkEmail(ctx context.Context, id uuid.UUID) error {
	_, e := r.db.ExecContext(ctx, `UPDATE users SET email=$2 WHERE id=$1`, id, "hr-sync-"+id.String()+"@invalid.local")
	return e
}
func (r *HRSyncRepository) Create(ctx context.Context, p HRProfile) error {
	_, e := r.db.ExecContext(ctx, `INSERT INTO users(id,corporate_id,email,password_hash,role,profession,department,team,first_name,last_name,telegram_username,birth_date,employed_since)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, p.ID, p.CorporateID, strings.ToLower(p.Email), p.PasswordHash, p.Role, p.Profession, p.Department, p.Team, p.FirstName, p.LastName, p.Telegram, p.BirthDate, p.EmployedSince)
	return e
}
func (r *HRSyncRepository) Update(ctx context.Context, p HRProfile) error {
	_, e := r.db.ExecContext(ctx, `UPDATE users SET corporate_id=$2,email=$3,role=$4,profession=$5,department=$6,team=$7,buddy_id=$8,teamlead_id=$9,first_name=COALESCE($10,first_name),last_name=COALESCE($11,last_name),telegram_username=COALESCE($12,telegram_username),birth_date=COALESCE($13::date,birth_date),employed_since=COALESCE($14::date,employed_since)WHERE id=$1`, p.ID, p.CorporateID, strings.ToLower(p.Email), p.Role, p.Profession, p.Department, p.Team, p.BuddyID, p.TeamleadID, p.FirstName, p.LastName, p.Telegram, p.BirthDate, p.EmployedSince)
	return e
}

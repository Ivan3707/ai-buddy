package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
)

type UserRepository struct {
	db DBTX
}

func NewUserRepository(db DBTX) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Create(
	ctx context.Context,
	user *model.User,
) error {
	query := `
		INSERT INTO users (email, password_hash, role, profession, buddy_id,
			first_name, last_name, telegram_username, department, team,
			teamlead_id,birth_date,employed_since)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING id, created_at
	`

	var profession any
	if user.Profession != nil {
		profession = string(*user.Profession)
	}

	var buddyID any
	if user.BuddyID != nil {
		buddyID = *user.BuddyID
	}

	err := r.db.QueryRowContext(
		ctx,
		query,
		user.Email,
		user.PasswordHash,
		string(user.Role),
		profession,
		buddyID,
		user.FirstName, user.LastName, user.TelegramUsername, user.Department, user.Team,
		user.TeamleadID, user.BirthDate, user.EmployedSince,
	).Scan(&user.ID, &user.CreatedAt)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			if pgErr.Code == "23505" &&
				pgErr.ConstraintName == "uq_users_email_lower" {
				return ErrEmailAlreadyExists
			}
		}

		return fmt.Errorf("create user: %w", err)
	}

	return nil
}

func (r *UserRepository) GetByID(
	ctx context.Context,
	id uuid.UUID,
) (*model.User, error) {
	query := `
		SELECT id, email, password_hash, role, profession, buddy_id, created_at, corporate_id,
			last_login_at, first_name, last_name, telegram_username, department, team,
			teamlead_id,birth_date,employed_since
		FROM users
		WHERE id = $1
	`

	return r.getUser(ctx, query, id)
}

func (r *UserRepository) GetByEmail(
	ctx context.Context,
	email string,
) (*model.User, error) {
	query := `
		SELECT id, email, password_hash, role, profession, buddy_id, created_at, corporate_id,
			last_login_at, first_name, last_name, telegram_username, department, team,
			teamlead_id,birth_date,employed_since
		FROM users
		WHERE LOWER(email) = LOWER($1)
	`

	return r.getUser(ctx, query, email)
}

// RecordLogin atomically updates last_login_at and reports whether its
// previous value was NULL. The row lock is acquired only for this statement.
func (r *UserRepository) RecordLogin(
	ctx context.Context,
	id uuid.UUID,
) (bool, error) {
	query := `
		WITH previous AS (
			SELECT id, last_login_at IS NULL AS is_first_login
			FROM users
			WHERE id = $1
			FOR UPDATE
		)
		UPDATE users AS u
		SET last_login_at = CURRENT_TIMESTAMP
		FROM previous
		WHERE u.id = previous.id
		RETURNING previous.is_first_login
	`

	var isFirstLogin bool
	if err := r.db.QueryRowContext(ctx, query, id).Scan(&isFirstLogin); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, ErrNotFound
		}
		return false, fmt.Errorf("record user login: %w", err)
	}

	return isFirstLogin, nil
}

func (r *UserRepository) ListAssignedInterns(ctx context.Context, actorID uuid.UUID, role model.UserRole) ([]model.User, error) {
	column := "buddy_id"
	if role == model.RoleTeamlead {
		column = "teamlead_id"
	} else if role != model.RoleBuddy {
		return []model.User{}, nil
	}
	query := `SELECT id, email, password_hash, role, profession, buddy_id, created_at, corporate_id,
		last_login_at, first_name, last_name, telegram_username, department, team,
		teamlead_id, birth_date, employed_since
		FROM users WHERE role='intern' AND ` + column + `=$1
		ORDER BY last_name NULLS LAST, first_name NULLS LAST, email`
	return r.listUsers(ctx, query, actorID)
}

func (r *UserRepository) ListByRole(ctx context.Context, role model.UserRole) ([]model.User, error) {
	query := `SELECT id, email, password_hash, role, profession, buddy_id, created_at, corporate_id,
		last_login_at, first_name, last_name, telegram_username, department, team,
		teamlead_id, birth_date, employed_since
		FROM users WHERE role=$1
		ORDER BY last_name NULLS LAST, first_name NULLS LAST, email`
	return r.listUsers(ctx, query, role)
}

func (r *UserRepository) listUsers(ctx context.Context, query string, arg any) ([]model.User, error) {
	rows, err := r.db.QueryContext(ctx, query, arg)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()
	users := make([]model.User, 0)
	for rows.Next() {
		var user model.User
		var profession, firstName, lastName, telegram, department, team sql.NullString
		var buddyID, teamleadID uuid.NullUUID
		var lastLoginAt, birthDate, employedSince sql.NullTime
		var corporateID sql.NullString
		if err := rows.Scan(&user.ID, &user.Email, &user.PasswordHash, &user.Role, &profession,
			&buddyID, &user.CreatedAt, &corporateID, &lastLoginAt, &firstName, &lastName, &telegram,
			&department, &team, &teamleadID, &birthDate, &employedSince); err != nil {
			return nil, fmt.Errorf("scan user list: %w", err)
		}
		if corporateID.Valid {
			user.CorporateID = &corporateID.String
		}
		applyNullableUserFields(&user, profession, buddyID, lastLoginAt, firstName, lastName, telegram, department, team, teamleadID, birthDate, employedSince)
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list users rows: %w", err)
	}
	return users, nil
}

func (r *UserRepository) getUser(
	ctx context.Context,
	query string,
	arg any,
) (*model.User, error) {
	user := &model.User{}
	var profession sql.NullString
	var buddyID uuid.NullUUID
	var lastLoginAt sql.NullTime
	var corporateID sql.NullString
	var firstName, lastName, telegram, department, team sql.NullString
	var teamleadID uuid.NullUUID
	var birthDate, employedSince sql.NullTime

	err := r.db.QueryRowContext(ctx, query, arg).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.Role,
		&profession,
		&buddyID,
		&user.CreatedAt,
		&corporateID,
		&lastLoginAt,
		&firstName, &lastName, &telegram, &department, &team,
		&teamleadID, &birthDate, &employedSince,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}

	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}

	if corporateID.Valid {
		user.CorporateID = &corporateID.String
	}
	applyNullableUserFields(user, profession, buddyID, lastLoginAt, firstName, lastName, telegram, department, team, teamleadID, birthDate, employedSince)
	return user, nil
}

func applyNullableUserFields(user *model.User, profession sql.NullString, buddyID uuid.NullUUID, lastLoginAt sql.NullTime, firstName, lastName, telegram, department, team sql.NullString, teamleadID uuid.NullUUID, birthDate, employedSince sql.NullTime) {
	if profession.Valid {
		value := model.Profession(profession.String)
		user.Profession = &value
	}

	if buddyID.Valid {
		user.BuddyID = &buddyID.UUID
	}

	if lastLoginAt.Valid {
		user.LastLoginAt = &lastLoginAt.Time
	}
	if firstName.Valid {
		user.FirstName = &firstName.String
	}
	if lastName.Valid {
		user.LastName = &lastName.String
	}
	if telegram.Valid {
		user.TelegramUsername = &telegram.String
	}
	if department.Valid {
		user.Department = &department.String
	}
	if team.Valid {
		user.Team = &team.String
	}
	if teamleadID.Valid {
		user.TeamleadID = &teamleadID.UUID
	}
	if birthDate.Valid {
		user.BirthDate = &birthDate.Time
	}
	if employedSince.Valid {
		user.EmployedSince = &employedSince.Time
	}
}

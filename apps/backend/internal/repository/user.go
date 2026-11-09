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
        INSERT INTO users (email, password_hash, role, profession)
        VALUES ($1, $2, $3, $4)
        RETURNING id, created_at
    `

	var profession any
	if user.Profession != nil {
		profession = string(*user.Profession)
	}

	err := r.db.QueryRowContext(
		ctx,
		query,
		user.Email,
		user.PasswordHash,
		string(user.Role),
		profession,
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
        SELECT id, email, password_hash, role, profession, created_at
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
        SELECT id, email, password_hash, role, profession, created_at
        FROM users
        WHERE LOWER(email) = LOWER($1)
    `

	return r.getUser(ctx, query, email)
}

func (r *UserRepository) getUser(
	ctx context.Context,
	query string,
	arg any,
) (*model.User, error) {
	user := &model.User{}
	var profession sql.NullString

	err := r.db.QueryRowContext(ctx, query, arg).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.Role,
		&profession,
		&user.CreatedAt,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}

	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}

	if profession.Valid {
		value := model.Profession(profession.String)
		user.Profession = &value
	}

	return user, nil
}

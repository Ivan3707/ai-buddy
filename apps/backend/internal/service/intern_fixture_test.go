package service

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
)

// createTestIntern inserts a synthetic intern and initializes onboarding
// progress through the production helper used by HRSynchronizer. It replaces
// the removed UserService.CreateIntern: tests only, accounts in the product
// are created by HR sync.
func createTestIntern(ctx context.Context, db *sql.DB, email, passwordHash string, profession model.Profession, buddyID uuid.UUID) (*model.User, error) {
	var created *model.User
	err := withTransaction(ctx, db, func(tx *sql.Tx) error {
		user := &model.User{
			Email:        email,
			PasswordHash: passwordHash,
			Role:         model.RoleIntern,
			Profession:   &profession,
			BuddyID:      &buddyID,
		}
		if err := repository.NewUserRepository(tx).Create(ctx, user); err != nil {
			return err
		}
		if err := InitializeInternProgress(ctx, tx, user.ID, profession); err != nil {
			return err
		}
		created = user
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

package service

import (
	"context"
	"database/sql"
	"fmt"
)

func withTransaction(
	ctx context.Context,
	db *sql.DB,
	fn func(*sql.Tx) error,
) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	defer func() {
		_ = tx.Rollback()
	}()

	if err := fn(tx); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

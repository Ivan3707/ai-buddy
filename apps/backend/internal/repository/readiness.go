package repository

import (
	"context"
	"errors"
	"fmt"
	"slices"
)

var (
	ErrDatabaseUnavailable = errors.New("database is unavailable")
	ErrSchemaNotMigrated   = errors.New("database schema is not migrated")
	ErrSchemaOutdated      = errors.New("database schema is outdated")
	ErrCatalogEmpty        = errors.New("checklist catalog is not loaded")
)

type ReadinessRepository struct {
	db               DBTX
	expectedVersions []int64
}

func NewReadinessRepository(db DBTX, expectedVersions []int64) *ReadinessRepository {
	return &ReadinessRepository{db: db, expectedVersions: slices.Clone(expectedVersions)}
}

func (r *ReadinessRepository) Check(ctx context.Context) error {
	var one int
	if err := r.db.QueryRowContext(ctx, `SELECT 1`).Scan(&one); err != nil {
		return fmt.Errorf("%w: %v", ErrDatabaseUnavailable, err)
	}

	rows, err := r.db.QueryContext(ctx, `SELECT DISTINCT version_id FROM goose_db_version WHERE is_applied AND version_id > 0 ORDER BY version_id`)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrSchemaNotMigrated, err)
	}
	defer rows.Close()
	applied := []int64{}
	for rows.Next() {
		var version int64
		if err := rows.Scan(&version); err != nil {
			return fmt.Errorf("%w: %v", ErrSchemaNotMigrated, err)
		}
		applied = append(applied, version)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("%w: %v", ErrSchemaNotMigrated, err)
	}
	if len(applied) == 0 {
		return ErrSchemaNotMigrated
	}
	if !slices.Equal(applied, r.expectedVersions) {
		return fmt.Errorf("%w: applied=%v expected=%v", ErrSchemaOutdated, applied, r.expectedVersions)
	}

	var catalogReady bool
	if err := r.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM checklist_items)`).Scan(&catalogReady); err != nil {
		return fmt.Errorf("%w: %v", ErrCatalogEmpty, err)
	}
	if !catalogReady {
		return ErrCatalogEmpty
	}
	return nil
}

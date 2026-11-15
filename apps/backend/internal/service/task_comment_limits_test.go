package service

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
)

// P2-07: migration 00006 duplicates the service limits as CHECK constraints, so a write that bypasses
// TaskCommentService still cannot store an oversized comment or a broken attachment.
func TestTaskCommentLimitsEnforcedByDatabase(t *testing.T) {
	db, ctx := openChecklistTestDB(t)
	fixture := createChecklistTestFixture(t, db, ctx, model.ProfessionBackend, 1, 1)
	intern := createChecklistTestIntern(t, db, ctx, model.ProfessionBackend)
	var progressID uuid.UUID
	if err := db.QueryRowContext(ctx, `SELECT id FROM user_checklist_progress WHERE user_id = $1 AND checklist_item_id = $2`,
		intern.ID, fixture.profItems[0].ID).Scan(&progressID); err != nil {
		t.Fatalf("load progress: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM task_comments WHERE progress_id = $1`, progressID)
	})
	insertComment := func(body string) (uuid.UUID, error) {
		var id uuid.UUID
		err := db.QueryRowContext(ctx, `INSERT INTO task_comments (progress_id, author_id, author_role, body) VALUES ($1, $2, 'intern', $3) RETURNING id`,
			progressID, intern.ID, body).Scan(&id)
		return id, err
	}
	if _, err := insertComment(strings.Repeat("я", MaxCommentChars+1)); err == nil {
		t.Fatal("database accepted a comment longer than MaxCommentChars")
	}
	commentID, err := insertComment(strings.Repeat("я", MaxCommentChars))
	if err != nil {
		t.Fatalf("comment at the limit rejected: %v", err)
	}
	for _, tc := range []struct {
		name, filename, contentType string
		size                        int
		data                        []byte
		ok                          bool
	}{
		{"valid", "notes.txt", "text/plain; charset=utf-8", 3, []byte("abc"), true},
		{"empty file", "empty.txt", "text/plain; charset=utf-8", 0, []byte{}, false},
		{"size mismatch", "notes.txt", "text/plain; charset=utf-8", 4, []byte("abc"), false},
		{"empty name", "", "text/plain; charset=utf-8", 3, []byte("abc"), false},
		{"long name", strings.Repeat("n", maxFilenameLength+1), "text/plain; charset=utf-8", 3, []byte("abc"), false},
		{"empty content type", "notes.txt", "", 3, []byte("abc"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := db.ExecContext(ctx, `INSERT INTO task_attachments (comment_id, filename, content_type, size_bytes, data) VALUES ($1, $2, $3, $4, $5)`,
				commentID, tc.filename, tc.contentType, tc.size, tc.data)
			if tc.ok && err != nil {
				t.Fatalf("valid attachment rejected: %v", err)
			}
			if !tc.ok && err == nil {
				t.Fatal("database accepted an invalid attachment")
			}
		})
	}
	var definition string
	if err := db.QueryRowContext(ctx, `SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conname = 'task_attachments_size_check'`).Scan(&definition); err != nil {
		t.Fatalf("read size constraint: %v", err)
	}
	if !strings.Contains(definition, strconv.Itoa(MaxFileBytes)) {
		t.Fatalf("migration file size limit %q differs from MaxFileBytes=%d", definition, MaxFileBytes)
	}
}

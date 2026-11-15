package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
)

type TaskCommentRepository struct{ db DBTX }

func NewTaskCommentRepository(db DBTX) *TaskCommentRepository { return &TaskCommentRepository{db: db} }
func (r *TaskCommentRepository) ProgressID(ctx context.Context, userID, itemID uuid.UUID) (uuid.UUID, error) {
	var id uuid.UUID
	e := r.db.QueryRowContext(ctx, `SELECT id FROM user_checklist_progress WHERE user_id=$1 AND checklist_item_id=$2`, userID, itemID).Scan(&id)
	if errors.Is(e, sql.ErrNoRows) {
		e = ErrNotFound
	}
	return id, e
}
func (r *TaskCommentRepository) List(ctx context.Context, progressID uuid.UUID) ([]model.TaskComment, error) {
	rows, e := r.db.QueryContext(ctx, `SELECT c.id,c.author_id,c.author_role,c.body,c.created_at,COALESCE(u.first_name,''),COALESCE(u.last_name,''),u.email FROM task_comments c JOIN users u ON u.id=c.author_id WHERE c.progress_id=$1 ORDER BY c.created_at,c.id`, progressID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []model.TaskComment{}
	index := map[uuid.UUID]int{}
	for rows.Next() {
		var v model.TaskComment
		var first, last, email string
		if e := rows.Scan(&v.ID, &v.AuthorID, &v.AuthorRole, &v.Body, &v.CreatedAt, &first, &last, &email); e != nil {
			return nil, e
		}
		v.AuthorName = displayName(first, last, email)
		v.Attachments = []model.TaskAttachment{}
		index[v.ID] = len(out)
		out = append(out, v)
	}
	if e := rows.Err(); e != nil {
		return nil, e
	}
	files, e := r.db.QueryContext(ctx, `SELECT a.id,a.comment_id,a.filename,a.content_type,a.size_bytes FROM task_attachments a JOIN task_comments c ON c.id=a.comment_id WHERE c.progress_id=$1 ORDER BY a.created_at,a.id`, progressID)
	if e != nil {
		return nil, e
	}
	defer files.Close()
	for files.Next() {
		var a model.TaskAttachment
		var commentID uuid.UUID
		if e := files.Scan(&a.ID, &commentID, &a.Filename, &a.ContentType, &a.Size); e != nil {
			return nil, e
		}
		if i, ok := index[commentID]; ok {
			out[i].Attachments = append(out[i].Attachments, a)
		}
	}
	return out, files.Err()
}
func displayName(first, last, email string) string {
	if v := strings.TrimSpace(first + " " + last); v != "" {
		return v
	}
	return email
}
func (r *TaskCommentRepository) AddComment(ctx context.Context, progressID, authorID uuid.UUID, role model.UserRole, body string) (model.TaskComment, error) {
	v := model.TaskComment{AuthorID: authorID, AuthorRole: role, Body: body, Attachments: []model.TaskAttachment{}}
	e := r.db.QueryRowContext(ctx, `INSERT INTO task_comments(progress_id,author_id,author_role,body)VALUES($1,$2,$3,$4)RETURNING id,created_at`, progressID, authorID, string(role), body).Scan(&v.ID, &v.CreatedAt)
	return v, e
}
func (r *TaskCommentRepository) AddAttachment(ctx context.Context, commentID uuid.UUID, filename, contentType string, data []byte) (model.TaskAttachment, error) {
	v := model.TaskAttachment{Filename: filename, ContentType: contentType, Size: len(data)}
	e := r.db.QueryRowContext(ctx, `INSERT INTO task_attachments(comment_id,filename,content_type,size_bytes,data)VALUES($1,$2,$3,$4,$5)RETURNING id`, commentID, filename, contentType, len(data), data).Scan(&v.ID)
	return v, e
}
func (r *TaskCommentRepository) AuthorName(ctx context.Context, id uuid.UUID) (string, error) {
	var first, last, email string
	e := r.db.QueryRowContext(ctx, `SELECT COALESCE(first_name,''),COALESCE(last_name,''),email FROM users WHERE id=$1`, id).Scan(&first, &last, &email)
	return displayName(first, last, email), e
}

type AttachmentAccess struct {
	File                model.AttachmentFile
	Owner               uuid.UUID
	BuddyID, TeamleadID *uuid.UUID
}

func (r *TaskCommentRepository) Attachment(ctx context.Context, id uuid.UUID) (*AttachmentAccess, error) {
	v := &AttachmentAccess{}
	e := r.db.QueryRowContext(ctx, `SELECT a.filename,a.content_type,a.data,p.user_id,u.buddy_id,u.teamlead_id FROM task_attachments a JOIN task_comments c ON c.id=a.comment_id JOIN user_checklist_progress p ON p.id=c.progress_id JOIN users u ON u.id=p.user_id WHERE a.id=$1`, id).Scan(&v.File.Filename, &v.File.ContentType, &v.File.Data, &v.Owner, &v.BuddyID, &v.TeamleadID)
	if errors.Is(e, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return v, e
}

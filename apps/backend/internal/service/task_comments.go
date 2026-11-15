package service

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"path"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
)

const (
	MaxCommentChars   = 2000
	MaxCommentFiles   = 5
	MaxFileBytes      = 5 << 20
	MaxCommentBytes   = 10 << 20
	maxFilenameLength = 120
)

var (
	ErrCommentForbidden = errors.New("no access to this task thread")
	ErrCommentInvalid   = errors.New("invalid comment")
)

// allowedFileTypes: extension -> stored content type. Everything else is rejected.
var allowedFileTypes = map[string]string{
	".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".gif": "image/gif", ".webp": "image/webp",
	".pdf": "application/pdf", ".txt": "text/plain; charset=utf-8", ".md": "text/plain; charset=utf-8",
	".csv": "text/plain; charset=utf-8", ".log": "text/plain; charset=utf-8", ".json": "application/json",
	".zip":  "application/zip",
	".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	".pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
}

type CommentFile struct {
	Filename string
	Data     []byte
}

type TaskAttachment struct {
	ID          uuid.UUID
	Filename    string
	ContentType string
	Size        int
}

type TaskComment struct {
	ID          uuid.UUID
	AuthorID    uuid.UUID
	AuthorName  string
	AuthorRole  model.UserRole
	Body        string
	CreatedAt   time.Time
	Attachments []TaskAttachment
}

type AttachmentFile struct {
	Filename    string
	ContentType string
	Data        []byte
}

type TaskCommentService struct{ db *sql.DB }

func NewTaskCommentService(db *sql.DB) *TaskCommentService { return &TaskCommentService{db: db} }

// threadProgress resolves the thread owner and checks the viewer's right to it.
// Interns see only their own thread; a buddy / teamlead only the threads of their interns.
func (s *TaskCommentService) threadProgress(ctx context.Context, viewer *model.User, itemID uuid.UUID, internID *uuid.UUID) (uuid.UUID, error) {
	owner := viewer.ID
	if viewer.Role == model.RoleIntern {
		if internID != nil && *internID != viewer.ID {
			return uuid.Nil, ErrCommentForbidden
		}
	} else {
		if internID == nil {
			return uuid.Nil, ErrCommentInvalid
		}
		owner = *internID
		var buddyID, leadID *uuid.UUID
		var role string
		err := s.db.QueryRowContext(ctx, `SELECT role, buddy_id, teamlead_id FROM users WHERE id = $1`, owner).Scan(&role, &buddyID, &leadID)
		if errors.Is(err, sql.ErrNoRows) {
			return uuid.Nil, repository.ErrNotFound
		}
		if err != nil {
			return uuid.Nil, err
		}
		mine := role == string(model.RoleIntern) &&
			((viewer.Role == model.RoleBuddy && buddyID != nil && *buddyID == viewer.ID) ||
				(viewer.Role == model.RoleTeamlead && leadID != nil && *leadID == viewer.ID))
		if !mine {
			return uuid.Nil, ErrCommentForbidden
		}
	}
	var progressID uuid.UUID
	err := s.db.QueryRowContext(ctx, `SELECT id FROM user_checklist_progress WHERE user_id = $1 AND checklist_item_id = $2`, owner, itemID).Scan(&progressID)
	if errors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, repository.ErrNotFound
	}
	return progressID, err
}

func displayName(first, last, email string) string {
	if name := strings.TrimSpace(first + " " + last); name != "" {
		return name
	}
	return email
}

func (s *TaskCommentService) List(ctx context.Context, viewer *model.User, itemID uuid.UUID, internID *uuid.UUID) ([]TaskComment, error) {
	progressID, err := s.threadProgress(ctx, viewer, itemID, internID)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.id, c.author_id, c.author_role, c.body, c.created_at, COALESCE(u.first_name,''), COALESCE(u.last_name,''), u.email
		FROM task_comments c JOIN users u ON u.id = c.author_id
		WHERE c.progress_id = $1 ORDER BY c.created_at, c.id`, progressID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TaskComment{}
	index := map[uuid.UUID]int{}
	for rows.Next() {
		var c TaskComment
		var role, first, last, email string
		if err := rows.Scan(&c.ID, &c.AuthorID, &role, &c.Body, &c.CreatedAt, &first, &last, &email); err != nil {
			return nil, err
		}
		c.AuthorRole = model.UserRole(role)
		c.AuthorName = displayName(first, last, email)
		c.Attachments = []TaskAttachment{}
		index[c.ID] = len(out)
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	files, err := s.db.QueryContext(ctx, `
		SELECT a.id, a.comment_id, a.filename, a.content_type, a.size_bytes
		FROM task_attachments a JOIN task_comments c ON c.id = a.comment_id
		WHERE c.progress_id = $1 ORDER BY a.created_at, a.id`, progressID)
	if err != nil {
		return nil, err
	}
	defer files.Close()
	for files.Next() {
		var a TaskAttachment
		var commentID uuid.UUID
		if err := files.Scan(&a.ID, &commentID, &a.Filename, &a.ContentType, &a.Size); err != nil {
			return nil, err
		}
		if i, ok := index[commentID]; ok {
			out[i].Attachments = append(out[i].Attachments, a)
		}
	}
	return out, files.Err()
}

// SanitizeFilename keeps the base name, drops control characters and caps the length.
func SanitizeFilename(name string) string {
	name = path.Base(strings.ReplaceAll(name, `\`, "/"))
	name = strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, name))
	if name == "." || name == "/" {
		return ""
	}
	if runes := []rune(name); len(runes) > maxFilenameLength {
		ext := path.Ext(name)
		if n := utf8.RuneCountInString(ext); n > 0 && n <= 20 {
			name = string(runes[:maxFilenameLength-n]) + ext
		} else {
			name = string(runes[:maxFilenameLength])
		}
	}
	return name
}

func checkFile(f CommentFile) (name, contentType string, err error) {
	name = SanitizeFilename(f.Filename)
	contentType, ok := allowedFileTypes[strings.ToLower(path.Ext(name))]
	if name == "" || !ok || len(f.Data) == 0 || len(f.Data) > MaxFileBytes {
		return "", "", ErrCommentInvalid
	}
	// Pictures are also checked by content, so that something else cannot pose as one.
	if strings.HasPrefix(contentType, "image/") && !strings.HasPrefix(http.DetectContentType(f.Data), "image/") {
		return "", "", ErrCommentInvalid
	}
	return name, contentType, nil
}

func (s *TaskCommentService) Add(ctx context.Context, author *model.User, itemID uuid.UUID, internID *uuid.UUID, body string, files []CommentFile) (*TaskComment, error) {
	body = strings.TrimSpace(body)
	if utf8.RuneCountInString(body) > MaxCommentChars || (body == "" && len(files) == 0) || len(files) > MaxCommentFiles {
		return nil, ErrCommentInvalid
	}
	type stored struct {
		name, contentType string
		data              []byte
	}
	total := 0
	prepared := make([]stored, 0, len(files))
	for _, f := range files {
		name, ct, err := checkFile(f)
		if err != nil {
			return nil, err
		}
		total += len(f.Data)
		prepared = append(prepared, stored{name, ct, f.Data})
	}
	if total > MaxCommentBytes {
		return nil, ErrCommentInvalid
	}
	progressID, err := s.threadProgress(ctx, author, itemID, internID)
	if err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	c := TaskComment{AuthorID: author.ID, AuthorRole: author.Role, Body: body, Attachments: []TaskAttachment{}}
	if err := tx.QueryRowContext(ctx, `INSERT INTO task_comments (progress_id, author_id, author_role, body) VALUES ($1,$2,$3,$4) RETURNING id, created_at`,
		progressID, author.ID, string(author.Role), body).Scan(&c.ID, &c.CreatedAt); err != nil {
		return nil, err
	}
	for _, p := range prepared {
		a := TaskAttachment{Filename: p.name, ContentType: p.contentType, Size: len(p.data)}
		if err := tx.QueryRowContext(ctx, `INSERT INTO task_attachments (comment_id, filename, content_type, size_bytes, data) VALUES ($1,$2,$3,$4,$5) RETURNING id`,
			c.ID, a.Filename, a.ContentType, a.Size, p.data).Scan(&a.ID); err != nil {
			return nil, err
		}
		c.Attachments = append(c.Attachments, a)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	var first, last, email string
	_ = s.db.QueryRowContext(ctx, `SELECT COALESCE(first_name,''), COALESCE(last_name,''), email FROM users WHERE id = $1`, author.ID).Scan(&first, &last, &email)
	c.AuthorName = displayName(first, last, email)
	return &c, nil
}

// Attachment returns the file if the viewer may see its thread (the owner intern, their buddy or teamlead).
func (s *TaskCommentService) Attachment(ctx context.Context, viewer *model.User, id uuid.UUID) (*AttachmentFile, error) {
	var f AttachmentFile
	var owner uuid.UUID
	var buddyID, leadID *uuid.UUID
	err := s.db.QueryRowContext(ctx, `
		SELECT a.filename, a.content_type, a.data, p.user_id, u.buddy_id, u.teamlead_id
		FROM task_attachments a
		JOIN task_comments c ON c.id = a.comment_id
		JOIN user_checklist_progress p ON p.id = c.progress_id
		JOIN users u ON u.id = p.user_id
		WHERE a.id = $1`, id).Scan(&f.Filename, &f.ContentType, &f.Data, &owner, &buddyID, &leadID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repository.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	ok := owner == viewer.ID ||
		(viewer.Role == model.RoleBuddy && buddyID != nil && *buddyID == viewer.ID) ||
		(viewer.Role == model.RoleTeamlead && leadID != nil && *leadID == viewer.ID)
	if !ok {
		return nil, ErrCommentForbidden
	}
	return &f, nil
}

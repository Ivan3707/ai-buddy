package service

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"path"
	"strings"
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

type TaskAttachment = model.TaskAttachment
type TaskComment = model.TaskComment
type AttachmentFile = model.AttachmentFile

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
		target, err := repository.NewUserRepository(s.db).GetByID(ctx, owner)
		if errors.Is(err, repository.ErrNotFound) {
			return uuid.Nil, repository.ErrNotFound
		}
		if err != nil {
			return uuid.Nil, err
		}
		mine := target.Role == model.RoleIntern &&
			((viewer.Role == model.RoleBuddy && target.BuddyID != nil && *target.BuddyID == viewer.ID) ||
				(viewer.Role == model.RoleTeamlead && target.TeamleadID != nil && *target.TeamleadID == viewer.ID))
		if !mine {
			return uuid.Nil, ErrCommentForbidden
		}
	}
	return repository.NewTaskCommentRepository(s.db).ProgressID(ctx, owner, itemID)
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
	return repository.NewTaskCommentRepository(s.db).List(ctx, progressID)
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
	repo := repository.NewTaskCommentRepository(tx)
	c, err := repo.AddComment(ctx, progressID, author.ID, author.Role, body)
	if err != nil {
		return nil, err
	}
	for _, p := range prepared {
		a, err := repo.AddAttachment(ctx, c.ID, p.name, p.contentType, p.data)
		if err != nil {
			return nil, err
		}
		c.Attachments = append(c.Attachments, a)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	c.AuthorName, _ = repository.NewTaskCommentRepository(s.db).AuthorName(ctx, author.ID)
	return &c, nil
}

// Attachment returns the file if the viewer may see its thread (the owner intern, their buddy or teamlead).
func (s *TaskCommentService) Attachment(ctx context.Context, viewer *model.User, id uuid.UUID) (*AttachmentFile, error) {
	access, err := repository.NewTaskCommentRepository(s.db).Attachment(ctx, id)
	if err != nil {
		return nil, err
	}
	ok := access.Owner == viewer.ID ||
		(viewer.Role == model.RoleBuddy && access.BuddyID != nil && *access.BuddyID == viewer.ID) ||
		(viewer.Role == model.RoleTeamlead && access.TeamleadID != nil && *access.TeamleadID == viewer.ID)
	if !ok {
		return nil, ErrCommentForbidden
	}
	return &access.File, nil
}

package handler

import (
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/middleware"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
	"github.com/nvkmaksim/ai-buddy/backend/internal/service"
)

type taskAttachmentDTO struct {
	ID          string `json:"id"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        int    `json:"size"`
}

type taskCommentDTO struct {
	ID          string              `json:"id"`
	AuthorID    string              `json:"author_id"`
	AuthorName  string              `json:"author_name"`
	AuthorRole  model.UserRole      `json:"author_role"`
	Body        string              `json:"body"`
	CreatedAt   string              `json:"created_at"`
	Attachments []taskAttachmentDTO `json:"attachments"`
}

func toTaskCommentDTO(c service.TaskComment) taskCommentDTO {
	files := make([]taskAttachmentDTO, 0, len(c.Attachments))
	for _, a := range c.Attachments {
		files = append(files, taskAttachmentDTO{ID: a.ID.String(), Filename: a.Filename, ContentType: a.ContentType, Size: a.Size})
	}
	return taskCommentDTO{
		ID: c.ID.String(), AuthorID: c.AuthorID.String(), AuthorName: c.AuthorName, AuthorRole: c.AuthorRole,
		Body: c.Body, CreatedAt: c.CreatedAt.UTC().Format(time.RFC3339), Attachments: files,
	}
}

func writeCommentError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrCommentForbidden):
		send(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
	case errors.Is(err, service.ErrCommentInvalid):
		send(w, http.StatusBadRequest, map[string]string{"error": "Комментарий пустой или вложения не подходят: до 5 файлов, каждый до 5 МБ, всего до 10 МБ; допустимы изображения, PDF, текст, CSV, JSON, ZIP и документы Office."})
	case errors.Is(err, repository.ErrNotFound):
		send(w, http.StatusNotFound, map[string]string{"error": "resource not found"})
	default:
		log.Printf("task comments error: %v", err)
		send(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}
}

func optionalUUID(raw string) (*uuid.UUID, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, true
	}
	id, err := uuid.Parse(raw)
	if err != nil || id == uuid.Nil {
		return nil, false
	}
	return &id, true
}

// RegisterTaskCommentRoutes: comments (with attachments) of an intern and their mentor under one task.
func RegisterTaskCommentRoutes(mux *http.ServeMux, auth func(http.Handler) http.Handler, s TaskComments) {
	route := func(pattern string, fn func(http.ResponseWriter, *http.Request, *model.User)) {
		mux.Handle(pattern, auth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := middleware.CurrentUser(r.Context())
			if !ok {
				send(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
				return
			}
			fn(w, r, user)
		})))
	}
	route("GET /api/checklist-items/{itemID}/comments", func(w http.ResponseWriter, r *http.Request, user *model.User) {
		itemID, ok := parsePathUUID(w, r, "itemID", "invalid checklist item ID")
		if !ok {
			return
		}
		internID, ok := optionalUUID(r.URL.Query().Get("intern_id"))
		if !ok {
			send(w, http.StatusBadRequest, map[string]string{"error": "invalid intern ID"})
			return
		}
		list, err := s.List(r.Context(), user, itemID, internID)
		if err != nil {
			writeCommentError(w, err)
			return
		}
		out := make([]taskCommentDTO, 0, len(list))
		for _, c := range list {
			out = append(out, toTaskCommentDTO(c))
		}
		send(w, http.StatusOK, out)
	})
	route("POST /api/checklist-items/{itemID}/comments", func(w http.ResponseWriter, r *http.Request, user *model.User) {
		itemID, ok := parsePathUUID(w, r, "itemID", "invalid checklist item ID")
		if !ok {
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, service.MaxCommentBytes+(1<<20))
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			status := http.StatusBadRequest
			message := "Не удалось прочитать форму."
			var tooBig *http.MaxBytesError
			if errors.As(err, &tooBig) {
				status, message = http.StatusRequestEntityTooLarge, "Вложения слишком большие: всего до 10 МБ."
			}
			send(w, status, map[string]string{"error": message})
			return
		}
		defer func() { _ = r.MultipartForm.RemoveAll() }()
		internID, ok := optionalUUID(r.FormValue("intern_id"))
		if !ok {
			send(w, http.StatusBadRequest, map[string]string{"error": "invalid intern ID"})
			return
		}
		headers := r.MultipartForm.File["files"]
		if len(headers) > service.MaxCommentFiles {
			writeCommentError(w, service.ErrCommentInvalid)
			return
		}
		files := make([]service.CommentFile, 0, len(headers))
		for _, header := range headers {
			if header.Size > service.MaxFileBytes {
				send(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "Файл слишком большой: до 5 МБ."})
				return
			}
			file, err := header.Open()
			if err != nil {
				writeCommentError(w, service.ErrCommentInvalid)
				return
			}
			data, err := io.ReadAll(io.LimitReader(file, service.MaxFileBytes+1))
			_ = file.Close()
			if err != nil {
				writeCommentError(w, service.ErrCommentInvalid)
				return
			}
			files = append(files, service.CommentFile{Filename: header.Filename, Data: data})
		}
		comment, err := s.Add(r.Context(), user, itemID, internID, r.FormValue("body"), files)
		if err != nil {
			writeCommentError(w, err)
			return
		}
		send(w, http.StatusCreated, toTaskCommentDTO(*comment))
	})
	route("GET /api/attachments/{attachmentID}", func(w http.ResponseWriter, r *http.Request, user *model.User) {
		id, ok := parsePathUUID(w, r, "attachmentID", "invalid attachment ID")
		if !ok {
			return
		}
		file, err := s.Attachment(r.Context(), user, id)
		if err != nil {
			writeCommentError(w, err)
			return
		}
		inline := file.ContentType == "image/png" || file.ContentType == "image/jpeg" || file.ContentType == "image/gif" || file.ContentType == "image/webp"
		disposition := "attachment"
		if inline {
			disposition = "inline"
		}
		w.Header().Set("Content-Type", file.ContentType)
		w.Header().Set("Content-Disposition", disposition+"; filename*=UTF-8''"+encodeFilename(file.Filename))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "sandbox")
		w.Header().Set("Content-Length", strconv.Itoa(len(file.Data)))
		w.Header().Set("Cache-Control", "private, no-store")
		_, _ = w.Write(file.Data)
	})
}

// encodeFilename percent-encodes a file name for the RFC 5987 filename* parameter.
func encodeFilename(name string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	for _, c := range []byte(name) {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_', c == '.':
			b.WriteByte(c)
		default:
			b.WriteByte('%')
			b.WriteByte(hex[c>>4])
			b.WriteByte(hex[c&15])
		}
	}
	return b.String()
}

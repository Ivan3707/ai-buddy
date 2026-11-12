package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
)

var (
	ErrCorporateUnavailable    = errors.New("corporate service unavailable")
	ErrCorporateForbidden      = errors.New("corporate access denied")
	ErrCorporateNotFound       = errors.New("corporate record not found")
	ErrIdempotencyConflict     = errors.New("idempotency key used for another tool")
	ErrInvalidCorporateRequest = errors.New("invalid corporate request")
)

type CorporateEmployee struct {
	ID           string            `json:"id"`
	Email        string            `json:"email"`
	Role         model.UserRole    `json:"role"`
	Profession   *model.Profession `json:"profession,omitempty"`
	Department   string            `json:"department"`
	Team         string            `json:"team"`
	ManagerID    string            `json:"manager_id,omitempty"`
	BuddyID      string            `json:"buddy_id,omitempty"`
	AllowedTools []string          `json:"allowed_tools"`
}
type CorporateDirectory struct {
	byEmail map[string]CorporateEmployee
	byID    map[string]CorporateEmployee
}

func NewCorporateDirectory(data []byte) (*CorporateDirectory, error) {
	var employees []CorporateEmployee
	if err := json.Unmarshal(data, &employees); err != nil {
		return nil, err
	}
	d := &CorporateDirectory{byEmail: map[string]CorporateEmployee{}, byID: map[string]CorporateEmployee{}}
	for _, e := range employees {
		email := strings.ToLower(strings.TrimSpace(e.Email))
		if e.ID == "" || email == "" || (e.Role != model.RoleIntern && e.Role != model.RoleBuddy && e.Role != model.RoleTeamlead) || d.byEmail[email].ID != "" || d.byID[e.ID].ID != "" {
			return nil, fmt.Errorf("invalid corporate directory entry")
		}
		d.byEmail[email] = e
		d.byID[e.ID] = e
	}
	return d, nil
}
func (d *CorporateDirectory) ForUser(user *model.User) (CorporateEmployee, error) {
	if d == nil {
		return CorporateEmployee{}, ErrCorporateUnavailable
	}
	e, ok := d.byEmail[strings.ToLower(user.Email)]
	if !ok {
		return CorporateEmployee{}, ErrCorporateNotFound
	}
	if e.Role != user.Role || (user.Profession == nil) != (e.Profession == nil) || (user.Profession != nil && *e.Profession != *user.Profession) {
		return CorporateEmployee{}, ErrCorporateForbidden
	}
	return e, nil
}
func (d *CorporateDirectory) ByID(id string) (CorporateEmployee, bool) {
	if d == nil {
		return CorporateEmployee{}, false
	}
	e, ok := d.byID[id]
	return e, ok
}

type AccessRequest = model.CorporateAccessRequest
type CorporateService struct {
	repo        *repository.CorporateRepository
	Directory   *CorporateDirectory
	Unavailable bool
}

func NewCorporateService(db repository.DBTX, d *CorporateDirectory) *CorporateService {
	return &CorporateService{repo: repository.NewCorporateRepository(db), Directory: d}
}
func (s *CorporateService) identity(user *model.User) (CorporateEmployee, error) {
	if s.Unavailable {
		return CorporateEmployee{}, ErrCorporateUnavailable
	}
	return s.Directory.ForUser(user)
}
func (s *CorporateService) Profile(user *model.User) (CorporateEmployee, error) {
	return s.identity(user)
}
func (s *CorporateService) ToolInfo(user *model.User, tool string) (map[string]string, error) {
	e, err := s.identity(user)
	if err != nil {
		return nil, err
	}
	for _, allowed := range e.AllowedTools {
		if allowed == tool {
			return map[string]string{"tool": tool, "instructions": "Create an access request with POST /api/corporate/access-requests and confirm=true.", "source": "corporate-mock"}, nil
		}
	}
	return nil, ErrCorporateForbidden
}
func (s *CorporateService) CreateRequest(ctx context.Context, user *model.User, tool, key string) (AccessRequest, bool, error) {
	e, err := s.identity(user)
	if err != nil {
		return AccessRequest{}, false, err
	}
	allowed := false
	for _, t := range e.AllowedTools {
		if t == tool {
			allowed = true
			break
		}
	}
	if !allowed {
		return AccessRequest{}, false, ErrCorporateForbidden
	}
	if len(key) < 8 || len(key) > 128 || strings.TrimSpace(key) != key {
		return AccessRequest{}, false, ErrInvalidCorporateRequest
	}
	id := uuid.New()
	ticket := "SD-" + strings.ToUpper(id.String())
	result, err := s.repo.CreateRequest(ctx, user.ID, id, tool, key, ticket)
	if err != nil {
		return AccessRequest{}, false, err
	}
	if result.Tool != tool {
		return AccessRequest{}, false, ErrIdempotencyConflict
	}
	return result, result.ID == id, nil
}
func (s *CorporateService) GetRequest(ctx context.Context, user *model.User, id uuid.UUID) (AccessRequest, error) {
	if _, err := s.identity(user); err != nil {
		return AccessRequest{}, err
	}
	result, err := s.repo.GetRequest(ctx, user.ID, id)
	if errors.Is(err, repository.ErrNotFound) {
		return AccessRequest{}, ErrCorporateNotFound
	}
	return result, err
}
func (s *CorporateService) ListRequests(ctx context.Context, user *model.User) ([]AccessRequest, error) {
	if _, err := s.identity(user); err != nil {
		return nil, err
	}
	return s.repo.ListRequests(ctx, user.ID)
}

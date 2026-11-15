package service

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
	"github.com/nvkmaksim/ai-buddy/backend/internal/repository"
	"github.com/nvkmaksim/ai-buddy/backend/internal/security"
)

// InitialCredential is emitted only for newly created accounts. The caller
// must store it outside source control; HR data never contains credentials.
type InitialCredential struct {
	CorporateID string `json:"corporate_id"`
	Email       string `json:"email"`
	Password    string `json:"password"`
}

// HRSynchronizer is the only production mechanism that creates accounts
// (Mock HR snapshot -> users + onboarding progress).
//
// The whole run stays in one transaction under a single advisory lock; the
// service resolves identities and assignments while HRSyncRepository performs
// the PostgreSQL operations on the caller-owned transaction.
type HRSynchronizer struct{ db *sql.DB }

// ErrHRCommitUncertain means PostgreSQL did not confirm whether COMMIT took
// effect. The already published credentials must be retained.
var ErrHRCommitUncertain = errors.New("HR sync commit outcome is uncertain; retain credentials file")

func hrProfessionValue(profession *model.Profession) any {
	if profession == nil {
		return nil
	}
	return string(*profession)
}

func NewHRSynchronizer(db *sql.DB) *HRSynchronizer { return &HRSynchronizer{db: db} }

func validateHRSnapshot(employees []CorporateEmployee) error {
	ids, emails := map[string]bool{}, map[string]bool{}
	for _, e := range employees {
		id, email := strings.TrimSpace(e.ID), strings.ToLower(strings.TrimSpace(e.Email))
		address, err := mail.ParseAddress(email)
		if id == "" || email == "" || err != nil || address.Address != email || strings.ContainsAny(id, "\r\n") {
			return fmt.Errorf("invalid HR identity %q", e.ID)
		}
		if ids[id] {
			return fmt.Errorf("duplicate corporate ID %q", id)
		}
		ids[id] = true
		if emails[email] {
			return fmt.Errorf("duplicate HR email %q", email)
		}
		emails[email] = true
		for _, field := range []*string{e.BirthDate, e.EmployedSince} {
			if field != nil {
				if _, err := time.Parse("2006-01-02", *field); err != nil {
					return fmt.Errorf("invalid HR date for %s: %w", id, err)
				}
			}
		}
		switch e.Role {
		case model.RoleIntern:
			if e.Profession == nil || (*e.Profession != model.ProfessionBackend && *e.Profession != model.ProfessionFrontend && *e.Profession != model.ProfessionML) {
				return fmt.Errorf("invalid profession for %s", id)
			}
			if e.BuddyID == "" || e.ManagerID == "" {
				return fmt.Errorf("missing assignment for %s", id)
			}
		case model.RoleBuddy, model.RoleTeamlead:
			if e.Profession != nil || e.BuddyID != "" || e.ManagerID != "" {
				return fmt.Errorf("invalid staff profile for %s", id)
			}
		default:
			return fmt.Errorf("invalid role for %s", id)
		}
	}
	byID := map[string]CorporateEmployee{}
	for _, e := range employees {
		byID[e.ID] = e
	}
	for _, e := range employees {
		if e.Role == model.RoleIntern {
			buddy, bok := byID[e.BuddyID]
			lead, lok := byID[e.ManagerID]
			if !bok || buddy.Role != model.RoleBuddy || !lok || lead.Role != model.RoleTeamlead {
				return fmt.Errorf("invalid Buddy or Teamlead reference for %s", e.ID)
			}
		}
	}
	return nil
}

// Sync publishes durable credentials before committing new accounts. The
// publish callback must atomically create a complete file without replacing
// an existing one. Missing HR entries, hashes, IDs and progress are kept.
func (s *HRSynchronizer) Sync(ctx context.Context, employees []CorporateEmployee, publish func([]InitialCredential) error) (newAccounts int, resultErr error) {
	if publish == nil {
		return 0, errors.New("credentials publisher is required")
	}
	if err := validateHRSnapshot(employees); err != nil {
		return 0, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	commitAttempted := false
	defer func() {
		if !commitAttempted {
			if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
				resultErr = errors.Join(resultErr, fmt.Errorf("HR sync rollback failed: %w", rollbackErr))
			}
		}
	}()
	// Serialize concurrent runs before any identity decisions.
	repo := repository.NewHRSyncRepository(tx)
	if err = repo.LockRun(ctx); err != nil {
		return 0, err
	}
	type local struct {
		id          uuid.UUID
		corp, email string
		role        model.UserRole
	}
	byCorp, byEmail := map[string]local{}, map[string]local{}
	identities, err := repo.LockedIdentities(ctx)
	if err != nil {
		return 0, err
	}
	for _, identity := range identities {
		u := local{id: identity.ID, corp: identity.CorporateID, email: identity.Email, role: identity.Role}
		if u.corp != "" {
			byCorp[u.corp] = u
		}
		byEmail[strings.ToLower(u.email)] = u
	}
	// Resolve every identity before changing any row. Email is used only once
	// to link legacy demo accounts that do not yet have a corporate ID.
	resolved, used := map[string]uuid.UUID{}, map[uuid.UUID]string{}
	for _, e := range employees {
		if u, ok := byCorp[e.ID]; ok {
			resolved[e.ID] = u.id
			used[u.id] = e.ID
			continue
		}
		if u, ok := byEmail[strings.ToLower(e.Email)]; ok && u.corp == "" && u.role == e.Role {
			if other := used[u.id]; other != "" {
				return 0, fmt.Errorf("identity conflict for %s", e.ID)
			}
			resolved[e.ID] = u.id
			used[u.id] = e.ID
		}
	}
	for _, e := range employees {
		if u, ok := byEmail[strings.ToLower(e.Email)]; ok && used[u.id] != e.ID {
			// A pair of linked HR employees may exchange addresses. Their old
			// addresses are freed before the final profile updates below.
			if used[u.id] == "" {
				return 0, fmt.Errorf("email %s belongs to another user", e.Email)
			}
		}
	}
	// Free old email addresses first, allowing two HR records to exchange them.
	for _, e := range employees {
		if id, ok := resolved[e.ID]; ok {
			u := byCorp[e.ID]
			if u.id == uuid.Nil {
				u = byEmail[strings.ToLower(e.Email)]
			}
			if !strings.EqualFold(u.email, e.Email) {
				if err = repo.ParkEmail(ctx, id); err != nil {
					return 0, err
				}
			}
		}
	}
	credentials := []InitialCredential{}
	for _, e := range employees {
		if _, ok := resolved[e.ID]; ok {
			continue
		}
		random := make([]byte, 32)
		if _, err = rand.Read(random); err != nil {
			return 0, err
		}
		password := base64.RawURLEncoding.EncodeToString(random)
		hash, hashErr := security.HashPassword(password)
		if hashErr != nil {
			return 0, hashErr
		}
		id := uuid.New()
		resolved[e.ID] = id
		err = repo.Create(ctx, repository.HRProfile{ID: id, CorporateID: e.ID, Email: e.Email, PasswordHash: hash, Role: e.Role, Profession: hrProfessionValue(e.Profession), Department: e.Department, Team: e.Team, FirstName: e.FirstName, LastName: e.LastName, Telegram: e.Telegram, BirthDate: e.BirthDate, EmployedSince: e.EmployedSince})
		if err != nil {
			return 0, fmt.Errorf("create %s: %w", e.ID, err)
		}
		credentials = append(credentials, InitialCredential{e.ID, e.Email, password})
	}
	for _, e := range employees {
		id := resolved[e.ID]
		var buddy, lead any
		if e.Role == model.RoleIntern {
			buddy = resolved[e.BuddyID]
			lead = resolved[e.ManagerID]
		}
		err = repo.Update(ctx, repository.HRProfile{ID: id, CorporateID: e.ID, Email: e.Email, Role: e.Role, Profession: hrProfessionValue(e.Profession), Department: e.Department, Team: e.Team, BuddyID: buddy, TeamleadID: lead, FirstName: e.FirstName, LastName: e.LastName, Telegram: e.Telegram, BirthDate: e.BirthDate, EmployedSince: e.EmployedSince})
		if err != nil {
			return 0, fmt.Errorf("update %s: %w", e.ID, err)
		}
		if e.Role != model.RoleIntern {
			continue
		}
		// Add only missing progress; old profession progress is retained for history.
		if err = InitializeInternProgress(ctx, tx, id, *e.Profession); err != nil {
			return 0, fmt.Errorf("initialize progress for %s: %w", e.ID, err)
		}
	}
	if err = publish(credentials); err != nil {
		return 0, fmt.Errorf("publish initial credentials: %w", err)
	}
	commitAttempted = true
	if err = tx.Commit(); err != nil {
		return 0, errors.Join(ErrHRCommitUncertain, err)
	}
	return len(credentials), nil
}

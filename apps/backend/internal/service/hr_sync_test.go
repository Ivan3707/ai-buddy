package service

import (
	"strings"
	"testing"

	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
)

func validHRFixture() []CorporateEmployee {
	profession := model.ProfessionBackend
	return []CorporateEmployee{
		{ID: "lead-1", Email: "lead@example.test", Role: model.RoleTeamlead},
		{ID: "buddy-1", Email: "buddy@example.test", Role: model.RoleBuddy},
		{ID: "intern-1", Email: "intern@example.test", Role: model.RoleIntern, Profession: &profession, BuddyID: "buddy-1", ManagerID: "lead-1"},
	}
}
func TestValidateHRSnapshot(t *testing.T) {
	tests := []struct {
		name   string
		change func([]CorporateEmployee)
		want   string
	}{
		{"duplicate ID", func(v []CorporateEmployee) { v[1].ID = v[0].ID }, "duplicate corporate ID"},
		{"duplicate email", func(v []CorporateEmployee) { v[1].Email = v[0].Email }, "duplicate HR email"},
		{"bad role", func(v []CorporateEmployee) { v[1].Role = "admin" }, "invalid role"},
		{"bad profession", func(v []CorporateEmployee) { p := model.Profession("unknown"); v[2].Profession = &p }, "invalid profession"},
		{"bad buddy", func(v []CorporateEmployee) { v[2].BuddyID = "missing" }, "invalid Buddy"},
		{"bad teamlead", func(v []CorporateEmployee) { v[2].ManagerID = "buddy-1" }, "invalid Buddy"},
		{"bad start date", func(v []CorporateEmployee) { value := "2026-02-30"; v[2].EmployedSince = &value }, "invalid HR date"},
	}
	if err := validateHRSnapshot(validHRFixture()); err != nil {
		t.Fatalf("valid snapshot: %v", err)
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v := validHRFixture()
			tc.change(v)
			if err := validateHRSnapshot(v); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want %s", err, tc.want)
			}
		})
	}
}

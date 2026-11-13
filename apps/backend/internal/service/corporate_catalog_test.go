package service

import (
	"testing"

	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
)

func TestCorporateToolCatalogUsesCurrentEmployeePermissions(t *testing.T) {
	directory, err := NewCorporateDirectory([]byte(`[
		{"id":"one","email":"one@example.com","role":"intern","profession":"backend","allowed_tools":["repository"]},
		{"id":"two","email":"two@example.com","role":"intern","profession":"frontend","allowed_tools":["issue-tracker"]}
	]`))
	if err != nil {
		t.Fatal(err)
	}
	profession := model.ProfessionBackend
	svc := NewCorporateService(nil, directory)
	tools, err := svc.ToolsForUser(&model.User{Email: "one@example.com", Role: model.RoleIntern, Profession: &profession})
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 2 || tools[0].Tool != "issue-tracker" || tools[0].Allowed || tools[1].Tool != "repository" || !tools[1].Allowed {
		t.Fatalf("unexpected catalog: %+v", tools)
	}
}

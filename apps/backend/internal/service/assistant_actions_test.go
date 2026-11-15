package service

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/nvkmaksim/ai-buddy/backend/internal/model"
)

func TestValidateParamsAccessRequest(t *testing.T) {
	valid := map[string]string{"system": "grafana", "resource": "payments", "access_level": "read", "justification": "онбординг"}
	with := func(key, value string) json.RawMessage {
		params := map[string]string{}
		for k, v := range valid {
			params[k] = v
		}
		if value == "<delete>" {
			delete(params, key)
		} else {
			params[key] = value
		}
		raw, _ := json.Marshal(params)
		return raw
	}
	cases := []struct {
		name   string
		params json.RawMessage
		ok     bool
	}{
		{"valid", with("resource", "payments"), true},
		{"admin is a known level", with("access_level", "admin"), true},
		{"resource at limit", with("resource", strings.Repeat("р", maxAccessResource)), true},
		{"justification at limit", with("justification", strings.Repeat("ж", maxAccessJustification)), true},
		{"missing resource", with("resource", "<delete>"), false},
		{"blank resource", with("resource", "   "), false},
		{"resource too long", with("resource", strings.Repeat("р", maxAccessResource+1)), false},
		{"blank justification", with("justification", " \t"), false},
		{"justification too long", with("justification", strings.Repeat("ж", maxAccessJustification+1)), false},
		{"unknown level", with("access_level", "owner"), false},
		{"unknown system", with("system", "prod-db"), false},
		{"not JSON", json.RawMessage(`{`), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := validateParams("access_request", tc.params)
			if tc.ok && err != nil {
				t.Fatalf("expected valid params, got %v", err)
			}
			if !tc.ok && !errors.Is(err, ErrActionInvalid) {
				t.Fatalf("expected ErrActionInvalid, got %v", err)
			}
		})
	}
}

func TestValidateParamsLeaveAndQuestion(t *testing.T) {
	cases := []struct {
		name, kind, params string
		ok                 bool
	}{
		{"leave", "leave_request", `{"kind":"day_off","date_from":"2026-11-20","date_to":"2026-11-20"}`, true},
		{"leave comment too long", "leave_request", `{"kind":"day_off","date_from":"2026-11-20","date_to":"2026-11-20","comment":"` + strings.Repeat("к", maxLeaveComment+1) + `"}`, false},
		{"leave reversed dates", "leave_request", `{"kind":"vacation","date_from":"2026-11-20","date_to":"2026-11-19"}`, false},
		{"leave too long", "leave_request", `{"kind":"vacation","date_from":"2026-01-01","date_to":"2026-04-01"}`, false},
		{"question", "mentor_question", `{"question":"Где взять доступ?"}`, true},
		{"blank question", "mentor_question", `{"question":"  "}`, false},
		{"unknown kind", "deploy", `{}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := validateParams(tc.kind, json.RawMessage(tc.params))
			if (err == nil) != tc.ok {
				t.Fatalf("ok=%v, got err=%v", tc.ok, err)
			}
		})
	}
}

func TestAccessLevelAllowed(t *testing.T) {
	for _, role := range []model.UserRole{model.RoleIntern, model.RoleBuddy, model.RoleTeamlead} {
		for _, level := range []string{"read", "write"} {
			if !accessLevelAllowed(role, level) {
				t.Errorf("%s must be allowed %s", role, level)
			}
		}
		if accessLevelAllowed(role, "admin") {
			t.Errorf("%s must not get admin through a request", role)
		}
	}
	if accessLevelAllowed(model.UserRole("guest"), "read") {
		t.Error("unknown role must not get access")
	}
}

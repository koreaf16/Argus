package tui

import (
	"testing"

	"github.com/koreaf16/argus/internal/services/workspace"
)

func TestSubmitServerFormAllowsRegisteredPassword(t *testing.T) {
	reg := workspace.NewRegistry("")
	if err := reg.Add(workspace.ServerEntry{
		Alias: "sandbox-server",
		Kind:  workspace.ServerKindSSH,
		Host:  "192.168.0.130",
		Port:  22,
		User:  "sandbox",
		Auth:  workspace.ServerAuth{AllowPassword: true},
	}); err != nil {
		t.Fatalf("add server: %v", err)
	}

	response := make(chan serverFormResult, 1)
	m := uiModel{
		app: &app{cfg: Config{Workspace: workspace.NewManager(reg, nil)}},
		modal: modalState{
			Kind:        modalServerForm,
			ServerFormC: response,
			ServerForm: &serverFormState{
				EditAlias:          "sandbox-server",
				Alias:              "sandbox-server",
				Host:               "192.168.0.130",
				PortStr:            "22",
				User:               "sandbox",
				AuthMode:           serverFormAuthPassword,
				PasswordRegistered: true,
				AllowElevation:     true,
				ElevationMethod:    "sudo",
			},
		},
	}

	m.submitServerForm()

	select {
	case got := <-response:
		if got.Err != nil {
			t.Fatalf("submit error: %v", got.Err)
		}
		if got.Result.Password != "" {
			t.Fatalf("registered password should not be re-emitted, got %q", got.Result.Password)
		}
		if !got.Result.Entry.Auth.AllowPassword {
			t.Fatalf("password auth should stay enabled: %+v", got.Result.Entry.Auth)
		}
		if got.Result.Entry.Alias != "sandbox-server" {
			t.Fatalf("alias = %q, want sandbox-server", got.Result.Entry.Alias)
		}
	default:
		t.Fatal("submit did not return a server form result")
	}
}

func TestSubmitServerFormRequiresPasswordWhenUnregistered(t *testing.T) {
	response := make(chan serverFormResult, 1)
	sf := &serverFormState{
		Alias:    "new-server",
		Host:     "192.168.0.130",
		PortStr:  "22",
		User:     "sandbox",
		AuthMode: serverFormAuthPassword,
	}
	m := uiModel{
		app: &app{},
		modal: modalState{
			Kind:        modalServerForm,
			ServerFormC: response,
			ServerForm:  sf,
		},
	}

	m.submitServerForm()

	if sf.ErrorMsg != "Password is required for password auth" {
		t.Fatalf("error = %q, want password requirement", sf.ErrorMsg)
	}
	if sf.ErrorField != sfPassword || sf.FocusIdx != sfPassword {
		t.Fatalf("error field/focus = %v/%v, want password field", sf.ErrorField, sf.FocusIdx)
	}
	select {
	case got := <-response:
		t.Fatalf("unexpected form result: %+v", got)
	default:
	}
}

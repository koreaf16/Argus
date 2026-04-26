package agent

import (
	"context"
	"testing"
	"time"

	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/llm"
	"github.com/yourorg/infractl/internal/store"
)

type fakeServerStore struct {
	servers []store.Server
}

func (f fakeServerStore) List(ctx context.Context) ([]store.Server, error) {
	return append([]store.Server(nil), f.servers...), nil
}

func (f fakeServerStore) Get(ctx context.Context, name string) (store.Server, error) {
	for _, srv := range f.servers {
		if srv.Name == name {
			return srv, nil
		}
	}
	return store.Server{}, context.Canceled
}

func (f fakeServerStore) Add(ctx context.Context, server store.Server) error    { return nil }
func (f fakeServerStore) Update(ctx context.Context, server store.Server) error { return nil }
func (f fakeServerStore) Remove(ctx context.Context, name string) error         { return nil }
func (f fakeServerStore) Close() error                                          { return nil }

type fakeIdleLLMClient struct {
	chatCalled bool
	resp       llm.Response
}

func (f *fakeIdleLLMClient) Chat(_ context.Context, _ []llm.Message, _ []llm.ToolDef, _ interface{}, opts ...llm.CallOption) (llm.Response, error) {
	f.chatCalled = true
	return f.resp, nil
}

func (f *fakeIdleLLMClient) ChatStream(_ context.Context, _ []llm.Message, _ []llm.ToolDef, _ interface{}, _ func(string), _ func(string), opts ...llm.CallOption) (llm.Response, error) {
	f.chatCalled = true
	return f.resp, nil
}

func TestResolveStoredPasswordByTargetName(t *testing.T) {
	st := fakeServerStore{
		servers: []store.Server{
			{
				Name:       "sandbox",
				Host:       "192.168.0.130",
				User:       "sandbox",
				AuthType:   store.AuthTypePassword,
				Credential: "secret123",
			},
		},
	}

	req := IdleInputRequest{
		Target:    "sandbox",
		LastLines: []string{"sandbox@192.168.0.130's password:"},
	}

	got, ok := resolveStoredPassword(context.Background(), st, req)
	if !ok {
		t.Fatal("expected stored password match")
	}
	if got != "secret123" {
		t.Fatalf("expected stored password, got %q", got)
	}
}

func TestResolveStoredPasswordByHostPrompt(t *testing.T) {
	st := fakeServerStore{
		servers: []store.Server{
			{
				Name:       "db",
				Host:       "db.example.com",
				User:       "oracle",
				AuthType:   store.AuthTypePassword,
				Credential: "dbpass",
			},
		},
	}

	req := IdleInputRequest{
		Target:    "",
		LastLines: []string{"oracle@db.example.com's password:"},
	}

	got, ok := resolveStoredPassword(context.Background(), st, req)
	if !ok {
		t.Fatal("expected stored password match")
	}
	if got != "dbpass" {
		t.Fatalf("expected stored password, got %q", got)
	}
}

func TestSmartIdleInputHandler_ExitsDBREPLViaLLM(t *testing.T) {
	tests := []struct {
		name      string
		command   string
		lines     []string
		llmResp   string
		wantInput string
	}{
		{
			name: "sqlplus", command: "sqlplus / as sysdba",
			lines:     []string{"Connected to:", "SQL>"},
			llmResp:   `{"input":"exit","close_stdin":false,"abort":false}`,
			wantInput: "exit",
		},
		{
			name: "mysql", command: "mysql -uroot",
			lines:     []string{"Welcome to the MySQL monitor.", "mysql>"},
			llmResp:   `{"input":"exit","close_stdin":false,"abort":false}`,
			wantInput: "exit",
		},
		{
			name: "psql readonly", command: "psql appdb",
			lines:     []string{"psql (16.0)", "appdb=>"},
			llmResp:   `{"input":"\\q","close_stdin":false,"abort":false}`,
			wantInput: `\q`,
		},
		{
			name: "psql admin", command: "psql postgres",
			lines:     []string{"psql (16.0)", "appdb=#"},
			llmResp:   `{"input":"\\q","close_stdin":false,"abort":false}`,
			wantInput: `\q`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := &fakeIdleLLMClient{
				resp: llm.Response{Content: tc.llmResp},
			}
			handler := NewSmartIdleInputHandler(client)

			resp, err := handler.RequestIdleInput(context.Background(), IdleInputRequest{
				ToolName:  "shell_exec",
				Target:    "oracle-db",
				Command:   tc.command,
				LastLines: tc.lines,
			})
			if err != nil {
				t.Fatalf("RequestIdleInput() error = %v", err)
			}
			if resp.Abort {
				t.Fatalf("expected exit command, got abort: %#v", resp)
			}
			if resp.Input != tc.wantInput {
				t.Fatalf("expected input %q, got %q", tc.wantInput, resp.Input)
			}
			if !client.chatCalled {
				t.Fatal("expected LLM to be called for DB REPL prompt")
			}
		})
	}
}

func TestSmartIdleInputHandler_AbortsOnUnknownREPLPrompt(t *testing.T) {
	client := &fakeIdleLLMClient{
		resp: llm.Response{Content: `{"input":"","abort":true}`},
	}
	handler := NewSmartIdleInputHandler(client)

	resp, err := handler.RequestIdleInput(context.Background(), IdleInputRequest{
		ToolName:  "shell_exec",
		Target:    "oracle-db",
		Command:   "dbcli --interactive",
		LastLines: []string{"inventory=>"},
	})
	if err != nil {
		t.Fatalf("RequestIdleInput() error = %v", err)
	}
	if !resp.Abort {
		t.Fatalf("expected abort for unknown REPL prompt, got %#v", resp)
	}
	if !client.chatCalled {
		t.Fatal("expected fallback LLM path for unknown REPL prompt")
	}
}

func TestSmartIdleInputHandler_SkipsLLMWhenNoPromptCue(t *testing.T) {
	client := &fakeIdleLLMClient{
		resp: llm.Response{Content: `{"input":"A","close_stdin":false,"abort":false}`},
	}
	handler := NewSmartIdleInputHandler(client)

	resp, err := handler.RequestIdleInput(context.Background(), IdleInputRequest{
		ToolName:  "shell_exec",
		Target:    "sandbox",
		Command:   "unzip -q db.zip",
		LastLines: []string{},
	})
	if err != nil {
		t.Fatalf("RequestIdleInput() error = %v", err)
	}
	if resp.Abort || resp.CloseStdin || resp.Input != "" {
		t.Fatalf("expected no-op response, got %#v", resp)
	}
	if client.chatCalled {
		t.Fatal("expected no LLM call when prompt cue is absent")
	}
}

func TestHasInteractivePromptCue(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		want  bool
	}{
		{
			name:  "unzip replace prompt",
			lines: []string{"replace OPatch/emdpatch.pl? [y]es, [n]o, [A]ll, [N]one, [r]ename:"},
			want:  true,
		},
		{
			name:  "ssh host key prompt",
			lines: []string{"Are you sure you want to continue connecting (yes/no/[fingerprint])?"},
			want:  true,
		},
		{
			name:  "normal output",
			lines: []string{"total 64", "-rw-r--r-- 1 oracle oinstall 1234 file.txt"},
			want:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := hasInteractivePromptCue(tc.lines)
			if got != tc.want {
				t.Fatalf("hasInteractivePromptCue() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestShouldInjectEmptyResponse(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		want  bool
	}{
		{name: "press enter", lines: []string{"Press Enter to continue"}, want: true},
		{name: "more prompt", lines: []string{"-- More --"}, want: true},
		{name: "regular line", lines: []string{"extracting: file.txt"}, want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldInjectEmptyResponse(tc.lines)
			if got != tc.want {
				t.Fatalf("shouldInjectEmptyResponse() = %v, want %v", got, tc.want)
			}
		})
	}
}

type fakePersistentStreamExecutor struct {
	sessions   []executor.SessionInfo
	listCalled bool
}

func (f *fakePersistentStreamExecutor) Execute(context.Context, string) (executor.ExecResult, error) {
	return executor.ExecResult{}, nil
}

func (f *fakePersistentStreamExecutor) ExecuteStream(context.Context, string, func(string)) (executor.ExecResult, error) {
	return executor.ExecResult{}, nil
}

func (f *fakePersistentStreamExecutor) InjectStdin(string) error { return nil }
func (f *fakePersistentStreamExecutor) Target() string           { return "db" }
func (f *fakePersistentStreamExecutor) Host() string             { return "db" }

func (f *fakePersistentStreamExecutor) SessionExecute(context.Context, string, string, time.Duration, func([]string) (string, bool)) (executor.ShellRunResult, error) {
	return executor.ShellRunResult{}, nil
}

func (f *fakePersistentStreamExecutor) SessionElevate(context.Context, string, string, time.Duration, func([]string) (string, bool)) (executor.ShellRunResult, error) {
	return executor.ShellRunResult{}, nil
}

func (f *fakePersistentStreamExecutor) SessionClose(context.Context, string) error {
	return nil
}

func (f *fakePersistentStreamExecutor) SessionList(context.Context) ([]executor.SessionInfo, error) {
	f.listCalled = true
	return f.sessions, nil
}

func TestIdleDetectExecutorPreservesPersistentSessions(t *testing.T) {
	original := &fakePersistentStreamExecutor{
		sessions: []executor.SessionInfo{
			{SessionID: "root", CurrentUser: "root", Alive: true},
		},
	}

	wrapped := wrapWithIdleDetect(original, nil, "shell_exec", "db")
	pse, ok := wrapped.(executor.PersistentSessionExecutor)
	if !ok {
		t.Fatal("expected idle wrapper to preserve PersistentSessionExecutor")
	}

	sessions, err := pse.SessionList(context.Background())
	if err != nil {
		t.Fatalf("SessionList() error = %v", err)
	}
	if !original.listCalled {
		t.Fatal("expected SessionList to be forwarded to original executor")
	}
	if len(sessions) != 1 || sessions[0].SessionID != "root" {
		t.Fatalf("unexpected sessions: %#v", sessions)
	}
}

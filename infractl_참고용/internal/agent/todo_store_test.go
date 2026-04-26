package agent

import (
	"testing"

	todoagent "github.com/yourorg/infractl/internal/agent/todo"
	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/tools"
)

func TestNewUsesRegisteredTodoWriteStoreForEnforcer(t *testing.T) {
	todoStore := todoagent.NewStore()
	tracker := todoagent.NewTracker(todoStore)
	registry := tools.NewRegistry()
	if err := registry.Register(&todoagent.WriteTool{Tracker: tracker}); err != nil {
		t.Fatalf("register TodoWrite: %v", err)
	}

	agent := New(noopLLMClient{}, registry, executor.NewManager(noopAgentExecutor{}), noopAgentEventHandler{}, nil)
	if agent.todoStore != todoStore {
		t.Fatal("agent todo store should be the store used by the registered TodoWrite tool")
	}

	if err := tracker.Add(todoagent.TodoItem{ID: "1", Content: "Upload Oracle installer", Status: todoagent.StatusPending}); err != nil {
		t.Fatalf("add todo: %v", err)
	}
	if ok, reason := agent.todoEnforcer.Enforce("file_transfer", false); !ok {
		t.Fatalf("file_transfer should be allowed after TodoWrite; reason=%q", reason)
	}
}

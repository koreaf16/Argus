package query

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/koreaf16/argus/internal/hooks"
	"github.com/koreaf16/argus/internal/services/llm"
	"github.com/koreaf16/argus/internal/services/workspace"
	"github.com/koreaf16/argus/internal/types"
	ctxpkg "github.com/koreaf16/argus/internal/context"
)

func (e *Engine) SetLLM(client llm.LLM) {
	e.mu.Lock()
	e.llm = client
	e.mu.Unlock()
}

func (e *Engine) SetConfig(cfg Config) {
	e.mu.Lock()
	e.cfg = applyPersistenceDefaults(cfg)
	e.mu.Unlock()
}

func (e *Engine) SetDeps(ctx context.Context, deps Deps) {
	var (
		sessionStart       *TraceRecord
		watcherToStop      *hooks.FileWatcher
		dispatcher         *hooks.HookDispatcher
		dispatchSession    bool
		sessionStartInput  hooks.HookInput
		shouldStartWatcher bool
	)

	e.mu.Lock()
	watcherToStop = e.fileWatcher
	e.fileWatcher = nil
	e.deps = deps

	// ArtifactStore initialization (requires baseDir and sessionID).
	if deps.BaseDir != "" && deps.SessionID != "" {
		store := ctxpkg.NewArtifactStore(deps.BaseDir, deps.SessionID)
		_ = store.Bootstrap()
		e.artStore = store
		e.distiller = ctxpkg.NewDistiller(store, e.artMF, e.makeSummarizeFn())
	} else {
		e.artStore = nil
		e.distiller = nil
	}

	// Emit debug session start once per session id.
	if deps.AIDebug.Enabled && deps.AIDebug.Emitter != nil {
		sessionID := strings.TrimSpace(deps.SessionID)
		if sessionID != "" && sessionID != e.debugSessionID {
			e.debugSessionID = sessionID
			e.debugSessionStarted = true
			e.debugSeq++
			sessionStart = &TraceRecord{
				TS:        time.Now().UTC().Format(time.RFC3339Nano),
				Type:      "session.start",
				SessionID: deps.SessionID,
				Seq:       e.debugSeq,
				Data: map[string]any{
					"working_dir": deps.WorkingDir,
				},
			}
		}
	} else {
		e.debugSessionStarted = false
		e.debugSessionID = ""
	}

	dispatcher = e.hookDispatcher
	if dispatcher != nil && !dispatcher.IsEmpty() {
		key := strings.TrimSpace(deps.SessionID) + "|" + strings.TrimSpace(deps.WorkingDir)
		if key != e.hookSessionStartKey {
			e.hookSessionStartKey = key
			dispatchSession = true
			sessionStartInput = hooks.HookInput{
				SessionID:  deps.SessionID,
				WorkingDir: deps.WorkingDir,
			}
		}
		shouldStartWatcher = true
	}

	// Restore ephemeral workspaces from state
	if deps.Workspace != nil && e.state != nil {
		if servers := e.state.EphemeralServers(); len(servers) > 0 {
			reg := deps.Workspace.Registry()
			for _, s := range servers {
				if data, err := json.Marshal(s); err == nil {
					var entry workspace.ServerEntry
					if err := json.Unmarshal(data, &entry); err == nil {
						entry.IsEphemeral = true
						_ = reg.Add(entry)
					}
				}
			}
		}
	}

	// Deps can affect permission decisions; drop stale cache.
	e.permissionRulesCacheValid = false
	e.permissionRulesCache = nil
	e.permissionRulesCachedAt = time.Time{}
	e.mu.Unlock()

	if watcherToStop != nil {
		watcherToStop.Stop()
	}
	if sessionStart != nil {
		deps.AIDebug.Emitter.Emit(*sessionStart)
	}
	if dispatcher == nil || dispatcher.IsEmpty() {
		return
	}
	if dispatchSession {
		go dispatcher.Dispatch(ctx, types.HookEventSessionStart, sessionStartInput)
	}
	if shouldStartWatcher {
		if fw, err := hooks.NewFileWatcher(ctx, dispatcher); err == nil && fw != nil {
			e.mu.Lock()
			e.fileWatcher = fw
			e.mu.Unlock()
			fw.Start()
		}
	}
}

// SetHookDispatcher configures the settings-based hook dispatcher.
func (e *Engine) SetHookDispatcher(d *hooks.HookDispatcher) {
	var watcherToStop *hooks.FileWatcher
	e.mu.Lock()
	watcherToStop = e.fileWatcher
	e.fileWatcher = nil
	e.hookDispatcher = d
	e.hookSessionStartKey = ""
	e.mu.Unlock()
	if watcherToStop != nil {
		watcherToStop.Stop()
	}
}

func (e *Engine) AddStopHook(hook StopHook) {
	if hook == nil {
		return
	}
	e.mu.Lock()
	e.stopHooks = append(e.stopHooks, hook)
	e.mu.Unlock()
}

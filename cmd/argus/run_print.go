package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/koreaf16/argus/internal/presentation"
	"github.com/koreaf16/argus/internal/query"
	"github.com/koreaf16/argus/internal/services/workspace"
)

func runPrint(flags *parsedFlags) error {
	ctx := context.Background()
	engine, _, config, err := bootstrap(ctx, flags)
	if err != nil {
		return err
	}

	reader := bufio.NewReader(os.Stdin)

	if config.Workspace != nil {
		config.Workspace.SetPromptFunc(func(alias, kind, prompt string) (string, error) {
			pwPrompt := workspace.FormatPasswordPrompt(config.Workspace.Registry(), alias, kind, prompt)
			fmt.Fprint(os.Stdout, pwPrompt+" ")
			pw, err := reader.ReadString('\n')
			if err != nil {
				return "", err
			}
			return strings.TrimSpace(pw), nil
		})
	}

	renderer := presentation.NewTextRenderer(os.Stdout, flags.aidebug)
	engine.SetApprovalGate(query.ApprovalGate{
		Prompt: func(ctx context.Context, toolName string, input json.RawMessage) (bool, error) {
			renderer.Emit(presentation.Event{
				Kind:     presentation.EventApprovalRequest,
				ToolName: toolName,
				Input:    presentation.FormatInputJSON(input),
			})
			ok := false
			var err error
			if flags.autoOK {
				ok = true
			} else {
				ok, err = promptToolApproval(ctx, toolName, input)
			}
			decision := "deny"
			if ok {
				decision = "allow"
			}
			renderer.Emit(presentation.Event{
				Kind:     presentation.EventApprovalDecision,
				Decision: decision,
			})
			return ok, err
		},
	})
	renderer.Emit(presentation.Event{
		Kind:   presentation.EventState,
		Footer: presentation.BuildFooterState(config.State, config.WorkDir),
	})

	// Single prompt execution
	renderer.Emit(presentation.Event{
		Kind: presentation.EventUser,
		Text: flags.print,
	})
	events, err := engine.SubmitMessage(ctx, flags.print)
	if err != nil {
		return err
	}

	for event := range events {
		if pe, ok := presentation.FromUIEvent(event); ok {
			renderer.Emit(pe)
		}
		if event.Kind == query.UIEventPasswordPrompt {
			pwPrompt := strings.TrimSpace(event.Prompt)
			if pwPrompt == "" {
				pwPrompt = "Password:"
			}
			if !strings.HasSuffix(pwPrompt, " ") {
				pwPrompt += " "
			}
			fmt.Fprint(os.Stdout, pwPrompt)
			pw, readErr := reader.ReadString('\n')
			if event.PasswordResponse != nil {
				if readErr != nil {
					event.PasswordResponse <- ""
				} else {
					event.PasswordResponse <- strings.TrimSpace(pw)
				}
			}
		}
		if event.Kind == query.UIEventAskUserPrompt {
			resp := promptAskUserFromReader(reader, os.Stdout, event.Question)
			if event.AskUserResponse != nil {
				event.AskUserResponse <- resp
			}
		}
		if event.Kind == query.UIEventAskUserBatchPrompt {
			resp := promptAskUserBatchFromReader(reader, os.Stdout, event.Questions)
			if event.AskUserBatchResponse != nil {
				event.AskUserBatchResponse <- resp
			}
		}
		if event.Kind == query.UIEventError && event.Err != nil {
			renderer.Close()
			return event.Err
		}
	}
	renderer.Close()
	if err := persistSessionSnapshot(config.Memory, config.State, engine.Messages()); err != nil {
		return err
	}
	printSessionID(os.Stderr, config.State)
	return nil
}

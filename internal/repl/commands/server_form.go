package commands

import (
	"fmt"
	"strings"

	"github.com/koreaf16/argus/internal/services/workspace"
)

// storeElevationPassword persists the elevation password from a ServerFormResult.
// The credential is stored per-target-user using the TargetPasswords map.
// Errors are printed as warnings rather than returned, matching the SSH password behaviour above.
func storeElevationPassword(ctx CommandContext, res ServerFormResult) {
	if ctx.Credentials == nil {
		return
	}
	entry := res.Entry

	// Handle wildcard target if mode is password and no targets are defined but allowed.
	// We use the legacy ElevationPassword field for this fallback.
	if len(res.TargetPasswords) == 0 && entry.Elevation.Allowed && entry.Elevation.Mode == "password" {
		pw := strings.TrimSpace(res.ElevationPassword)
		if pw != "" {
			if err := ctx.Credentials.SetPasswordForTarget(entry.Alias, "sudo", "", pw); err != nil {
				fmt.Fprintf(ctx.Stdout, "warning: could not save elevation credential: %v\n", err)
			}
		}
	} else {
		// Save per-target passwords
		for targetUser, pw := range res.TargetPasswords {
			pw = strings.TrimSpace(pw)
			if pw != "" {
				if err := ctx.Credentials.SetPasswordForTarget(entry.Alias, "sudo", targetUser, pw); err != nil {
					fmt.Fprintf(ctx.Stdout, "warning: could not save elevation credential for %q: %v\n", targetUser, err)
				}
			}
		}
	}

	if err := ctx.Credentials.Save(); err != nil {
		fmt.Fprintf(ctx.Stdout, "warning: could not write credential store: %v\n", err)
	}
}

func saveWorkAccounts(ctx CommandContext, res ServerFormResult) {
	for _, account := range res.WorkAccounts {
		account.ParentAlias = strings.TrimSpace(account.ParentAlias)
		if account.ParentAlias == "" {
			account.ParentAlias = res.Entry.Alias
		}
		if existing, ok := ctx.Workspace.Registry().Get(account.Alias); ok {
			if existing.Kind == workspace.ServerKindAccount && existing.ParentAlias == account.ParentAlias {
				_ = ctx.Workspace.Registry().Remove(account.Alias)
			}
		}
		if err := ctx.Workspace.Registry().Add(account); err != nil {
			fmt.Fprintf(ctx.Stdout, "warning: could not add work account %s: %v\n", account.Alias, err)
		}
	}
}

// handleServerEditForm invokes the interactive TUI server-edit form for an existing server.
func handleServerEditForm(ctx CommandContext, alias string) error {
	if ctx.ServerFormPrompt == nil {
		return fmt.Errorf("interactive form requires TUI mode")
	}
	if _, ok := ctx.Workspace.Registry().Get(alias); !ok {
		return fmt.Errorf("server not found: %s", alias)
	}

	res, err := ctx.ServerFormPrompt(ctx.Context, ServerFormRequest{EditAlias: alias})
	if err != nil {
		if strings.Contains(err.Error(), "canceled") {
			fmt.Fprintln(ctx.Stdout, "server edit: canceled")
			return nil
		}
		return err
	}

	// Remove old entry and add updated one (no Update method on Registry).
	_ = ctx.Workspace.Registry().Remove(alias)
	if err := ctx.Workspace.Registry().Add(res.Entry); err != nil {
		return fmt.Errorf("update server: %w", err)
	}
	saveWorkAccounts(ctx, res)
	if err := ctx.Workspace.Registry().Save(); err != nil {
		return fmt.Errorf("save server registry: %w", err)
	}

	if strings.TrimSpace(res.Password) != "" {
		if res.SavePassword && ctx.Credentials != nil {
			if err := ctx.Credentials.SetPassword(res.Entry.Alias, "ssh", res.Password); err != nil {
				fmt.Fprintf(ctx.Stdout, "warning: could not save credential: %v\n", err)
			} else if err := ctx.Credentials.Save(); err != nil {
				fmt.Fprintf(ctx.Stdout, "warning: could not write credential store: %v\n", err)
			}
		} else {
			ctx.Workspace.SetPassword(res.Entry.Alias, "ssh", res.Password)
		}
	}
	storeElevationPassword(ctx, res)

	e := res.Entry.Elevation
	if e.Allowed {
		targets := strings.Join(e.TargetUsers, ", ")
		if targets == "" {
			targets = "any"
		}
		fmt.Fprintf(ctx.Stdout, "updated server: %s (%s@%s:%d)\n  elevation: ENABLED  mode=%s  targets=%s\n",
			res.Entry.Alias, res.Entry.User, res.Entry.Host, res.Entry.Port, e.Mode, targets)
	} else {
		fmt.Fprintf(ctx.Stdout, "updated server: %s (%s@%s:%d)\n  elevation: DISABLED\n",
			res.Entry.Alias, res.Entry.User, res.Entry.Host, res.Entry.Port)
	}
	return nil
}

// handleServerAddForm invokes the interactive TUI server-registration form.
// Falls back with an error message if the context does not support TUI prompts.
func handleServerAddForm(ctx CommandContext) error {
	if ctx.ServerFormPrompt == nil {
		return fmt.Errorf(
			"interactive form requires TUI mode; use /server add <alias> <user>@<host> [--port <port>] [--password] instead",
		)
	}

	res, err := ctx.ServerFormPrompt(ctx.Context, ServerFormRequest{})
	if err != nil {
		if strings.Contains(err.Error(), "canceled") {
			fmt.Fprintln(ctx.Stdout, "server add: canceled")
			return nil
		}
		return err
	}

	if err := ctx.Workspace.Registry().Add(res.Entry); err != nil {
		return fmt.Errorf("add server: %w", err)
	}
	saveWorkAccounts(ctx, res)
	if err := ctx.Workspace.Registry().Save(); err != nil {
		return fmt.Errorf("save server registry: %w", err)
	}

	if strings.TrimSpace(res.Password) != "" {
		if res.SavePassword && ctx.Credentials != nil {
			if err := ctx.Credentials.SetPassword(res.Entry.Alias, "ssh", res.Password); err != nil {
				fmt.Fprintf(ctx.Stdout, "warning: could not save credential: %v\n", err)
			} else if err := ctx.Credentials.Save(); err != nil {
				fmt.Fprintf(ctx.Stdout, "warning: could not write credential store: %v\n", err)
			}
		} else {
			if res.SavePassword {
				fmt.Fprintln(ctx.Stdout, "warning: credential store unavailable; password cached in memory only")
			}
			ctx.Workspace.SetPassword(res.Entry.Alias, "ssh", res.Password)
		}
	}
	storeElevationPassword(ctx, res)

	e := res.Entry.Elevation
	if e.Allowed {
		targets := strings.Join(e.TargetUsers, ", ")
		if targets == "" {
			targets = "any"
		}
		fmt.Fprintf(ctx.Stdout, "added server: %s (%s@%s:%d)\n  elevation: ENABLED  mode=%s  targets=%s\n",
			res.Entry.Alias, res.Entry.User, res.Entry.Host, res.Entry.Port, e.Mode, targets)
	} else {
		fmt.Fprintf(ctx.Stdout, "added server: %s (%s@%s:%d)\n  elevation: DISABLED — sudo/su refused until /server edit %s\n",
			res.Entry.Alias, res.Entry.User, res.Entry.Host, res.Entry.Port, res.Entry.Alias)
	}
	return nil
}

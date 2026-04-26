package commands

import (
	"fmt"
	"strings"
)

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

	fmt.Fprintf(ctx.Stdout, "updated server: %s (%s@%s:%d)\n",
		res.Entry.Alias, res.Entry.User, res.Entry.Host, res.Entry.Port)
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

	fmt.Fprintf(ctx.Stdout, "added server: %s (%s@%s:%d)\n",
		res.Entry.Alias, res.Entry.User, res.Entry.Host, res.Entry.Port)
	return nil
}

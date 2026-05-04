package servercopy

import (
	tool "github.com/koreaf16/argus/internal/tools"
	"github.com/koreaf16/argus/internal/tools/promptkit"
)

// GetSystemPromptGuide implements tools.SystemPromptContributor.
func (t *ServerCopyTool) GetSystemPromptGuide(ctx tool.Context) string {
	s := promptkit.New()

	s.WhenToUse(
		"Copy one file between registered workspaces, including local <-> SSH transfers.",
		"Upload a local file to a remote server, or download a remote file to local.",
		"Move file bytes between workspaces without asking the user to run scp manually.",
	)

	s.WhenNotToUse(
		"Reading or inspecting file contents; use file_read, fs_list, glob, or bash instead.",
		"Copying within the same workspace; use bash cp/mv when appropriate.",
		"Copying a directory directly; archive it first or use a shell command because server_copy copies files.",
	)

	s.UsageNotes(
		"Use src and dst with explicit aliases whenever possible: alias:path.",
		"For a local file path, use the local alias, for example local:docs/README.md.",
		"For a remote path, use the workspace alias, for example sandbox-server:/tmp/README.md.",
		"If the user asks to copy a known local file to a remote workspace, verify the source with glob or fs_list, then call server_copy immediately.",
		"When multiple remote workspaces exist, both endpoints must be explicit via alias:path or src_server/dst_server.",
	)

	s.Examples(
		`Local to remote: src="local:docs/README.md", dst="sandbox-server:/tmp/README.md"`,
		`Remote to local: src="sandbox-server:/var/log/app.log", dst="local:logs/app.log"`,
	)

	s.Parameters(
		"`src`: source file endpoint, preferably alias:path.",
		"`dst`: destination file endpoint, preferably alias:path.",
		"`src_server` and `src_path`: source alias/path split into separate fields.",
		"`dst_server` and `dst_path`: destination alias/path split into separate fields.",
		"`overwrite`: whether to overwrite the destination file; defaults to true.",
	)

	return s.Build()
}

// Package routing
// File: routing.go
// Description: Deterministic routing of shell commands to the appropriate service user.
// Responsibility: RouteUser — map tool name + command args to asUser (oracle, postgres, root, etc.)

package routing

import (
	"regexp"
	"strings"

	"github.com/yourorg/infractl/internal/store"
)

// oracleCmdRe matches Oracle-specific CLI tools that must run as the oracle user.
var oracleCmdRe = regexp.MustCompile(`(?i)\b(sqlplus|opatch|runInstaller|dbca|lsnrctl|dgmgrl|tnsping|adrci|crsctl|srvctl|rman|impdp|expdp)\b`)

// oracleArchiveRe matches archive extractions targeting Oracle installation paths.
var oracleArchiveRe = regexp.MustCompile(`(?i)\b(unzip|tar)\b.*(LINUX\.X64|db_home|p\d{7,}[_\.]|ORA[A-Z0-9]+|oracle[_\-\.]|dbhome)`)

// oracleEnvVarRe matches Oracle environment variable references that require oracle user context.
// These always route to oracle because the env vars won't expand without oracle's environment.
var oracleEnvVarRe = regexp.MustCompile(`(\$ORACLE_BASE|\$ORACLE_HOME)`)

// oracleHardPathRe matches hardcoded Oracle filesystem paths (not env vars).
var oracleHardPathRe = regexp.MustCompile(`(/home/oracle|/u01/app/oracle|/opt/oracle)`)

// oraclePathMutationRe matches mutation operations that require oracle user ownership.
// Read-only commands (id, df, cat, ls, stat) that merely reference oracle paths do NOT need oracle routing.
var oraclePathMutationRe = regexp.MustCompile(`(?i)\b(mkdir|cp|mv|chmod|chown|touch|rsync|tee|dd|install)\b|\bunzip\b|\btar\b`)

// postgresRe matches PostgreSQL CLI tools.
var postgresRe = regexp.MustCompile(`(?i)\b(psql|pg_ctl|pg_dump|pg_restore|pg_basebackup|initdb|postgres)\b`)

// rootCmdRe matches system commands that require root / sudo privilege.
var rootCmdRe = regexp.MustCompile(`(?i)\b(systemctl|firewall-cmd|setenforce|sysctl\s+-w|mount\b|umount\b|modprobe|setcap|useradd|usermod|groupadd)\b`)

// rootPathRe matches paths that imply root ownership.
var rootPathRe = regexp.MustCompile(`^(sudo\s|/etc/|/var/log/|/proc/sys/)`)

// RouteResult carries the resolved user and the reason for the decision.
type RouteResult struct {
	AsUser string
	Reason string
}

// RouteUser determines which service user a tool call should run as.
// Priority: explicit as_user arg > oracle tools/paths > postgres tools > root tools > default.
// Returns empty AsUser when the default SSH user should be used.
func RouteUser(toolName string, args map[string]any, server *store.Server) RouteResult {
	// Rule 1: explicit override in args.
	if u, _ := args["as_user"].(string); strings.TrimSpace(u) != "" {
		return RouteResult{AsUser: strings.TrimSpace(u), Reason: "explicit as_user"}
	}

	tool := strings.ToLower(strings.TrimSpace(toolName))
	if tool != "shell_exec" {
		return routeNonShellTool(tool, args, server)
	}

	cmd, _ := args["command"].(string)
	if cmd == "" {
		return RouteResult{}
	}

	// Rule 2: Oracle CLI tools.
	if oracleCmdRe.MatchString(cmd) {
		return RouteResult{AsUser: resolveServiceUser(server, "oracle"), Reason: "oracle CLI command"}
	}

	// Rule 3: Archive extraction targeting Oracle installation paths.
	if oracleArchiveRe.MatchString(cmd) {
		return RouteResult{AsUser: resolveServiceUser(server, "oracle"), Reason: "oracle archive extraction"}
	}

	// Rule 4a: Oracle env var references always route to oracle (env won't expand without oracle context).
	if oracleEnvVarRe.MatchString(cmd) {
		return RouteResult{AsUser: resolveServiceUser(server, "oracle"), Reason: "oracle env var reference"}
	}

	// Rule 4b: Hardcoded oracle path references only route when the command also mutates files.
	// Read-only commands (id, df, cat, ls) referencing /home/oracle do NOT need oracle user.
	if oracleHardPathRe.MatchString(cmd) && oraclePathMutationRe.MatchString(cmd) {
		return RouteResult{AsUser: resolveServiceUser(server, "oracle"), Reason: "oracle path reference"}
	}

	// Rule 5: PostgreSQL tools.
	if postgresRe.MatchString(cmd) {
		return RouteResult{AsUser: resolveServiceUser(server, "postgres"), Reason: "postgres CLI command"}
	}

	// Rule 6: Root-level system commands.
	if rootCmdRe.MatchString(cmd) || rootPathRe.MatchString(strings.TrimSpace(cmd)) {
		return RouteResult{AsUser: "root", Reason: "root system command"}
	}

	// Rule 7: Ambiguous — if exactly one service user is registered, default to it.
	if server != nil && len(server.ServiceUsers) == 1 {
		return RouteResult{AsUser: server.ServiceUsers[0].Name, Reason: "single registered service user"}
	}

	// Default: use the SSH connection user.
	return RouteResult{}
}

func routeNonShellTool(toolName string, args map[string]any, server *store.Server) RouteResult {
	switch toolName {
	case "file_transfer":
		action, _ := args["action"].(string)
		if !strings.EqualFold(strings.TrimSpace(action), "upload") {
			return RouteResult{}
		}
		remotePath, _ := args["remote_path"].(string)
		if oracleHardPathRe.MatchString(remotePath) {
			return RouteResult{AsUser: resolveServiceUser(server, "oracle"), Reason: "oracle file upload path"}
		}
	case "file_write", "file_line_edit":
		path, _ := args["path"].(string)
		if oracleHardPathRe.MatchString(path) {
			return RouteResult{AsUser: resolveServiceUser(server, "oracle"), Reason: "oracle file mutation path"}
		}
	}
	return RouteResult{}
}

// resolveServiceUser returns the canonical user name if it's registered (and verified)
// on the server, otherwise returns the well-known name as-is.
func resolveServiceUser(server *store.Server, name string) string {
	if server == nil {
		return name
	}
	for _, u := range server.ServiceUsers {
		if strings.EqualFold(u.Name, name) && u.Verified {
			return u.Name
		}
	}
	return name
}

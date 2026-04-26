// Package generic
// File: connector.go
// Description: 학습된 명령어를 기반으로 동적 도구 생성 및 SQL 오류 시 스키마 자동 검증
// Responsibility: LLM이 생성한 명령어 스펙(JSON)을 tools.Tool 인스턴스로 변환하고,
//                 SQL 쿼리 명령의 컬럼/테이블 오류 시 schema_check_command로 자동 스키마 조회

package generic

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"strings"

	conn "github.com/yourorg/infractl/internal/connector"
	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/tools"
)

var invalidToolName = regexp.MustCompile(`[^a-z0-9_]+`)

// GenericConnector는 학습된 명령어 스펙이나 합성된 프로필에서 도구를 다시 생성한다.
type GenericConnector struct {
	info        conn.ServiceInfo
	creds       conn.Credentials
	learnedCmds []conn.LearnedCommand
	status      conn.ConnectorStatus
	names       []string
}

// New는 GenericConnector를 생성한다.
func New() *GenericConnector {
	return &GenericConnector{status: conn.StatusDisconnected}
}

func (c *GenericConnector) ServiceType() string          { return "generic" }
func (c *GenericConnector) Status() conn.ConnectorStatus { return c.status }
func (c *GenericConnector) ToolNames() []string          { return c.names }

// GenerateTools는 학습된 명령어 정보로 도구를 생성한다.
func (c *GenericConnector) GenerateTools(info conn.ServiceInfo, creds conn.Credentials) []tools.Tool {
	c.info = info
	c.creds = creds
	c.status = conn.StatusConnected

	if len(c.learnedCmds) == 0 {
		c.learnedCmds = loadLearnedCommands(info.Details["commands_json"])
	}

	serviceName := sanitizeToolName(info.Name)
	if serviceName == "" {
		serviceName = "svc"
	}
	prefix := "generic_" + serviceName

	var toolList []tools.Tool
	for _, cmd := range c.learnedCmds {
		if strings.TrimSpace(cmd.Command) == "" {
			continue
		}
		cmd := cmd
		params := buildParameters(cmd)

		t := conn.NewGeneratedTool(
			prefix+"."+sanitizeToolName(cmd.Name),
			defaultString(cmd.Description, fmt.Sprintf("Run learned command %s", cmd.Name)),
			params,
			cmd.ReadOnly,
			&c.status,
			func(ctx context.Context, args map[string]any, exec executor.Executor) (string, error) {
				merged := buildTemplateValues(c.info, c.creds, args)

				rendered := renderCommandTemplate(cmd.Command, merged)
				res, err := exec.Execute(ctx, rendered)
				if err != nil {
					return fmt.Sprintf("Execution failed: %s", err), nil
				}

				output := formatOutput(res.Stdout, res.Stderr, res.ExitCode)

				// SQL 쿼리 명령에서 컬럼/테이블 오류 발생 시 자동 스키마 조회
				if cmd.IsSQLQuery && cmd.SchemaCheckCommand != "" && conn.HasQueryError(output, 0) {
					sql, _ := args["sql"].(string)
					schemaNote := runSchemaCheck(ctx, cmd, sql, merged, exec)
					if schemaNote != "" {
						output += "\n\n[자동 스키마 조회 결과]\n" + schemaNote
					}
				}

				return output, nil
			},
		)
		toolList = append(toolList, t)
	}

	c.names = make([]string, len(toolList))
	for i, t := range toolList {
		c.names[i] = t.Name()
	}
	return toolList
}

// runSchemaCheck는 SQL에서 테이블명을 추출하고 schema_check_command를 실행한다.
func runSchemaCheck(ctx context.Context, cmd conn.LearnedCommand, sql string, baseValues map[string]string, exec executor.Executor) string {
	tables := conn.ExtractTableNames(sql)
	if len(tables) == 0 {
		return ""
	}

	var sb strings.Builder
	for _, table := range tables {
		values := make(map[string]string, len(baseValues)+1)
		maps.Copy(values, baseValues)
		values["table_name"] = table

		rendered := renderCommandTemplate(cmd.SchemaCheckCommand, values)
		res, err := exec.Execute(ctx, rendered)
		if err != nil {
			continue
		}
		out := strings.TrimSpace(res.Stdout)
		if out == "" {
			fmt.Fprintf(&sb, "── %s: 테이블/뷰를 찾을 수 없습니다.\n", table)
			continue
		}
		fmt.Fprintf(&sb, "── %s 컬럼 목록:\n", table)
		for line := range strings.SplitSeq(out, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				sb.WriteString("  " + line + "\n")
			}
		}
	}
	return sb.String()
}

func loadLearnedCommands(raw string) []conn.LearnedCommand {
	if strings.TrimSpace(raw) == "" {
		return nil
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil
	}

	var commands []conn.LearnedCommand
	for name, value := range payload {
		switch v := value.(type) {
		case string:
			commands = append(commands, conn.LearnedCommand{
				Name:        name,
				Description: fmt.Sprintf("Run learned command %s", name),
				Command:     v,
				ReadOnly:    inferReadOnly(v),
			})
		case map[string]any:
			command, _ := v["command"].(string)
			description, _ := v["description"].(string)
			readOnly, ok := v["read_only"].(bool)
			if !ok {
				readOnly = inferReadOnly(command)
			}
			backupCmd, _ := v["backup_command"].(string)
			schemaCheckCmd, _ := v["schema_check_command"].(string)
			isSQLQuery, _ := v["is_sql_query"].(bool)
			toolName := name
			if explicit, ok := v["name"].(string); ok && explicit != "" {
				toolName = explicit
			}

			params := map[string]any{}
			if rawParams, ok := v["parameters"].(map[string]any); ok {
				params = rawParams
			}

			var required []string
			if rawReq, ok := v["required"].([]any); ok {
				for _, item := range rawReq {
					if s, ok := item.(string); ok && s != "" {
						required = append(required, s)
					}
				}
			}

			commands = append(commands, conn.LearnedCommand{
				Name:               toolName,
				Description:        description,
				Command:            command,
				ReadOnly:           readOnly,
				BackupCommand:      backupCmd,
				Parameters:         params,
				Required:           required,
				IsSQLQuery:         isSQLQuery,
				SchemaCheckCommand: schemaCheckCmd,
			})
		}
	}
	return commands
}

func buildParameters(cmd conn.LearnedCommand) map[string]any {
	params := map[string]any{
		"target": targetParam(),
	}

	maps.Copy(params, cmd.Parameters)

	required := append([]string{}, cmd.Required...)
	if len(required) > 0 {
		return map[string]any{
			"type":       "object",
			"properties": params,
			"required":   required,
		}
	}
	return map[string]any{
		"type":       "object",
		"properties": params,
	}
}

func buildTemplateValues(info conn.ServiceInfo, creds conn.Credentials, args map[string]any) map[string]string {
	values := map[string]string{
		"server_name":  info.ServerName,
		"service_type": info.ServiceType,
		"service_name": info.Name,
		"host":         info.Details["host"],
		"port":         fmt.Sprintf("%d", info.Port),
		"username":     creds.Username,
		"password":     creds.Password,
		"role":         creds.Role,
	}
	maps.Copy(values, info.Details)
	for k, v := range args {
		if k == "target" {
			continue
		}
		switch tv := v.(type) {
		case string:
			values[k] = tv
		default:
			values[k] = fmt.Sprintf("%v", v)
		}
	}
	return values
}

func renderCommandTemplate(template string, values map[string]string) string {
	return templatePlaceholderRe.ReplaceAllStringFunc(template, func(match string) string {
		inner := strings.TrimSpace(match[2 : len(match)-2])
		parts := strings.SplitN(inner, ":", 2)
		var mode, key string
		if len(parts) == 1 {
			key = strings.TrimSpace(parts[0])
		} else {
			mode = strings.TrimSpace(parts[0])
			key = strings.TrimSpace(parts[1])
		}
		val := values[key]
		switch mode {
		case "raw", "q":
			return val
		default:
			return shellQuote(val)
		}
	})
}

var templatePlaceholderRe = regexp.MustCompile(`\{\{([^}]+)\}\}`)

func inferReadOnly(command string) bool {
	lower := strings.ToLower(command)
	mutating := []string{" rm ", " mv ", " cp ", " sed -i", " systemctl restart", " systemctl stop", " kill ", "truncate", " drop ", " delete ", " update ", " insert ", " create ", " alter "}
	padded := " " + lower + " "
	for _, marker := range mutating {
		if strings.Contains(padded, marker) {
			return false
		}
	}
	return true
}

func sanitizeToolName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = invalidToolName.ReplaceAllString(name, "_")
	name = strings.Trim(name, "_")
	if name == "" {
		return "cmd"
	}
	return name
}

func defaultString(v string, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func targetParam() map[string]any {
	return map[string]any{
		"type":        "string",
		"description": "Target server name.",
	}
}

func formatOutput(stdout, stderr string, exitCode int) string {
	out := strings.TrimSpace(stdout)
	if exitCode != 0 && stderr != "" {
		return fmt.Sprintf("[ExitCode: %d]\n%s\n[Stderr]\n%s", exitCode, out, strings.TrimSpace(stderr))
	}
	if out == "" {
		return "(no output)"
	}
	return out
}

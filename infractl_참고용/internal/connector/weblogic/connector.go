// Package weblogic
// File: connector.go
// Description: WebLogic WAS 커넥터 — 프로세스 레벨 모니터링 도구 생성
// Responsibility: WebLogic 프로세스 기반 status/logs/thread_dump 도구 생성

package weblogic

import (
	"context"
	"fmt"
	"strings"

	conn "github.com/yourorg/infractl/internal/connector"
	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/tools"
)

// WebLogicConnector는 WebLogic WAS 전용 커넥터이다.
type WebLogicConnector struct {
	info   conn.ServiceInfo
	status conn.ConnectorStatus
	names  []string
}

// New는 WebLogicConnector를 생성한다.
func New() *WebLogicConnector {
	return &WebLogicConnector{status: conn.StatusDisconnected}
}

func (c *WebLogicConnector) ServiceType() string          { return "weblogic" }
func (c *WebLogicConnector) Status() conn.ConnectorStatus { return c.status }
func (c *WebLogicConnector) ToolNames() []string          { return c.names }

// GenerateTools는 WebLogic 프로세스 기반 모니터링 도구를 생성한다.
func (c *WebLogicConnector) GenerateTools(info conn.ServiceInfo, creds conn.Credentials) []tools.Tool {
	c.info = info
	c.status = conn.StatusConnected

	port := info.Port
	if port == 0 {
		port = 7001
	}
	prefix := fmt.Sprintf("weblogic_%d", port)

	pid := info.Details["pid"]
	domainHome := info.Details["domain_home"]

	toolList := []tools.Tool{
		c.makeStatusTool(prefix, pid),
		c.makeLogsTool(prefix, domainHome),
		c.makeThreadDumpTool(prefix, pid),
		c.makeRestartTool(prefix, pid, domainHome),
	}

	c.names = make([]string, len(toolList))
	for i, t := range toolList {
		c.names[i] = t.Name()
	}
	return toolList
}

func (c *WebLogicConnector) makeStatusTool(prefix, pid string) tools.Tool {
	return conn.NewConnectorTool(
		prefix+".status",
		fmt.Sprintf("WebLogic(%s) 프로세스 상태 — CPU/메모리/포트 확인.", prefix),
		map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{"target": targetParam()},
		},
		true, &c.status,
		func(ctx context.Context, args map[string]interface{}, exec executor.Executor) (string, error) {
			var cmd string
			if pid != "" {
				cmd = fmt.Sprintf("ps -p %s -o pid,user,%%cpu,%%mem,vsz,rss,stat,start,time,args 2>/dev/null && ss -tlnp | grep %s", pid, pid)
			} else {
				cmd = `ps aux | grep -i '[wW]eblogic\.Server' | head -5`
			}
			res, err := exec.Execute(ctx, cmd)
			if err != nil {
				return fmt.Sprintf("실행 오류: %s", err), nil
			}
			return formatOutput(res.Stdout, res.Stderr, res.ExitCode), nil
		},
	)
}

func (c *WebLogicConnector) makeLogsTool(prefix, domainHome string) tools.Tool {
	return conn.NewConnectorTool(
		prefix+".logs",
		fmt.Sprintf("WebLogic(%s) 서버 로그 조회.", prefix),
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"lines":  map[string]interface{}{"type": "integer", "description": "출력 줄 수 (기본 100)"},
				"target": targetParam(),
			},
		},
		true, &c.status,
		func(ctx context.Context, args map[string]interface{}, exec executor.Executor) (string, error) {
			lines := 100
			if v, ok := args["lines"].(float64); ok {
				lines = int(v)
			}
			var cmd string
			if domainHome != "" {
				cmd = fmt.Sprintf("find %s -name '*.log' | head -3 | xargs -I{} tail -n %d {}", domainHome, lines)
			} else {
				cmd = fmt.Sprintf("find /opt /u01 -name 'AdminServer.log' 2>/dev/null | head -1 | xargs -I{} tail -n %d {}", lines)
			}
			res, err := exec.Execute(ctx, cmd)
			if err != nil {
				return fmt.Sprintf("실행 오류: %s", err), nil
			}
			return formatOutput(res.Stdout, res.Stderr, res.ExitCode), nil
		},
	)
}

func (c *WebLogicConnector) makeThreadDumpTool(prefix, pid string) tools.Tool {
	return conn.NewConnectorTool(
		prefix+".thread_dump",
		fmt.Sprintf("WebLogic(%s) JVM 스레드 덤프 (jstack).", prefix),
		map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{"target": targetParam()},
		},
		true, &c.status,
		func(ctx context.Context, args map[string]interface{}, exec executor.Executor) (string, error) {
			var cmd string
			if pid != "" {
				cmd = fmt.Sprintf("jstack %s 2>/dev/null | head -200", pid)
			} else {
				cmd = `WPID=$(ps aux | grep -i '[wW]eblogic\.Server' | awk '{print $2}' | head -1); [ -n "$WPID" ] && jstack $WPID 2>/dev/null | head -200 || echo 'WebLogic PID를 찾을 수 없습니다'`
			}
			res, err := exec.Execute(ctx, cmd)
			if err != nil {
				return fmt.Sprintf("실행 오류: %s", err), nil
			}
			return formatOutput(res.Stdout, res.Stderr, res.ExitCode), nil
		},
	)
}

func (c *WebLogicConnector) makeRestartTool(prefix, pid, domainHome string) tools.Tool {
	return conn.NewConnectorTool(
		prefix+".restart",
		fmt.Sprintf("WebLogic(%s) 재시작. 재시작 전 현재 상태를 캡처하고 롤백 명령을 기록한다.", prefix),
		map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{"target": targetParam()},
		},
		false, &c.status,
		func(ctx context.Context, args map[string]interface{}, exec executor.Executor) (string, error) {
			// 재시작 전 상태 캡처
			var statusCmd string
			if pid != "" {
				statusCmd = fmt.Sprintf("ps -p %s -o pid,user,%%cpu,%%mem,stat 2>/dev/null | tail -1", pid)
			} else {
				statusCmd = `ps aux | grep -i '[wW]eblogic\.Server' | head -1`
			}
			stateRes, _ := exec.Execute(ctx, statusCmd)
			preState := strings.TrimSpace(stateRes.Stdout)

			// 재시작 명령 결정
			var restartCmd, startCmd string
			if domainHome != "" {
				restartCmd = fmt.Sprintf("%s/bin/stopWebLogic.sh && sleep 5 && %s/bin/startWebLogic.sh", domainHome, domainHome)
				startCmd = domainHome + "/bin/startWebLogic.sh"
			} else {
				restartCmd = "systemctl restart weblogic"
				startCmd = "systemctl start weblogic"
			}

			// rollback_command을 args에 주입 (체크포인트용)
			args["rollback_command"] = startCmd
			args["checkpoint_description"] = fmt.Sprintf("WebLogic(%s) 재시작 전 상태", prefix)

			res, err := exec.Execute(ctx, restartCmd)
			if err != nil {
				return fmt.Sprintf("재시작 오류: %s", err), nil
			}
			result := formatOutput(res.Stdout, res.Stderr, res.ExitCode)
			if preState != "" {
				result = fmt.Sprintf("[재시작 전 상태: %s]\n%s", preState, result)
			}
			return result, nil
		},
	)
}

func targetParam() map[string]interface{} {
	return map[string]interface{}{
		"type":        "string",
		"description": "대상 서버 이름. 생략 시 커넥터 등록 서버 사용.",
	}
}

func formatOutput(stdout, stderr string, exitCode int) string {
	out := strings.TrimSpace(stdout)
	if exitCode != 0 && stderr != "" {
		return fmt.Sprintf("[ExitCode: %d]\n%s\n[Stderr]\n%s", exitCode, out, strings.TrimSpace(stderr))
	}
	if out == "" {
		return "(결과 없음)"
	}
	return out
}

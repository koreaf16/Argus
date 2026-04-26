// Package tomcat
// File: connector.go
// Description: Tomcat WAS 커넥터 — 프로세스 레벨 모니터링 도구 생성
// Responsibility: Tomcat 접속 정보 없이 프로세스 기반 status/logs/thread_dump 도구 생성

package tomcat

import (
	"context"
	"fmt"
	"strings"

	conn "github.com/yourorg/infractl/internal/connector"
	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/tools"
)

// TomcatConnector는 Tomcat WAS 전용 커넥터이다.
// DB 크리덴셜 없이 프로세스 레벨에서 동작한다.
type TomcatConnector struct {
	info   conn.ServiceInfo
	status conn.ConnectorStatus
	names  []string
}

// New는 TomcatConnector를 생성한다.
func New() *TomcatConnector {
	return &TomcatConnector{status: conn.StatusDisconnected}
}

func (c *TomcatConnector) ServiceType() string          { return "tomcat" }
func (c *TomcatConnector) Status() conn.ConnectorStatus { return c.status }
func (c *TomcatConnector) ToolNames() []string          { return c.names }

// GenerateTools는 Tomcat 프로세스 기반 모니터링 도구를 생성한다.
func (c *TomcatConnector) GenerateTools(info conn.ServiceInfo, creds conn.Credentials) []tools.Tool {
	c.info = info
	c.status = conn.StatusConnected

	port := info.Port
	if port == 0 {
		port = 8080
	}
	prefix := fmt.Sprintf("tomcat_%d", port)

	pid := info.Details["pid"]
	catalinaHome := info.Details["catalina_home"]

	toolList := []tools.Tool{
		c.makeStatusTool(prefix, pid),
		c.makeLogsTool(prefix, catalinaHome),
		c.makeThreadDumpTool(prefix, pid),
		c.makeRestartTool(prefix, pid, catalinaHome),
	}

	c.names = make([]string, len(toolList))
	for i, t := range toolList {
		c.names[i] = t.Name()
	}
	return toolList
}

func (c *TomcatConnector) makeStatusTool(prefix, pid string) tools.Tool {
	return conn.NewConnectorTool(
		prefix+".status",
		fmt.Sprintf("Tomcat(%s) 프로세스 상태 — CPU/메모리/열린 포트 확인.", prefix),
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
				cmd = `ps aux | grep -i '[tc]atalina\|[jJ]ava.*[Tt]omcat' | head -5`
			}
			res, err := exec.Execute(ctx, cmd)
			if err != nil {
				return fmt.Sprintf("실행 오류: %s", err), nil
			}
			return formatOutput(res.Stdout, res.Stderr, res.ExitCode), nil
		},
	)
}

func (c *TomcatConnector) makeLogsTool(prefix, catalinaHome string) tools.Tool {
	return conn.NewConnectorTool(
		prefix+".logs",
		fmt.Sprintf("Tomcat(%s) catalina.out 최근 로그 조회.", prefix),
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
			var logPath string
			if catalinaHome != "" {
				logPath = catalinaHome + "/logs/catalina.out"
			} else {
				logPath = "$(find /opt /usr/local -name 'catalina.out' 2>/dev/null | head -1)"
			}
			cmd := fmt.Sprintf("tail -n %d %s 2>/dev/null || echo 'catalina.out 파일을 찾을 수 없습니다'", lines, logPath)
			res, err := exec.Execute(ctx, cmd)
			if err != nil {
				return fmt.Sprintf("실행 오류: %s", err), nil
			}
			return formatOutput(res.Stdout, res.Stderr, res.ExitCode), nil
		},
	)
}

func (c *TomcatConnector) makeThreadDumpTool(prefix, pid string) tools.Tool {
	return conn.NewConnectorTool(
		prefix+".thread_dump",
		fmt.Sprintf("Tomcat(%s) JVM 스레드 덤프 출력 (jstack).", prefix),
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
				cmd = `TPID=$(ps aux | grep -i '[tc]atalina' | awk '{print $2}' | head -1); [ -n "$TPID" ] && jstack $TPID 2>/dev/null | head -200 || echo 'Tomcat PID를 찾을 수 없습니다'`
			}
			res, err := exec.Execute(ctx, cmd)
			if err != nil {
				return fmt.Sprintf("실행 오류: %s", err), nil
			}
			return formatOutput(res.Stdout, res.Stderr, res.ExitCode), nil
		},
	)
}

func (c *TomcatConnector) makeRestartTool(prefix, pid, catalinaHome string) tools.Tool {
	return conn.NewConnectorTool(
		prefix+".restart",
		fmt.Sprintf("Tomcat(%s) 재시작. 재시작 전 현재 상태를 캡처하고 롤백 명령을 기록한다.", prefix),
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
				statusCmd = `ps aux | grep -i '[tc]atalina' | head -1`
			}
			stateRes, _ := exec.Execute(ctx, statusCmd)
			preState := strings.TrimSpace(stateRes.Stdout)

			// 재시작 명령 결정
			var restartCmd, startCmd string
			if catalinaHome != "" {
				restartCmd = fmt.Sprintf("%s/bin/shutdown.sh && sleep 3 && %s/bin/startup.sh", catalinaHome, catalinaHome)
				startCmd = catalinaHome + "/bin/startup.sh"
			} else {
				restartCmd = "systemctl restart tomcat"
				startCmd = "systemctl start tomcat"
			}

			// rollback_command을 args에 주입 (체크포인트용)
			args["rollback_command"] = startCmd
			args["checkpoint_description"] = fmt.Sprintf("Tomcat(%s) 재시작 전 상태", prefix)

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

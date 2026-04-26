// Package main
// File: audit_cmd.go
// Description: infractl audit 서브커맨드 구현 (list / show)
// Responsibility: 감사 이력 조회 CLI 출력

package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/yourorg/infractl/internal/store"
)

// runAudit는 `infractl audit <subcommand>` 진입점이다.
func runAudit(args []string) error {
	sub := "list"
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "list":
		return runAuditList(args[1:])
	case "show":
		if len(args) < 2 {
			return fmt.Errorf("usage: infractl audit show <run_id>")
		}
		return runAuditShow(args[1])
	case "export":
		if len(args) < 2 {
			return fmt.Errorf("usage: infractl audit export <run_id> [--md <file>]")
		}
		outFile := ""
		for i := 2; i+1 < len(args); i++ {
			if args[i] == "--md" {
				outFile = args[i+1]
			}
		}
		return runAuditExport(args[1], outFile)
	default:
		fmt.Println("audit 서브커맨드: list [--server X] [--since Nd], show <run_id>, export <run_id> [--md <file>]")
		return nil
	}
}

func runAuditList(args []string) error {
	ctx := context.Background()
	d, err := buildDeps(ctx)
	if err != nil {
		return fmt.Errorf("build deps: %w", err)
	}
	defer d.serverStore.Close()

	serverFilter := ""
	sinceDays := 7
	for i := 0; i+1 < len(args); i++ {
		switch args[i] {
		case "--server":
			serverFilter = args[i+1]
		case "--since":
			fmt.Sscanf(args[i+1], "%dd", &sinceDays)
		}
	}
	since := time.Now().AddDate(0, 0, -sinceDays)

	summaries, err := d.auditStore.ListAuditRunSummaries(ctx, serverFilter, since, 50)
	if err != nil {
		return fmt.Errorf("list audit runs: %w", err)
	}

	if len(summaries) == 0 {
		fmt.Println("감사 이력이 없습니다.")
		return nil
	}

	fmt.Printf("%-8s  %-9s  %-30s  %-10s  %-12s  %s\n", "ID", "상태", "제목", "서버", "경과", "진행")
	fmt.Println(strings.Repeat("─", 84))
	for _, rs := range summaries {
		statusTag := formatStatus(rs.Status)
		title := rs.Title
		if title == "" {
			title = "(제목 없음)"
		}
		progress := fmt.Sprintf("%d/%d step", rs.StepDone, rs.StepTotal)
		shortID := rs.ID
		if len(shortID) > 8 {
			shortID = shortID[:8]
		}
		fmt.Printf("%-8s  %-9s  %-30s  %-10s  %-12s  %s\n",
			shortID,
			statusTag,
			padRight(title, 30),
			padRight(rs.ServerName, 10),
			elapsedStr(rs.StartedAt),
			progress,
		)
	}
	return nil
}

func runAuditShow(runID string) error {
	ctx := context.Background()
	d, err := buildDeps(ctx)
	if err != nil {
		return fmt.Errorf("build deps: %w", err)
	}
	defer d.serverStore.Close()

	detail, err := d.auditStore.DescribeAuditRun(ctx, runID)
	if err != nil {
		return fmt.Errorf("describe run: %w", err)
	}

	run := detail.Run
	fmt.Printf("# %s\n", run.Title)
	fmt.Printf("ID: %s | 서버: %s | 상태: %s\n", run.ID, run.ServerName, run.Status)
	fmt.Printf("시작: %s\n", run.StartedAt.Format("2006-01-02 15:04:05"))
	if run.EndedAt != nil {
		fmt.Printf("종료: %s\n", run.EndedAt.Format("2006-01-02 15:04:05"))
	}
	if run.SummaryMD != "" {
		fmt.Printf("\n## 요약\n%s\n", run.SummaryMD)
	}

	if detail.ActivePlan != nil {
		p := detail.ActivePlan
		fmt.Printf("\n## 계획서 (revision %d)\n", p.Revision)
		if p.GoalMD != "" {
			fmt.Printf("### 목표\n%s\n", p.GoalMD)
		}
		if p.PlanMD != "" {
			fmt.Printf("### 계획\n%s\n", p.PlanMD)
		}
	}

	if len(detail.Steps) > 0 {
		fmt.Printf("\n## 실행 스텝\n")
		fmt.Printf("%-4s  %-8s  %s\n", "seq", "상태", "레이블")
		fmt.Println(strings.Repeat("─", 50))
		for _, st := range detail.Steps {
			fmt.Printf("%-4d  %-8s  %s\n", st.Seq, stepStatusIcon(st.Status), st.Label)
		}
	}
	return nil
}

func runAuditExport(runID, outFile string) error {
	ctx := context.Background()
	d, err := buildDeps(ctx)
	if err != nil {
		return fmt.Errorf("build deps: %w", err)
	}
	defer d.serverStore.Close()

	detail, err := d.auditStore.DescribeAuditRun(ctx, runID)
	if err != nil {
		return fmt.Errorf("describe run: %w", err)
	}

	var sb strings.Builder
	run := detail.Run
	title := run.Title
	if title == "" {
		title = "(제목 없음)"
	}
	sb.WriteString("# " + title + "\n\n")
	sb.WriteString(fmt.Sprintf("- **ID**: %s\n", run.ID))
	sb.WriteString(fmt.Sprintf("- **서버**: %s\n", run.ServerName))
	sb.WriteString(fmt.Sprintf("- **상태**: %s\n", run.Status))
	sb.WriteString(fmt.Sprintf("- **시작**: %s\n", run.StartedAt.Format("2006-01-02 15:04:05")))
	if run.EndedAt != nil {
		sb.WriteString(fmt.Sprintf("- **종료**: %s\n", run.EndedAt.Format("2006-01-02 15:04:05")))
	}
	if run.SummaryMD != "" {
		sb.WriteString("\n## 요약\n\n" + run.SummaryMD + "\n")
	}

	if detail.ActivePlan != nil {
		p := detail.ActivePlan
		sb.WriteString(fmt.Sprintf("\n## 계획서 (revision %d)\n\n", p.Revision))
		if p.GoalMD != "" {
			sb.WriteString("### 목표\n\n" + p.GoalMD + "\n")
		}
		if p.RationaleMD != "" {
			sb.WriteString("### 배경\n\n" + p.RationaleMD + "\n")
		}
		if p.PlanMD != "" {
			sb.WriteString("### 실행 계획\n\n" + p.PlanMD + "\n")
		}
		if p.RiskMD != "" {
			sb.WriteString("### 리스크 / 롤백\n\n" + p.RiskMD + "\n")
		}
	}

	if len(detail.Steps) > 0 {
		sb.WriteString("\n## 실행 스텝\n\n")
		sb.WriteString("| seq | 상태 | 레이블 |\n|-----|------|--------|\n")
		for _, st := range detail.Steps {
			sb.WriteString(fmt.Sprintf("| %d | %s | %s |\n", st.Seq, stepStatusIcon(st.Status), st.Label))
		}
	}
	sb.WriteString(fmt.Sprintf("\n---\n*generated by infractl audit export at %s*\n", time.Now().Format("2006-01-02 15:04")))

	content := sb.String()
	if outFile == "" {
		fmt.Print(content)
		return nil
	}
	if err := os.WriteFile(outFile, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write export file: %w", err)
	}
	fmt.Printf("보고서 저장: %s\n", outFile)
	return nil
}

func formatStatus(s store.AuditRunStatus) string {
	switch s {
	case store.AuditRunStatusPlanning:
		return "[planning]"
	case store.AuditRunStatusRunning:
		return "[running] "
	case store.AuditRunStatusSucceeded:
		return "[done]    "
	case store.AuditRunStatusFailed:
		return "[failed]  "
	case store.AuditRunStatusAborted:
		return "[aborted] "
	}
	return string(s)
}

func stepStatusIcon(s store.AuditStepStatus) string {
	switch s {
	case store.AuditStepStatusOK:
		return "ok      "
	case store.AuditStepStatusFail:
		return "FAIL    "
	case store.AuditStepStatusSkipped:
		return "skipped "
	case store.AuditStepStatusUnplanned:
		return "adhoc   "
	default:
		return string(s)
	}
}

func elapsedStr(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

func padRight(s string, n int) string {
	if len([]rune(s)) >= n {
		runes := []rune(s)
		return string(runes[:n])
	}
	return s + strings.Repeat(" ", n-len([]rune(s)))
}

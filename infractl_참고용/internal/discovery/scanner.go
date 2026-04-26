// Package discovery
// File: scanner.go
// Description: [TODO: Add description]
// Responsibility: [TODO: Add responsibility]

package discovery

import (
	"context"
	"encoding/csv"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/tools"
)

// Scanner performs process, port, and config-file discovery on a target host.
type Scanner struct {
	patterns []ServicePattern
}

// NewScanner creates a scanner with builtin service patterns.
func NewScanner() *Scanner {
	return &Scanner{patterns: BuiltinPatterns}
}

// Scan runs discovery on the supplied executor target in parallel.
func (s *Scanner) Scan(ctx context.Context, exec executor.Executor) (*ScanResult, error) {
	serverName := exec.Target()
	slog.Info("starting parallel service scan", "server", serverName)

	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		processes []processInfo
		ports     []portInfo
		others    []DiscoveredService
		errs      []error
	)

	// Task 1: Processes
	wg.Add(1)
	go func() {
		defer wg.Done()
		tools.EmitOutput(ctx, "task_start task=processes")
		slog.Info("task_start", "task", "processes")
		start := time.Now()
		p, err := s.scanProcesses(ctx, exec)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			errs = append(errs, fmt.Errorf("process scan: %w", err))
			tools.EmitOutput(ctx, fmt.Sprintf("task_fail task=processes dur=%s", time.Since(start)))
			slog.Info("task_fail", "task", "processes", "dur", time.Since(start))
		} else {
			processes = p
			tools.EmitOutput(ctx, fmt.Sprintf("task_done task=processes dur=%s count=%d", time.Since(start), len(p)))
			slog.Info("task_done", "task", "processes", "dur", time.Since(start), "count", len(p))
		}
	}()

	// Task 2: Ports
	wg.Add(1)
	go func() {
		defer wg.Done()
		tools.EmitOutput(ctx, "task_start task=ports")
		slog.Info("task_start", "task", "ports")
		start := time.Now()
		p, err := s.scanPorts(ctx, exec)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			errs = append(errs, fmt.Errorf("port scan: %w", err))
			tools.EmitOutput(ctx, fmt.Sprintf("task_fail task=ports dur=%s", time.Since(start)))
			slog.Info("task_fail", "task", "ports", "dur", time.Since(start))
		} else {
			ports = p
			tools.EmitOutput(ctx, fmt.Sprintf("task_done task=ports dur=%s count=%d", time.Since(start), len(p)))
			slog.Info("task_done", "task", "ports", "dur", time.Since(start), "count", len(p))
		}
	}()

	// Task 3: Systemd (Unix only)
	if executor.CommandPlatform(exec) != executor.PlatformWindows {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tools.EmitOutput(ctx, "task_start task=systemd")
			slog.Info("task_start", "task", "systemd")
			start := time.Now()
			svc, err := s.scanSystemd(ctx, exec)
			mu.Lock()
			defer mu.Unlock()
			if err == nil && len(svc) > 0 {
				others = append(others, svc...)
				tools.EmitOutput(ctx, fmt.Sprintf("task_done task=systemd dur=%s count=%d", time.Since(start), len(svc)))
				slog.Info("task_done", "task", "systemd", "dur", time.Since(start), "count", len(svc))
			} else {
				tools.EmitOutput(ctx, fmt.Sprintf("task_done task=systemd dur=%s count=0", time.Since(start)))
				slog.Info("task_done", "task", "systemd", "dur", time.Since(start), "count", 0)
			}
		}()

		// Task 4: Docker (Unix only)
		wg.Add(1)
		go func() {
			defer wg.Done()
			tools.EmitOutput(ctx, "task_start task=docker")
			slog.Info("task_start", "task", "docker")
			start := time.Now()
			svc, err := s.scanDocker(ctx, exec)
			mu.Lock()
			defer mu.Unlock()
			if err == nil && len(svc) > 0 {
				others = append(others, svc...)
				tools.EmitOutput(ctx, fmt.Sprintf("task_done task=docker dur=%s count=%d", time.Since(start), len(svc)))
				slog.Info("task_done", "task", "docker", "dur", time.Since(start), "count", len(svc))
			} else {
				tools.EmitOutput(ctx, fmt.Sprintf("task_done task=docker dur=%s count=0", time.Since(start)))
				slog.Info("task_done", "task", "docker", "dur", time.Since(start), "count", 0)
			}
		}()
	}

	wg.Wait()

	if len(errs) > 0 {
		slog.Warn("some scan tasks failed", "server", serverName, "errors", errs)
	}

	result := &ScanResult{
		ServerName: serverName,
		ScannedAt:  time.Now(),
	}

	// Pattern matching from processes & ports
	for _, pattern := range s.patterns {
		svc := s.matchPattern(ctx, exec, pattern, processes, ports)
		if svc == nil {
			continue
		}
		svc.ServerName = serverName
		result.Services = append(result.Services, *svc)
	}

	// Add services found via other methods (systemd, docker)
	for i := range others {
		others[i].ServerName = serverName
		result.Services = append(result.Services, others[i])
	}

	unknowns := s.findUnknown(processes, result.Services)
	result.Services = append(result.Services, unknowns...)

	// Oracle enrichment
	for i := range result.Services {
		svc := &result.Services[i]
		if svc.Type == ServiceOracle {
			enrichOracleService(ctx, exec, svc)
		}
	}

	slog.Info("scan complete", "server", serverName, "found", len(result.Services))
	return result, nil
}

func (s *Scanner) scanSystemd(ctx context.Context, exec executor.Executor) ([]DiscoveredService, error) {
	// Only list running services to avoid noise
	res, err := exec.Execute(ctx, "systemctl list-units --type=service --state=running --no-pager --no-legend 2>/dev/null")
	if err != nil || res.ExitCode != 0 {
		return nil, err
	}

	var services []DiscoveredService
	lines := strings.Split(res.Stdout, "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		// fields: [UNIT, LOAD, ACTIVE, SUB, DESCRIPTION...]
		name := strings.TrimSuffix(fields[0], ".service")
		services = append(services, DiscoveredService{
			Type:       ServiceSystemd,
			Name:       name,
			Confidence: 0.8,
			Level:      ConfidenceHigh,
			Details: map[string]string{
				"status":      fields[3],
				"description": strings.Join(fields[4:], " "),
			},
		})
	}
	return services, nil
}

func (s *Scanner) scanDocker(ctx context.Context, exec executor.Executor) ([]DiscoveredService, error) {
	// List running containers
	res, err := exec.Execute(ctx, "docker ps --format '{{.ID}}\t{{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}' 2>/dev/null")
	if err != nil || res.ExitCode != 0 {
		return nil, err
	}

	var services []DiscoveredService
	lines := strings.Split(res.Stdout, "\n")
	for _, line := range lines {
		fields := strings.Split(line, "\t")
		if len(fields) < 4 {
			continue
		}
		services = append(services, DiscoveredService{
			Type:       ServiceDocker,
			Name:       fields[1],
			Confidence: 0.9,
			Level:      ConfidenceHigh,
			Details: map[string]string{
				"container_id": fields[0],
				"image":        fields[2],
				"status":       fields[3],
				"ports":        fields[4],
			},
		})
	}
	return services, nil
}

type processInfo struct {
	PID  int
	User string
	Comm string
	Args string
}

type portInfo struct {
	Port    int
	Process string
}

func (s *Scanner) scanProcesses(ctx context.Context, exec executor.Executor) ([]processInfo, error) {
	switch executor.CommandPlatform(exec) {
	case executor.PlatformWindows:
		return s.scanWindowsProcesses(ctx, exec)
	default:
		return s.scanUnixProcesses(ctx, exec)
	}
}

func (s *Scanner) scanUnixProcesses(ctx context.Context, exec executor.Executor) ([]processInfo, error) {
	res, err := exec.Execute(ctx, "ps -eo pid,user,comm,args --no-headers 2>/dev/null || ps -eo pid,user,comm,args 2>/dev/null")
	if err != nil {
		return nil, fmt.Errorf("execute ps: %w", err)
	}
	if res.ExitCode != 0 {
		return nil, fmt.Errorf("ps exit %d: %s", res.ExitCode, res.Stderr)
	}

	var procs []processInfo
	for _, line := range strings.Split(res.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		p := parseProcessLine(line)
		if p != nil {
			procs = append(procs, *p)
		}
	}
	return procs, nil
}

func (s *Scanner) scanWindowsProcesses(ctx context.Context, exec executor.Executor) ([]processInfo, error) {
	cmd := executor.PowerShellCommand(exec, "Get-CimInstance Win32_Process | Select-Object ProcessId,Name,CommandLine | ConvertTo-Csv -NoTypeInformation")
	res, err := exec.Execute(ctx, cmd)
	if err != nil {
		return nil, fmt.Errorf("execute windows process scan: %w", err)
	}
	if res.ExitCode != 0 {
		return nil, fmt.Errorf("windows process scan exit %d: %s", res.ExitCode, res.Stderr)
	}
	return parseWindowsProcessCSV(res.Stdout), nil
}

func parseProcessLine(line string) *processInfo {
	fields := strings.Fields(line)
	if len(fields) < 4 {
		return nil
	}
	pid, err := strconv.Atoi(fields[0])
	if err != nil {
		return nil
	}
	return &processInfo{
		PID:  pid,
		User: fields[1],
		Comm: fields[2],
		Args: strings.Join(fields[3:], " "),
	}
}

func parseWindowsProcessCSV(output string) []processInfo {
	reader := csv.NewReader(strings.NewReader(strings.TrimSpace(output)))
	reader.FieldsPerRecord = -1

	records, err := reader.ReadAll()
	if err != nil || len(records) <= 1 {
		return nil
	}

	var procs []processInfo
	for _, row := range records[1:] {
		if len(row) < 3 {
			continue
		}
		pid, err := strconv.Atoi(strings.TrimSpace(row[0]))
		if err != nil {
			continue
		}
		comm := strings.TrimSpace(row[1])
		args := strings.TrimSpace(row[2])
		if args == "" {
			args = comm
		}
		procs = append(procs, processInfo{
			PID:  pid,
			Comm: comm,
			Args: args,
		})
	}
	return procs
}

func (s *Scanner) scanPorts(ctx context.Context, exec executor.Executor) ([]portInfo, error) {
	switch executor.CommandPlatform(exec) {
	case executor.PlatformWindows:
		return s.scanWindowsPorts(ctx, exec)
	default:
		return s.scanUnixPorts(ctx, exec)
	}
}

func (s *Scanner) scanUnixPorts(ctx context.Context, exec executor.Executor) ([]portInfo, error) {
	res, err := exec.Execute(ctx, "ss -tlnp 2>/dev/null || netstat -tlnp 2>/dev/null")
	if err != nil {
		return nil, fmt.Errorf("execute port scan: %w", err)
	}
	if res.ExitCode != 0 {
		return nil, fmt.Errorf("port scan exit %d: %s", res.ExitCode, res.Stderr)
	}
	return parsePortOutput(res.Stdout), nil
}

func (s *Scanner) scanWindowsPorts(ctx context.Context, exec executor.Executor) ([]portInfo, error) {
	res, err := exec.Execute(ctx, "netstat -ano -p tcp")
	if err != nil {
		return nil, fmt.Errorf("execute windows port scan: %w", err)
	}
	if res.ExitCode != 0 {
		return nil, fmt.Errorf("windows port scan exit %d: %s", res.ExitCode, res.Stderr)
	}
	return parsePortOutput(res.Stdout), nil
}

func parsePortOutput(output string) []portInfo {
	var ports []portInfo
	portRe := regexp.MustCompile(`:(\d+)\s`)

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "State") || strings.HasPrefix(line, "Active") {
			continue
		}
		upper := strings.ToUpper(line)
		if !strings.Contains(upper, "LISTEN") {
			continue
		}
		matches := portRe.FindStringSubmatch(line)
		if len(matches) < 2 {
			continue
		}
		port, err := strconv.Atoi(matches[1])
		if err != nil || port <= 0 {
			continue
		}
		ports = append(ports, portInfo{Port: port})
	}
	return ports
}

func (s *Scanner) matchPattern(ctx context.Context, exec executor.Executor, pattern ServicePattern, procs []processInfo, ports []portInfo) *DiscoveredService {
	re, err := regexp.Compile(pattern.ProcessRegex)
	if err != nil {
		slog.Warn("invalid pattern regex", "type", pattern.Type, "err", err)
		return nil
	}

	var confidence float64
	svc := &DiscoveredService{
		Type:    pattern.Type,
		Details: make(map[string]string),
	}

	for _, p := range procs {
		if m := re.FindStringSubmatch(p.Args); m != nil {
			confidence += WeightProcess
			svc.PID = p.PID
			svc.User = p.User
			if len(m) > 1 && m[1] != "" {
				svc.Name = m[1]
			}
			svc.Details["comm"] = p.Comm
			break
		}
	}

	for _, port := range ports {
		for _, defaultPort := range pattern.DefaultPorts {
			if port.Port == defaultPort {
				confidence += WeightPort
				svc.Port = port.Port
				break
			}
		}
	}

	for _, cfgFile := range pattern.ConfigFiles {
		res, err := exec.Execute(ctx, buildConfigFileCheckCommand(exec, cfgFile))
		if err == nil && strings.Contains(res.Stdout, "exists") {
			confidence += WeightConfigFile
			svc.Details["config_file"] = cfgFile
			break
		}
	}

	if confidence == 0 {
		return nil
	}

	svc.Confidence = confidence
	svc.Level = CalcLevel(confidence)
	return svc
}

func buildConfigFileCheckCommand(exec executor.Executor, path string) string {
	switch executor.CommandPlatform(exec) {
	case executor.PlatformWindows:
		return executor.PowerShellCommand(exec, fmt.Sprintf("if (Test-Path -LiteralPath %s) { 'exists' }", executor.QuotePowerShell(path)))
	default:
		return fmt.Sprintf("test -f %s && echo exists", executor.QuotePOSIX(path))
	}
}

func (s *Scanner) findUnknown(procs []processInfo, known []DiscoveredService) []DiscoveredService {
	knownPIDs := make(map[int]bool, len(known))
	for _, k := range known {
		knownPIDs[k.PID] = true
	}

	highResRe := regexp.MustCompile(`(?i)^(java(\.exe)?|python[23]?(\.exe)?|node(\.exe)?|ruby(\.exe)?|perl(\.exe)?)$`)
	var unknowns []DiscoveredService

	for _, p := range procs {
		if knownPIDs[p.PID] {
			continue
		}
		if !highResRe.MatchString(p.Comm) {
			continue
		}
		unknowns = append(unknowns, DiscoveredService{
			Type:       ServiceUnknown,
			PID:        p.PID,
			User:       p.User,
			Confidence: 0.1,
			Level:      ConfidenceLow,
			Details: map[string]string{
				"comm": p.Comm,
				"args": truncate(p.Args, 120),
			},
		})
	}
	return unknowns
}

// ScanWeb runs web-specific discovery (servers, SSL, vhosts) in parallel.
func (s *Scanner) ScanWeb(ctx context.Context, exec executor.Executor) (*ScanResult, error) {
	serverName := exec.Target()
	slog.Info("starting parallel web server scan", "server", serverName)

	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		processes []processInfo
		ports     []portInfo
		others    []DiscoveredService
		errs      []error
	)

	// Task 1: Web Processes (Shared logic)
	wg.Add(1)
	go func() {
		defer wg.Done()
		tools.EmitOutput(ctx, "task_start task=web_servers")
		slog.Info("task_start", "task", "web_servers")
		start := time.Now()
		p, err := s.scanProcesses(ctx, exec)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			errs = append(errs, err)
			tools.EmitOutput(ctx, fmt.Sprintf("task_fail task=web_servers dur=%s", time.Since(start)))
			slog.Info("task_fail", "task", "web_servers", "dur", time.Since(start))
		} else {
			processes = p
			tools.EmitOutput(ctx, fmt.Sprintf("task_done task=web_servers dur=%s count=%d", time.Since(start), len(p)))
			slog.Info("task_done", "task", "web_servers", "dur", time.Since(start), "count", len(p))
		}
	}()

	// Task 2: Web Ports
	wg.Add(1)
	go func() {
		defer wg.Done()
		tools.EmitOutput(ctx, "task_start task=web_ports")
		slog.Info("task_start", "task", "web_ports")
		start := time.Now()
		p, err := s.scanPorts(ctx, exec)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			errs = append(errs, err)
			tools.EmitOutput(ctx, fmt.Sprintf("task_fail task=web_ports dur=%s", time.Since(start)))
			slog.Info("task_fail", "task", "web_ports", "dur", time.Since(start))
		} else {
			ports = p
			tools.EmitOutput(ctx, fmt.Sprintf("task_done task=web_ports dur=%s count=%d", time.Since(start), len(p)))
			slog.Info("task_done", "task", "web_ports", "dur", time.Since(start), "count", len(p))
		}
	}()

	// Task 3: SSL Certificates
	wg.Add(1)
	go func() {
		defer wg.Done()
		tools.EmitOutput(ctx, "task_start task=ssl_certs")
		slog.Info("task_start", "task", "ssl_certs")
		start := time.Now()
		svc, err := s.scanSSL(ctx, exec)
		mu.Lock()
		defer mu.Unlock()
		if err == nil && len(svc) > 0 {
			others = append(others, svc...)
		}
		tools.EmitOutput(ctx, fmt.Sprintf("task_done task=ssl_certs dur=%s count=%d", time.Since(start), len(svc)))
		slog.Info("task_done", "task", "ssl_certs", "dur", time.Since(start), "count", len(svc))
	}()

	// Task 4: Virtual Hosts
	wg.Add(1)
	go func() {
		defer wg.Done()
		tools.EmitOutput(ctx, "task_start task=vhosts")
		slog.Info("task_start", "task", "vhosts")
		start := time.Now()
		svc, err := s.scanVHosts(ctx, exec)
		mu.Lock()
		defer mu.Unlock()
		if err == nil && len(svc) > 0 {
			others = append(others, svc...)
		}
		tools.EmitOutput(ctx, fmt.Sprintf("task_done task=vhosts dur=%s count=%d", time.Since(start), len(svc)))
		slog.Info("task_done", "task", "vhosts", "dur", time.Since(start), "count", len(svc))
	}()

	wg.Wait()

	result := &ScanResult{
		ServerName: serverName,
		ScannedAt:  time.Now(),
	}

	// Match web patterns
	webPatterns := []ServiceType{ServiceNginx, ServiceApache, ServiceHAProxy, ServiceTraefik}
	for _, pattern := range s.patterns {
		isWeb := false
		for _, wt := range webPatterns {
			if pattern.Type == wt {
				isWeb = true
				break
			}
		}
		if !isWeb {
			continue
		}

		svc := s.matchPattern(ctx, exec, pattern, processes, ports)
		if svc != nil {
			svc.ServerName = serverName
			result.Services = append(result.Services, *svc)
		}
	}

	// Add SSL & VHosts
	for i := range others {
		others[i].ServerName = serverName
		result.Services = append(result.Services, others[i])
	}

	return result, nil
}

func (s *Scanner) scanSSL(ctx context.Context, exec executor.Executor) ([]DiscoveredService, error) {
	// Look for common SSL certificate paths and certbot artifacts
	res, err := exec.Execute(ctx, "ls /etc/letsencrypt/live/ 2>/dev/null || find /etc/ssl/certs -maxdepth 1 -type f 2>/dev/null | head -n 5")
	if err != nil || res.ExitCode != 0 {
		return nil, nil
	}

	var svcs []DiscoveredService
	lines := strings.Split(res.Stdout, "\n")
	for _, line := range lines {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		svcs = append(svcs, DiscoveredService{
			Type:       ServiceSSL,
			Name:       name,
			Confidence: 0.7,
			Level:      ConfidenceMedium,
			Details:    map[string]string{"path": "/etc/letsencrypt/live/" + name},
		})
	}
	return svcs, nil
}

func (s *Scanner) scanVHosts(ctx context.Context, exec executor.Executor) ([]DiscoveredService, error) {
	// Check for nginx sites-enabled or apache conf-enabled
	res, err := exec.Execute(ctx, "ls /etc/nginx/sites-enabled/ 2>/dev/null || ls /etc/httpd/conf.d/ 2>/dev/null")
	if err != nil || res.ExitCode != 0 {
		return nil, nil
	}

	var svcs []DiscoveredService
	lines := strings.Split(res.Stdout, "\n")
	for _, line := range lines {
		name := strings.TrimSpace(line)
		if name == "" || name == "default" {
			continue
		}
		svcs = append(svcs, DiscoveredService{
			Type:       ServiceVHost,
			Name:       name,
			Confidence: 0.6,
			Level:      ConfidenceMedium,
		})
	}
	return svcs, nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

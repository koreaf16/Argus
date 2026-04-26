// Package background
// File: stall_watchdog.go
// Description: Soft stall detector for background streaming jobs.
// Responsibility: Poll stdout growth and emit non-terminal "stall" notices.

package background

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
)

var stallPromptPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\(y\/n\)`),
	regexp.MustCompile(`(?i)\[y\/n\]`),
	regexp.MustCompile(`(?i)continue\?`),
	regexp.MustCompile(`(?i)password`),
	regexp.MustCompile(`(?i)press enter`),
}

// StallWatchdogConfig configures one watchdog loop.
type StallWatchdogConfig struct {
	JobID       int
	StoragePath string
	Notifier    WatchdogNotifier
	PollEvery   time.Duration
	StallAfter  time.Duration
	Cooldown    time.Duration
	TailBytes   int64
}

// RunStallWatchdog runs until ctx is done.
func RunStallWatchdog(ctx context.Context, cfg StallWatchdogConfig) {
	if cfg.Notifier == nil || strings.TrimSpace(cfg.StoragePath) == "" || cfg.JobID <= 0 {
		return
	}
	if cfg.PollEvery <= 0 {
		cfg.PollEvery = 5 * time.Second
	}
	if cfg.StallAfter <= 0 {
		cfg.StallAfter = 45 * time.Second
	}
	if cfg.Cooldown <= 0 {
		cfg.Cooldown = 40 * time.Second
	}
	if cfg.TailBytes <= 0 {
		cfg.TailBytes = 1024
	}

	ticker := time.NewTicker(cfg.PollEvery)
	defer ticker.Stop()

	var (
		lastSize   int64 = -1
		lastGrowth       = time.Now()
		lastNotice time.Time
	)

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			size, ok := currentFileSize(cfg.StoragePath)
			if !ok {
				continue
			}
			if size != lastSize {
				lastSize = size
				lastGrowth = now
				continue
			}
			if now.Sub(lastGrowth) < cfg.StallAfter {
				continue
			}
			if !lastNotice.IsZero() && now.Sub(lastNotice) < cfg.Cooldown {
				continue
			}

			lines, err := readTailLines(cfg.StoragePath, cfg.TailBytes)
			if err != nil {
				continue
			}
			if !matchesStallPrompt(lines) {
				continue
			}

			cfg.Notifier.Notify(NoticeEvent{
				JobID: cfg.JobID,
				Kind:  "stall",
				Tail:  lines,
				At:    now,
			})
			lastNotice = now
		}
	}
}

func currentFileSize(path string) (int64, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, false
	}
	return info.Size(), true
}

func readTailLines(path string, tailBytes int64) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open tail file: %w", err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat tail file: %w", err)
	}
	size := info.Size()
	if size <= 0 {
		return nil, nil
	}

	start := size - tailBytes
	if start < 0 {
		start = 0
	}
	buf := make([]byte, size-start)
	if _, err := f.ReadAt(buf, start); err != nil {
		return nil, fmt.Errorf("read tail file: %w", err)
	}

	rawLines := strings.Split(string(buf), "\n")
	lines := make([]string, 0, len(rawLines))
	for _, line := range rawLines {
		trimmed := strings.TrimSpace(strings.TrimRight(line, "\r"))
		if trimmed == "" {
			continue
		}
		lines = append(lines, trimmed)
	}
	if len(lines) > 8 {
		lines = lines[len(lines)-8:]
	}
	return lines, nil
}

func matchesStallPrompt(lines []string) bool {
	for _, line := range lines {
		for _, re := range stallPromptPatterns {
			if re.MatchString(line) {
				return true
			}
		}
	}
	return false
}

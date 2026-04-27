package tui

import (
	"bufio"
	"os/exec"
	"strconv"
	"strings"
)

// gitDiffStat runs `git diff HEAD --numstat` in dir and returns total inserted/deleted lines.
// Returns (0, 0) if not a git repo or on any error.
func gitDiffStat(dir string) (added, removed int) {
	cmd := exec.Command("git", "diff", "HEAD", "--numstat")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return 0, 0
	}
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		if a, err := strconv.Atoi(fields[0]); err == nil {
			added += a
		}
		if r, err := strconv.Atoi(fields[1]); err == nil {
			removed += r
		}
	}
	return added, removed
}

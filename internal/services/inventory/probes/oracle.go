package probes

import (
	"context"
	"strings"
	"time"
)

// OracleProbe detects Oracle Database instances via process and listener inspection.
type OracleProbe struct{}

func (p *OracleProbe) Name() string { return "oracle" }

func (p *OracleProbe) PreferredTimeout() time.Duration { return 8 * time.Second }

func (p *OracleProbe) Run(ctx context.Context, fn ProbeExec) (Result, error) {
	out, err := fn(ctx, p.ScriptFragment())
	if err != nil {
		return Result{}, err
	}
	return p.Parse(out)
}

func (p *OracleProbe) ScriptFragment() string {
	return `set +e
printf '<<ORA:proc>>\n'
pgrep -fa 'ora_pmon\|tnslsnr\|ora_smon' 2>/dev/null | head -20
printf '<<ORA:lsnr>>\n'
lsnrctl status 2>/dev/null | grep -E 'Listening|PORT|Service|Instance' | head -20
printf '<<ORA:ss>>\n'
ss -tln 2>/dev/null | grep -E ':(1521|1522|1526|1527) ' | head -10
printf '<<ORA:end>>\n'`
}

func (p *OracleProbe) Parse(stdout string) (Result, error) {
	sections := splitTagSections(stdout, "ORA")
	proc := sections["proc"]
	lsnr := sections["lsnr"]
	ss := sections["ss"]

	hasProc := strings.TrimSpace(proc) != ""
	hasSS := strings.TrimSpace(ss) != ""
	if !hasProc && !hasSS {
		return Result{}, nil
	}

	res := DatabaseResult{Engine: "oracle"}
	var evidence []string

	// Extract SIDs from ora_pmon_<SID> processes.
	for _, line := range strings.Split(proc, "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "ora_pmon_") {
			if idx := strings.Index(line, "ora_pmon_"); idx >= 0 {
				sid := strings.Fields(line[idx:])[0]
				sid = strings.TrimPrefix(sid, "ora_pmon_")
				if sid != "" {
					res.Instances = appendUnique(res.Instances, sid)
					evidence = append(evidence, "process: ora_pmon_"+sid)
				}
			}
		}
		if strings.Contains(line, "tnslsnr") {
			evidence = append(evidence, "process: tnslsnr")
		}
	}

	// Listener ports from lsnrctl status.
	for _, line := range strings.Split(lsnr, "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "PORT=") {
			if port := extractTagValue(line, "PORT="); port != "" {
				res.Listeners = appendUnique(res.Listeners, port)
			}
		}
		if strings.Contains(line, "Service") {
			evidence = append(evidence, "lsnrctl: "+line)
		}
	}

	// Fallback: ss port detection.
	for _, line := range strings.Split(ss, "\n") {
		if port := extractLastPort(line); port != "" {
			res.Listeners = appendUnique(res.Listeners, ":"+port)
		}
	}

	if hasProc && hasSS {
		res.Confidence = "high"
	} else if hasProc {
		res.Confidence = "medium"
	} else {
		res.Confidence = "low"
	}
	res.Evidence = evidence

	return Result{Databases: []DatabaseResult{res}}, nil
}

// extractTagValue extracts "PORT=1521" → "1521".
func extractTagValue(s, key string) string {
	idx := strings.Index(s, key)
	if idx < 0 {
		return ""
	}
	rest := s[idx+len(key):]
	rest = strings.TrimRight(rest, ")")
	return strings.Fields(rest)[0]
}

// extractLastPort returns the port from "0.0.0.0:1521" or ":::1521".
func extractLastPort(s string) string {
	for _, field := range strings.Fields(s) {
		if idx := strings.LastIndex(field, ":"); idx >= 0 {
			p := field[idx+1:]
			if len(p) > 0 && len(p) <= 5 {
				return p
			}
		}
	}
	return ""
}

// appendUnique appends s to slice only if not already present.
func appendUnique(slice []string, s string) []string {
	for _, v := range slice {
		if v == s {
			return slice
		}
	}
	return append(slice, s)
}

// splitTagSections splits output by <<TAG:key>> markers.
func splitTagSections(raw, tag string) map[string]string {
	sections := make(map[string]string)
	var current string
	var buf strings.Builder
	prefix := "<<" + tag + ":"
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, prefix) && strings.HasSuffix(line, ">>") {
			if current != "" && current != "end" {
				sections[current] = buf.String()
				buf.Reset()
			}
			current = line[len(prefix) : len(line)-2]
			continue
		}
		if current != "" && current != "end" {
			buf.WriteString(line)
			buf.WriteByte('\n')
		}
	}
	if current != "" && current != "end" && buf.Len() > 0 {
		sections[current] = buf.String()
	}
	return sections
}

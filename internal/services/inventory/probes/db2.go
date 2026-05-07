package probes

import (
	"context"
	"strings"
	"time"
)

// DB2Probe detects IBM Db2 instances.
type DB2Probe struct{}

func (p *DB2Probe) Name() string { return "db2" }

func (p *DB2Probe) PreferredTimeout() time.Duration { return 6 * time.Second }

func (p *DB2Probe) Run(ctx context.Context, fn ProbeExec) (Result, error) {
	out, err := fn(ctx, p.ScriptFragment())
	if err != nil {
		return Result{}, err
	}
	return p.Parse(out)
}

func (p *DB2Probe) ScriptFragment() string {
	return `set +e
printf '<<DB2:proc>>\n'
pgrep -fa 'db2sysc\|db2fmp\|db2agent' 2>/dev/null | head -10
printf '<<DB2:ss>>\n'
ss -tln 2>/dev/null | grep -E ':(50000|50001|50002) ' | head -5
printf '<<DB2:db>>\n'
command -v db2 >/dev/null 2>&1 && db2 list db directory 2>/dev/null | grep 'Database alias' | head -10
printf '<<DB2:end>>\n'`
}

func (p *DB2Probe) Parse(stdout string) (Result, error) {
	sections := splitTagSections(stdout, "DB2")
	proc := sections["proc"]
	ss := sections["ss"]
	db := sections["db"]

	hasProc := strings.TrimSpace(proc) != ""
	hasSS := strings.TrimSpace(ss) != ""
	if !hasProc && !hasSS {
		return Result{}, nil
	}

	res := DatabaseResult{Engine: "db2"}
	var evidence []string

	if hasProc {
		evidence = append(evidence, "process: db2sysc")
	}
	for _, line := range strings.Split(db, "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "Database alias") {
			if idx := strings.Index(line, "="); idx >= 0 {
				alias := strings.TrimSpace(line[idx+1:])
				if alias != "" {
					res.Instances = appendUnique(res.Instances, alias)
					evidence = append(evidence, "db: "+alias)
				}
			}
		}
	}
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

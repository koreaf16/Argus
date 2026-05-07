package probes

import (
	"context"
	"strings"
	"time"
)

// GeneralDatabasesProbe detects common database engines:
// PostgreSQL, MySQL, MariaDB, MongoDB, MSSQL, Redis.
type GeneralDatabasesProbe struct{}

func (p *GeneralDatabasesProbe) Name() string { return "general_databases" }

func (p *GeneralDatabasesProbe) PreferredTimeout() time.Duration { return 4 * time.Second }

func (p *GeneralDatabasesProbe) Run(ctx context.Context, fn ProbeExec) (Result, error) {
	out, err := fn(ctx, p.ScriptFragment())
	if err != nil {
		return Result{}, err
	}
	return p.Parse(out)
}

func (p *GeneralDatabasesProbe) ScriptFragment() string {
	return `set +e
printf '<<GDB:proc>>\n'
pgrep -fa 'postgres\|mysqld\|mariadbd\|mongod\|sqlservr\|redis-server\|memcached' 2>/dev/null | head -20
printf '<<GDB:ss>>\n'
ss -tln 2>/dev/null | grep -E ':(5432|3306|27017|1433|6379|11211) ' | head -10
printf '<<GDB:ver>>\n'
command -v psql >/dev/null 2>&1 && psql --version 2>/dev/null | head -1
command -v mysql >/dev/null 2>&1 && mysql --version 2>/dev/null | head -1
command -v mongod >/dev/null 2>&1 && mongod --version 2>/dev/null | head -1
command -v redis-cli >/dev/null 2>&1 && redis-cli --version 2>/dev/null | head -1
printf '<<GDB:end>>\n'`
}

// portEngineMap maps well-known ports to engine names.
var portEngineMap = map[string]string{
	"5432":  "postgres",
	"3306":  "mysql",
	"27017": "mongodb",
	"1433":  "mssql",
	"6379":  "redis",
	"11211": "memcached",
}

// procPatternMap maps process name patterns to engine names.
var procPatternMap = []struct{ pattern, engine string }{
	{"postgres", "postgres"},
	{"mysqld", "mysql"},
	{"mariadbd", "mariadb"},
	{"mongod", "mongodb"},
	{"sqlservr", "mssql"},
	{"redis-server", "redis"},
	{"memcached", "memcached"},
}

func (p *GeneralDatabasesProbe) Parse(stdout string) (Result, error) {
	sections := splitTagSections(stdout, "GDB")
	proc := sections["proc"]
	ss := sections["ss"]
	ver := sections["ver"]

	procEngines := make(map[string]bool)
	for _, line := range strings.Split(proc, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		for _, pm := range procPatternMap {
			if strings.Contains(line, pm.pattern) {
				procEngines[pm.engine] = true
			}
		}
	}

	portEngines := make(map[string]string) // engine → port
	for _, line := range strings.Split(ss, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		port := extractLastPort(line)
		if engine, ok := portEngineMap[port]; ok {
			portEngines[engine] = port
		}
	}

	// Version strings by engine.
	versions := make(map[string]string)
	for _, line := range strings.Split(ver, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.Contains(line, "PostgreSQL"):
			versions["postgres"] = extractFirstWord(line, "PostgreSQL")
		case strings.Contains(line, "mysql") || strings.Contains(line, "Distrib"):
			versions["mysql"] = extractFirstWord(line, "Distrib")
		case strings.Contains(line, "Redis"):
			versions["redis"] = extractFirstWord(line, "Redis")
		case strings.Contains(line, "db version"):
			versions["mongodb"] = extractFirstWord(line, "v")
		}
	}

	allEngines := make(map[string]bool)
	for e := range procEngines {
		allEngines[e] = true
	}
	for e := range portEngines {
		allEngines[e] = true
	}
	if len(allEngines) == 0 {
		return Result{}, nil
	}

	var results []DatabaseResult
	for engine := range allEngines {
		res := DatabaseResult{Engine: engine}
		res.Version = versions[engine]
		if port, ok := portEngines[engine]; ok {
			res.Listeners = []string{":" + port}
		}
		hasProc := procEngines[engine]
		hasSS := portEngines[engine] != ""
		switch {
		case hasProc && hasSS:
			res.Confidence = "high"
			res.Evidence = []string{"process+listener detected"}
		case hasProc:
			res.Confidence = "medium"
			res.Evidence = []string{"process detected"}
		default:
			res.Confidence = "low"
			res.Evidence = []string{"listener detected on port " + portEngines[engine]}
		}
		results = append(results, res)
	}
	return Result{Databases: results}, nil
}

func extractFirstWord(s, afterKeyword string) string {
	idx := strings.Index(s, afterKeyword)
	if idx < 0 {
		return ""
	}
	fields := strings.Fields(s[idx:])
	if len(fields) < 2 {
		return ""
	}
	return fields[1]
}

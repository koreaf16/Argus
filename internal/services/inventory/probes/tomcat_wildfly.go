package probes

import (
	"context"
	"strings"
	"time"
)

// TomcatWildflyProbe detects Java WAS: Tomcat, WildFly/JBoss, WebLogic, JEUS.
type TomcatWildflyProbe struct{}

func (p *TomcatWildflyProbe) Name() string { return "tomcat_wildfly" }

func (p *TomcatWildflyProbe) PreferredTimeout() time.Duration { return 4 * time.Second }

func (p *TomcatWildflyProbe) Run(ctx context.Context, fn ProbeExec) (Result, error) {
	out, err := fn(ctx, p.ScriptFragment())
	if err != nil {
		return Result{}, err
	}
	return p.Parse(out)
}

func (p *TomcatWildflyProbe) ScriptFragment() string {
	return `set +e
printf '<<JWAS:proc>>\n'
pgrep -fa 'catalina\|jboss\|wildfly\|weblogic\|jeus' 2>/dev/null | head -15
printf '<<JWAS:home>>\n'
ps -eo args 2>/dev/null | grep -E 'catalina\.home|jboss\.home|weblogic\.home|jeus\.home' | head -5
printf '<<JWAS:ss>>\n'
ss -tln 2>/dev/null | grep -E ':(8080|8443|8180|9990|9993|7001|7002|9900) ' | head -8
printf '<<JWAS:end>>\n'`
}

// wasPatterns maps process patterns to WAS engine names.
var wasPatterns = []struct{ pattern, engine string }{
	{"catalina", "tomcat"},
	{"jboss", "wildfly"},
	{"wildfly", "wildfly"},
	{"weblogic", "weblogic"},
	{"jeus", "jeus"},
}

// wasPortMap maps ports to engine hints.
var wasPortMap = map[string]string{
	"8080": "tomcat",
	"8443": "tomcat",
	"8180": "wildfly",
	"9990": "wildfly",
	"9993": "wildfly",
	"7001": "weblogic",
	"7002": "weblogic",
	"9900": "jeus",
}

func (p *TomcatWildflyProbe) Parse(stdout string) (Result, error) {
	sections := splitTagSections(stdout, "JWAS")
	proc := sections["proc"]
	home := sections["home"]
	ss := sections["ss"]

	engines := make(map[string]*WASResult)

	for _, line := range strings.Split(proc, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		for _, wp := range wasPatterns {
			if strings.Contains(strings.ToLower(line), wp.pattern) {
				if _, ok := engines[wp.engine]; !ok {
					engines[wp.engine] = &WASResult{Engine: wp.engine}
				}
				engines[wp.engine].Evidence = append(engines[wp.engine].Evidence, "process detected")
				break
			}
		}
	}

	// Extract home directories.
	for _, line := range strings.Split(home, "\n") {
		line = strings.TrimSpace(line)
		for _, hw := range []struct{ key, engine string }{
			{"catalina.home=", "tomcat"},
			{"jboss.home=", "wildfly"},
			{"weblogic.home=", "weblogic"},
			{"jeus.home=", "jeus"},
		} {
			if idx := strings.Index(line, hw.key); idx >= 0 {
				val := strings.Fields(line[idx+len(hw.key):])[0]
				if e, ok := engines[hw.engine]; ok {
					e.Home = val
				}
			}
		}
	}

	// Port matching.
	for _, line := range strings.Split(ss, "\n") {
		port := extractLastPort(line)
		if engine, ok := wasPortMap[port]; ok {
			if _, exists := engines[engine]; !exists {
				engines[engine] = &WASResult{Engine: engine}
			}
			portInt := 0
			for _, f := range strings.Fields(line) {
				if idx := strings.LastIndex(f, ":"); idx >= 0 {
					if p := f[idx+1:]; p == port {
						break
					}
				}
				_ = f
			}
			_ = portInt
			engines[engine].Ports = appendIntUnique(engines[engine].Ports, portToInt(port))
			engines[engine].Evidence = append(engines[engine].Evidence, "listener: "+port)
		}
	}

	if len(engines) == 0 {
		return Result{}, nil
	}

	var results []WASResult
	for _, e := range engines {
		hasProc := len(e.Evidence) > 0 && strings.Contains(strings.Join(e.Evidence, ","), "process")
		hasPort := len(e.Ports) > 0
		switch {
		case hasProc && hasPort:
			e.Confidence = "high"
		case hasProc:
			e.Confidence = "medium"
		default:
			e.Confidence = "low"
		}
		results = append(results, *e)
	}
	return Result{WAS: results}, nil
}

func appendIntUnique(slice []int, v int) []int {
	for _, x := range slice {
		if x == v {
			return slice
		}
	}
	return append(slice, v)
}

func portToInt(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}

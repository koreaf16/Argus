package probes

import (
	"context"
	"strings"
	"time"
)

// NginxApacheProbe detects Nginx and Apache HTTP servers.
type NginxApacheProbe struct{}

func (p *NginxApacheProbe) Name() string { return "nginx_apache" }

func (p *NginxApacheProbe) PreferredTimeout() time.Duration { return 3 * time.Second }

func (p *NginxApacheProbe) Run(ctx context.Context, fn ProbeExec) (Result, error) {
	out, err := fn(ctx, p.ScriptFragment())
	if err != nil {
		return Result{}, err
	}
	return p.Parse(out)
}

func (p *NginxApacheProbe) ScriptFragment() string {
	return `set +e
printf '<<WEB:proc>>\n'
pgrep -fa 'nginx\|httpd\|apache2' 2>/dev/null | head -10
printf '<<WEB:ver>>\n'
nginx -v 2>&1 | head -1
httpd -v 2>/dev/null | head -1
apache2 -v 2>/dev/null | head -1
printf '<<WEB:ss>>\n'
ss -tln 2>/dev/null | grep -E ':(80|443|8080|8443) ' | head -8
printf '<<WEB:end>>\n'`
}

func (p *NginxApacheProbe) Parse(stdout string) (Result, error) {
	sections := splitTagSections(stdout, "WEB")
	proc := sections["proc"]
	ver := sections["ver"]
	ss := sections["ss"]

	hasNginxProc := strings.Contains(proc, "nginx")
	hasApacheProc := strings.Contains(proc, "httpd") || strings.Contains(proc, "apache2")
	hasSS := strings.TrimSpace(ss) != ""

	if !hasNginxProc && !hasApacheProc && !hasSS {
		return Result{}, nil
	}

	var results []WASResult

	// Nginx.
	if hasNginxProc || strings.Contains(ver, "nginx") {
		res := WASResult{Engine: "nginx"}
		for _, line := range strings.Split(ver, "\n") {
			if strings.Contains(line, "nginx/") {
				if idx := strings.Index(line, "nginx/"); idx >= 0 {
					fields := strings.Fields(line[idx:])
					if len(fields) > 0 {
						res.Version = strings.TrimPrefix(fields[0], "nginx/")
					}
				}
			}
		}
		for _, line := range strings.Split(ss, "\n") {
			if port := extractLastPort(line); port == "80" || port == "443" || port == "8080" || port == "8443" {
				res.Ports = appendIntUnique(res.Ports, portToInt(port))
			}
		}
		hasPorts := len(res.Ports) > 0
		if hasNginxProc && hasPorts {
			res.Confidence = "high"
		} else if hasNginxProc {
			res.Confidence = "medium"
		} else {
			res.Confidence = "low"
		}
		res.Evidence = []string{"process: nginx"}
		results = append(results, res)
	}

	// Apache.
	if hasApacheProc || strings.Contains(ver, "Apache") {
		res := WASResult{Engine: "apache"}
		for _, line := range strings.Split(ver, "\n") {
			if strings.Contains(line, "Apache/") {
				if idx := strings.Index(line, "Apache/"); idx >= 0 {
					fields := strings.Fields(line[idx:])
					if len(fields) > 0 {
						res.Version = strings.TrimPrefix(fields[0], "Apache/")
					}
				}
			}
		}
		for _, line := range strings.Split(ss, "\n") {
			if port := extractLastPort(line); port == "80" || port == "443" || port == "8080" || port == "8443" {
				res.Ports = appendIntUnique(res.Ports, portToInt(port))
			}
		}
		hasPorts := len(res.Ports) > 0
		if hasApacheProc && hasPorts {
			res.Confidence = "high"
		} else if hasApacheProc {
			res.Confidence = "medium"
		} else {
			res.Confidence = "low"
		}
		res.Evidence = []string{"process: httpd/apache2"}
		results = append(results, res)
	}

	if len(results) == 0 {
		return Result{}, nil
	}
	return Result{WAS: results}, nil
}

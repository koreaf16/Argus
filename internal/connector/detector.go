package connector

import (
	"strings"
)

type ConnectorHint struct {
	Name    string
	Runtime RuntimeType
	Source  string
}

var techHints = map[string]ConnectorHint{
	"oracle":     {Name: "oracle-mcp", Runtime: RuntimeUVX, Source: "registry"},
	"sqlplus":    {Name: "oracle-mcp", Runtime: RuntimeUVX, Source: "registry"},
	"mysql":      {Name: "mysql-mcp-server", Runtime: RuntimeUVX, Source: "registry"},
	"postgres":   {Name: "postgres-mcp", Runtime: RuntimeNPX, Source: "registry"},
	"postgresql": {Name: "postgres-mcp", Runtime: RuntimeNPX, Source: "registry"},
	"kubectl":    {Name: "k8s-mcp-server", Runtime: RuntimeNPX, Source: "registry"},
	"kubernetes": {Name: "k8s-mcp-server", Runtime: RuntimeNPX, Source: "registry"},
	"helm":       {Name: "k8s-mcp-server", Runtime: RuntimeNPX, Source: "registry"},
	"terraform":  {Name: "terraform-mcp-server", Runtime: RuntimeNPX, Source: "registry"},
	"docker":     {Name: "portainer-mcp", Runtime: RuntimeNPX, Source: "registry"},
	"redis":      {Name: "mcp-redis-cloud", Runtime: RuntimeNPX, Source: "registry"},
	"mongodb":    {Name: "mongodb-lens", Runtime: RuntimeNPX, Source: "registry"},
	"sqlite":     {Name: "mcp-server-sqlite", Runtime: RuntimeNPX, Source: "registry"},
}

func Detect(userMessage string, installed []string) *ConnectorHint {
	userMessage = strings.ToLower(userMessage)
	
	installedMap := make(map[string]bool)
	for _, name := range installed {
		installedMap[name] = true
	}

	for keyword, hint := range techHints {
		if strings.Contains(userMessage, keyword) {
			if !installedMap[hint.Name] {
				return &hint
			}
		}
	}
	return nil
}

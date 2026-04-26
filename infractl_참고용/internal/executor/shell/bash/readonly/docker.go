// Package readonly
// File: docker.go
// Description: docker 읽기 전용 서브커맨드 화이트리스트
// Responsibility: DockerReadOnlyCommands 매핑 제공

// Ported from: claude_cli/src/utils/shell/readOnlyCommandValidation.ts:DOCKER_READ_ONLY_COMMANDS

package readonly

// DockerReadOnlyCommands 는 안전한 docker 서브커맨드 목록이다.
var DockerReadOnlyCommands = map[string]CommandConfig{
	"docker ps": {SafeFlags: map[string]FlagArgType{
		"-a": FlagArgNone, "--all": FlagArgNone,
		"-q": FlagArgNone, "--quiet": FlagArgNone,
		"-n": FlagArgNumber, "--last": FlagArgNumber,
		"--no-trunc": FlagArgNone,
		"--format": FlagArgString,
		"-f": FlagArgString, "--filter": FlagArgString,
		"-s": FlagArgNone, "--size": FlagArgNone,
	}},
	"docker images": {SafeFlags: map[string]FlagArgType{
		"-a": FlagArgNone, "--all": FlagArgNone,
		"-q": FlagArgNone, "--quiet": FlagArgNone,
		"--no-trunc": FlagArgNone,
		"--format": FlagArgString,
		"-f": FlagArgString, "--filter": FlagArgString,
		"--digests": FlagArgNone,
	}},
	"docker logs": {SafeFlags: map[string]FlagArgType{
		"--follow": FlagArgNone, "-f": FlagArgNone,
		"--tail": FlagArgString, "-n": FlagArgString,
		"--timestamps": FlagArgNone, "-t": FlagArgNone,
		"--since": FlagArgString, "--until": FlagArgString,
		"--details": FlagArgNone,
	}},
	"docker inspect": {SafeFlags: map[string]FlagArgType{
		"--format": FlagArgString, "-f": FlagArgString,
		"--type": FlagArgString,
		"--size": FlagArgNone, "-s": FlagArgNone,
	}},
	"docker stats": {SafeFlags: map[string]FlagArgType{
		"--no-stream": FlagArgNone, "--no-trunc": FlagArgNone,
		"--format": FlagArgString,
		"-a": FlagArgNone, "--all": FlagArgNone,
	}},
	"docker version": {SafeFlags: map[string]FlagArgType{
		"--format": FlagArgString, "-f": FlagArgString,
	}},
	"docker info": {SafeFlags: map[string]FlagArgType{
		"--format": FlagArgString, "-f": FlagArgString,
	}},
	"docker top": {SafeFlags: map[string]FlagArgType{}},
}

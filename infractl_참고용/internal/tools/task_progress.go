package tools

import (
	"encoding/json"
	"path"
	"regexp"
	"strings"
	"unicode"

	"github.com/yourorg/infractl/internal/executor"
)

var phasePrefixRE = regexp.MustCompile(`(?i)^phase\s+[a-z0-9]+\s*:\s*`)

// TaskProgressMetadata captures task-level execution and verification state
// for shell-backed work so the runtime and UI can track meaningful progress.
type TaskProgressMetadata struct {
	TaskKind             string `json:"task_kind,omitempty"`
	TaskSummary          string `json:"task_summary,omitempty"`
	TaskSubject          string `json:"task_subject,omitempty"`
	TaskKey              string `json:"task_key,omitempty"`
	JobID                int    `json:"job_id,omitempty"`
	ExecutionStatus      string `json:"execution_status,omitempty"`
	VerificationRequired bool   `json:"verification_required,omitempty"`
	VerificationStatus   string `json:"verification_status,omitempty"`
	VerificationHint     string `json:"verification_hint,omitempty"`
	VerificationSummary  string `json:"verification_summary,omitempty"`
	VerificationEvidence string `json:"verification_evidence,omitempty"`
	VerifiedByToolID     string `json:"verified_by_tool_id,omitempty"`
}

func ParseTaskProgressMetadata(raw string) (TaskProgressMetadata, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		return TaskProgressMetadata{}, false
	}

	var meta TaskProgressMetadata
	if err := json.Unmarshal([]byte(raw), &meta); err != nil {
		return TaskProgressMetadata{}, false
	}

	meta.TaskSummary = NormalizeTaskSummary(meta.TaskSummary)
	meta.TaskSubject = strings.TrimSpace(meta.TaskSubject)
	meta.TaskKey = strings.TrimSpace(meta.TaskKey)
	if meta.TaskKey == "" {
		meta.TaskKey = buildTaskProgressKey(meta.TaskSummary, meta.TaskSubject)
	}
	if meta.TaskSummary == "" && meta.ExecutionStatus == "" && meta.VerificationStatus == "" {
		return TaskProgressMetadata{}, false
	}
	return meta, true
}

func (m TaskProgressMetadata) JSON() string {
	meta := m
	meta.TaskSummary = NormalizeTaskSummary(meta.TaskSummary)
	meta.TaskSubject = strings.TrimSpace(meta.TaskSubject)
	if meta.TaskKey == "" {
		meta.TaskKey = buildTaskProgressKey(meta.TaskSummary, meta.TaskSubject)
	}
	b, err := json.Marshal(meta)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func (m TaskProgressMetadata) PendingVerification() bool {
	return m.VerificationRequired && m.VerificationStatus == "pending"
}

func (m TaskProgressMetadata) ApplyVerification(verifiedByToolID, summary, evidence string) TaskProgressMetadata {
	meta := m
	meta.ExecutionStatus = "succeeded"
	meta.VerificationRequired = true
	meta.VerificationStatus = "verified"
	meta.VerificationSummary = NormalizeTaskSummary(summary)
	meta.VerificationEvidence = strings.TrimSpace(evidence)
	meta.VerifiedByToolID = strings.TrimSpace(verifiedByToolID)
	if meta.TaskKey == "" {
		meta.TaskKey = buildTaskProgressKey(meta.TaskSummary, meta.TaskSubject)
	}
	return meta
}

func NormalizeTaskSummary(raw string) string {
	s := strings.TrimSpace(raw)
	s = phasePrefixRE.ReplaceAllString(s, "")
	s = strings.Join(strings.Fields(s), " ")
	return s
}

func BuildShellTaskProgressMetadata(args map[string]interface{}, command string, result executor.ExecResult) TaskProgressMetadata {
	return buildShellTaskProgressMetadataWithSuccess(args, command, result, result.ExitCode == 0)
}

func buildShellTaskProgressMetadataWithSuccess(args map[string]interface{}, command string, result executor.ExecResult, success bool) TaskProgressMetadata {
	kind := inferShellTaskKind(command)
	subject := inferShellTaskSubject(kind, command)
	summary := inferShellTaskSummary(args, command, kind, subject)
	required := shellTaskRequiresVerification(kind, command)

	verificationStatus := "not_required"
	if !success {
		verificationStatus = "failed"
	} else if required {
		verificationStatus = "pending"
	}

	verificationHint := ""
	if required {
		verificationHint = inferVerificationHint(kind, subject)
	}

	executionStatus := "failed"
	if success {
		executionStatus = "succeeded"
	}

	return TaskProgressMetadata{
		TaskKind:             kind,
		TaskSummary:          summary,
		TaskSubject:          subject,
		TaskKey:              buildTaskProgressKey(summary, subject),
		ExecutionStatus:      executionStatus,
		VerificationRequired: required,
		VerificationStatus:   verificationStatus,
		VerificationHint:     verificationHint,
	}
}

func inferShellTaskSummary(args map[string]interface{}, command, kind, subject string) string {
	for _, key := range []string{"description", "reason", "action", "step", "thought", "explanation"} {
		if val, err := argString(args, key, false); err == nil {
			if normalized := NormalizeTaskSummary(val); normalized != "" {
				return normalized
			}
		}
	}

	switch kind {
	case "install":
		switch packageActionFromCommand(command) {
		case "remove":
			if subject != "" {
				return subject + " 제거"
			}
			return "패키지 제거"
		case "upgrade", "update":
			if subject != "" {
				return subject + " 업데이트"
			}
			return "패키지 업데이트"
		default:
			if subject != "" {
				return subject + " 설치"
			}
			return "패키지 설치"
		}
	case "service":
		action := serviceActionFromCommand(command)
		actionLabel := map[string]string{
			"start":   "시작",
			"stop":    "중지",
			"restart": "재시작",
			"reload":  "재적용",
			"enable":  "활성화",
			"disable": "비활성화",
			"status":  "상태 확인",
		}[action]
		if actionLabel == "" {
			actionLabel = "변경"
		}
		if subject != "" {
			return subject + " 서비스 " + actionLabel
		}
		return "서비스 " + actionLabel
	case "configure":
		if subject != "" {
			return subject + " 설정 변경"
		}
		return "설정 변경"
	case "file":
		if subject != "" {
			return subject + " 변경"
		}
		return "파일 변경"
	default:
		if subject != "" {
			return subject + " 상태 확인"
		}
		return "상태 확인"
	}
}

func inferShellTaskKind(command string) string {
	lower := " " + strings.ToLower(strings.TrimSpace(command)) + " "

	switch {
	case isPackageMutation(lower):
		return "install"
	case isServiceCommand(lower):
		if shellTaskRequiresVerification("service", command) {
			return "service"
		}
		return "inspect"
	case isConfigMutation(lower):
		return "configure"
	case isFileMutation(lower):
		return "file"
	default:
		return "inspect"
	}
}

func inferShellTaskSubject(kind, command string) string {
	if kind == "service" || kind == "inspect" {
		if subject := serviceSubjectFromCommand(command); subject != "" {
			return subject
		}
	}

	tokens := shellTokens(command)
	if len(tokens) == 0 {
		return ""
	}

	switch kind {
	case "install":
		if subject := packageSubjectFromTokens(tokens); subject != "" {
			return subject
		}
	case "service":
		if subject := serviceSubjectFromTokens(tokens); subject != "" {
			return subject
		}
	case "configure":
		if subject := configSubjectFromTokens(tokens); subject != "" {
			return subject
		}
	case "file":
		if subject := fileSubjectFromTokens(tokens); subject != "" {
			return subject
		}
	case "inspect":
		if subject := inspectSubjectFromTokens(tokens); subject != "" {
			return subject
		}
	}
	return ""
}

func shellTaskRequiresVerification(kind, command string) bool {
	if kind == "inspect" {
		return false
	}
	if kind == "service" {
		switch serviceActionFromCommand(command) {
		case "status", "is-active", "show", "list":
			return false
		}
	}
	return true
}

func inferVerificationHint(kind, subject string) string {
	switch kind {
	case "install":
		if subject != "" {
			return subject + " 설치 상태를 확인하세요"
		}
		return "설치된 패키지 상태를 확인하세요"
	case "service":
		if subject != "" {
			return subject + " 서비스가 기대 상태인지 확인하세요"
		}
		return "서비스 상태를 확인하세요"
	case "file":
		if subject != "" {
			return subject + " 파일 또는 디렉터리 상태를 확인하세요"
		}
		return "파일 또는 디렉터리 상태를 확인하세요"
	case "configure":
		if subject != "" {
			return subject + " 설정이 반영됐는지 확인하세요"
		}
		return "설정 반영 여부를 확인하세요"
	default:
		return "후속 확인 명령으로 기대 상태를 검증하세요"
	}
}

func buildTaskProgressKey(summary, subject string) string {
	base := strings.TrimSpace(subject)
	if base == "" {
		base = NormalizeTaskSummary(summary)
	}
	if base == "" {
		return ""
	}

	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(base) {
		switch {
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			b.WriteRune(r)
			prevDash = false
		case unicode.IsSpace(r) || r == '-' || r == '_' || r == '/' || r == '\\' || r == '.':
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func isPackageMutation(lower string) bool {
	switch {
	case strings.Contains(lower, " apt install "),
		strings.Contains(lower, " apt-get install "),
		strings.Contains(lower, " yum install "),
		strings.Contains(lower, " yum localinstall "),
		strings.Contains(lower, " dnf install "),
		strings.Contains(lower, " apk add "),
		strings.Contains(lower, " pip install "),
		strings.Contains(lower, " npm install "),
		strings.Contains(lower, " pnpm install "),
		strings.Contains(lower, " go install "),
		strings.Contains(lower, " rpm -i "),
		strings.Contains(lower, " rpm -iv"),
		strings.Contains(lower, " dpkg -i "),
		strings.Contains(lower, " apt remove "),
		strings.Contains(lower, " apt-get remove "),
		strings.Contains(lower, " yum remove "),
		strings.Contains(lower, " dnf remove "),
		strings.Contains(lower, " apk del "),
		strings.Contains(lower, " npm uninstall "),
		strings.Contains(lower, " pnpm remove "),
		strings.Contains(lower, " apt upgrade "),
		strings.Contains(lower, " apt-get upgrade "),
		strings.Contains(lower, " yum update "),
		strings.Contains(lower, " dnf update "):
		return true
	default:
		return false
	}
}

func isServiceCommand(lower string) bool {
	switch {
	case strings.Contains(lower, " systemctl "),
		strings.HasPrefix(strings.TrimSpace(lower), "systemctl "),
		strings.Contains(lower, " service "),
		strings.HasPrefix(strings.TrimSpace(lower), "service "),
		strings.HasPrefix(strings.TrimSpace(lower), "sc "),
		strings.HasPrefix(strings.TrimSpace(lower), "sc.exe "),
		strings.HasPrefix(strings.TrimSpace(lower), "start-service "),
		strings.HasPrefix(strings.TrimSpace(lower), "stop-service "),
		strings.HasPrefix(strings.TrimSpace(lower), "restart-service "),
		strings.HasPrefix(strings.TrimSpace(lower), "get-service "):
		return true
	default:
		return false
	}
}

func isConfigMutation(lower string) bool {
	switch {
	case strings.Contains(lower, " sed -i "),
		strings.Contains(lower, " sysctl "),
		strings.HasPrefix(strings.TrimSpace(lower), "setx "),
		strings.Contains(lower, " reg add "),
		strings.Contains(lower, " export "),
		strings.Contains(lower, " set-itemproperty "),
		strings.Contains(lower, " new-itemproperty "):
		return true
	default:
		return false
	}
}

func isFileMutation(lower string) bool {
	switch {
	case strings.Contains(lower, " mkdir "),
		strings.Contains(lower, " cp "),
		strings.Contains(lower, " mv "),
		strings.Contains(lower, " chmod "),
		strings.Contains(lower, " chown "),
		strings.Contains(lower, " ln "),
		strings.Contains(lower, " touch "),
		strings.Contains(lower, " rm "),
		strings.HasPrefix(strings.TrimSpace(lower), "new-item "),
		strings.HasPrefix(strings.TrimSpace(lower), "copy-item "),
		strings.HasPrefix(strings.TrimSpace(lower), "move-item "),
		strings.HasPrefix(strings.TrimSpace(lower), "remove-item "),
		strings.HasPrefix(strings.TrimSpace(lower), "set-content "),
		strings.HasPrefix(strings.TrimSpace(lower), "add-content "):
		return true
	default:
		return false
	}
}

func packageActionFromCommand(command string) string {
	lower := " " + strings.ToLower(strings.TrimSpace(command)) + " "
	switch {
	case strings.Contains(lower, " remove "),
		strings.Contains(lower, " uninstall "),
		strings.Contains(lower, " del "):
		return "remove"
	case strings.Contains(lower, " upgrade "),
		strings.Contains(lower, " update "):
		return "upgrade"
	default:
		return "install"
	}
}

func serviceActionFromCommand(command string) string {
	for _, seg := range parseShellSegmentInfo(command) {
		if action := serviceActionFromTokens(seg.Tokens); action != "" {
			return action
		}
	}
	return ""
}

func serviceActionFromTokens(tokens []string) string {
	if len(tokens) == 0 {
		return ""
	}

	switch strings.ToLower(tokens[0]) {
	case "systemctl":
		if len(tokens) > 1 {
			return strings.ToLower(tokens[1])
		}
	case "service":
		if len(tokens) > 2 {
			return strings.ToLower(tokens[2])
		}
	case "sc", "sc.exe":
		if len(tokens) > 1 {
			return strings.ToLower(tokens[1])
		}
	case "start-service":
		return "start"
	case "stop-service":
		return "stop"
	case "restart-service":
		return "restart"
	case "get-service":
		return "status"
	}
	return ""
}

func serviceSubjectFromCommand(command string) string {
	for _, seg := range parseShellSegmentInfo(command) {
		if serviceActionFromTokens(seg.Tokens) == "" {
			continue
		}
		if subject := serviceSubjectFromTokens(seg.Tokens); subject != "" {
			return subject
		}
	}
	return ""
}

func packageSubjectFromTokens(tokens []string) string {
	if len(tokens) == 0 {
		return ""
	}

	lower0 := strings.ToLower(tokens[0])
	switch lower0 {
	case "rpm":
		for _, tok := range tokens[1:] {
			if strings.HasPrefix(tok, "-q") {
				break
			}
			if strings.HasPrefix(tok, "-") {
				continue
			}
			return trimPackageToken(tok)
		}
	case "dpkg":
		for _, tok := range tokens[1:] {
			if strings.HasPrefix(tok, "-") {
				continue
			}
			return trimPackageToken(tok)
		}
	}

	verbs := map[string]bool{
		"install": true, "remove": true, "uninstall": true, "upgrade": true,
		"update": true, "localinstall": true, "add": true, "del": true,
	}
	for i, tok := range tokens {
		if !verbs[strings.ToLower(tok)] {
			continue
		}
		for _, next := range tokens[i+1:] {
			if strings.HasPrefix(next, "-") {
				continue
			}
			return trimPackageToken(next)
		}
	}
	return ""
}

func serviceSubjectFromTokens(tokens []string) string {
	if len(tokens) == 0 {
		return ""
	}

	switch strings.ToLower(tokens[0]) {
	case "systemctl":
		if len(tokens) > 2 {
			return cleanToken(tokens[2])
		}
	case "service":
		if len(tokens) > 1 {
			return cleanToken(tokens[1])
		}
	case "sc", "sc.exe":
		if len(tokens) > 2 {
			return cleanToken(tokens[2])
		}
	case "start-service", "stop-service", "restart-service", "get-service":
		for i := 1; i < len(tokens); i++ {
			if strings.EqualFold(tokens[i], "-name") && i+1 < len(tokens) {
				return cleanToken(tokens[i+1])
			}
			if !strings.HasPrefix(tokens[i], "-") {
				return cleanToken(tokens[i])
			}
		}
	}
	return ""
}

func fileSubjectFromTokens(tokens []string) string {
	if len(tokens) == 0 {
		return ""
	}
	last := cleanToken(tokens[len(tokens)-1])
	if last == "" {
		return ""
	}
	return last
}

func configSubjectFromTokens(tokens []string) string {
	if len(tokens) == 0 {
		return ""
	}
	for i, tok := range tokens {
		lower := strings.ToLower(tok)
		if lower == "sysctl" {
			for _, next := range tokens[i+1:] {
				if strings.HasPrefix(next, "-") {
					continue
				}
				return cleanToken(next)
			}
		}
	}
	return fileSubjectFromTokens(tokens)
}

func inspectSubjectFromTokens(tokens []string) string {
	if len(tokens) == 0 {
		return ""
	}
	switch strings.ToLower(tokens[0]) {
	case "systemctl", "service", "sc", "sc.exe", "get-service":
		if subject := serviceSubjectFromTokens(tokens); subject != "" {
			return subject
		}
	case "rpm", "dpkg", "apt", "apt-get", "yum", "dnf":
		if subject := packageSubjectFromTokens(tokens); subject != "" {
			return subject
		}
	}
	return cleanToken(tokens[len(tokens)-1])
}

func shellTokens(command string) []string {
	raw := strings.Fields(command)
	out := make([]string, 0, len(raw))
	for _, tok := range raw {
		if cleaned := cleanToken(tok); cleaned != "" {
			out = append(out, cleaned)
		}
	}
	return out
}

func cleanToken(token string) string {
	return strings.Trim(token, "\"'` ,;()[]{}")
}

func trimPackageToken(token string) string {
	token = cleanToken(token)
	if token == "" {
		return ""
	}
	token = strings.TrimSuffix(token, ".rpm")
	token = strings.TrimSuffix(token, ".deb")
	token = path.Base(strings.ReplaceAll(token, "\\", "/"))
	return token
}

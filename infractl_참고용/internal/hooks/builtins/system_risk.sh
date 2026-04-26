#!/usr/bin/env sh
# File: system_risk.sh
# Description: Fast-path 결정론 차단기 — 명백히 파괴적인 셸 명령을 정규식으로 deny.
# Responsibility: PreToolUse command backend — stdin JSON → stdout HookOutput JSON.
# 의존성: sh + grep (POSIX). Windows 는 git-bash 전제.
#
# claude_cli 대응: 별도 등가물 없음 (LLM 위임 정책). InfraCtl 만의 안전망.
# 출력 스키마(새 HookOutput):
#   {"decision":"deny","reason":"...","systemMessage":"..."}  또는
#   {"decision":"allow"}

set -eu

# stdin 전체를 한 번에 읽는다.
INPUT="$(cat)"

# tool 이 bash/shell_exec 가 아니면 즉시 allow.
TOOL=$(printf '%s' "$INPUT" | sed -n 's/.*"tool":"\([^"]*\)".*/\1/p')
case "$TOOL" in
  bash|shell_exec|shell|exec) ;;
  *) printf '{"decision":"allow"}\n'; exit 0 ;;
esac

# command 필드 추출 (따옴표/이스케이프는 대략 처리).
CMD=$(printf '%s' "$INPUT" | sed -n 's/.*"command":"\(\([^"\\]\|\\.\)*\)".*/\1/p')
if [ -z "$CMD" ]; then
  printf '{"decision":"allow"}\n'
  exit 0
fi

deny() {
  # $1 = reason
  # JSON-safe escape (quotes + backslashes)
  msg=$(printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g')
  printf '{"decision":"deny","reason":"%s","systemMessage":"차단됨: %s"}\n' "$msg" "$msg"
  exit 0
}

# 패턴 매칭 (grep -E). 개별 패턴은 보수적으로 좁게 유지 — false positive 최소화.
# rm -rf 루트·시스템 디렉토리 차단 (두 패턴으로 분리)
# 패턴 A: rm -r[f] / 또는 rm -r[f] /*
echo "$CMD" | grep -Eq '(^|[^A-Za-z_])rm[[:space:]]+(-[[:alnum:]]*[rRfF][[:alnum:]]*[[:space:]]+)+(-[[:alnum:]]+[[:space:]]+)*(/\*?([[:space:]]|$))' \
  && deny "rm -rf 루트 경로"
# 패턴 B: rm -r[f] /시스템경로 (서브디렉토리 포함: /etc, /var/log, /usr/bin 등)
echo "$CMD" | grep -Eq '(^|[^A-Za-z_])rm[[:space:]]+(-[[:alnum:]]*[rRfF][[:alnum:]]*[[:space:]]+)+(-[[:alnum:]]+[[:space:]]+)*/(etc|var|usr|bin|sbin|boot|root|proc|sys|dev)' \
  && deny "rm -rf 시스템 경로"
# 패턴 C: rm -r[f] ~ 또는 rm -r[f] $HOME (홈 디렉토리 전체 삭제)
echo "$CMD" | grep -Eq '(^|[^A-Za-z_])rm[[:space:]]+(-[[:alnum:]]*[rRfF][[:alnum:]]*[[:space:]]+)+(-[[:alnum:]]+[[:space:]]+)*(~|\$HOME)([[:space:]]|$)' \
  && deny "rm -rf 홈 디렉토리"

# dd of=/dev/sdX 또는 /dev/nvmeX — 디스크 원본 덮어쓰기
echo "$CMD" | grep -Eq 'dd[[:space:]]+.*of=/dev/(sd[a-z]|nvme|hd[a-z]|xvd)' \
  && deny "dd 디스크 블록 디바이스 쓰기"

# mkfs — 파일시스템 포맷
echo "$CMD" | grep -Eq '(^|[^A-Za-z_])mkfs(\.[a-z0-9]+)?[[:space:]]+/dev/' \
  && deny "mkfs 디바이스 포맷"

# fork bomb :(){ :|:& };:
echo "$CMD" | grep -Eq ':\(\)[[:space:]]*\{[[:space:]]*:[[:space:]]*\|[[:space:]]*:' \
  && deny "fork bomb 패턴"

# chmod -R 777 / 또는 시스템 경로
echo "$CMD" | grep -Eq 'chmod[[:space:]]+(-R[[:space:]]+)?777[[:space:]]+(/|/\*|/etc|/var|/usr|/bin|/sbin|/root)(/\*)?([[:space:]]|$)' \
  && deny "chmod 777 시스템 경로"

# chown -R ... / 또는 시스템 경로 (소유자 일괄 변경)
echo "$CMD" | grep -Eq 'chown[[:space:]]+-R[[:space:]]+[^[:space:]]+[[:space:]]+(/|/etc|/var|/usr|/bin|/sbin|/root)(/\*)?([[:space:]]|$)' \
  && deny "chown -R 시스템 경로"

# iptables -F / ufw disable — 방화벽 전체 해제
echo "$CMD" | grep -Eq '(^|[^A-Za-z_])iptables[[:space:]]+(-F|--flush)([[:space:]]|$)' \
  && deny "iptables 전체 flush"
echo "$CMD" | grep -Eq '(^|[^A-Za-z_])ufw[[:space:]]+disable([[:space:]]|$)' \
  && deny "ufw 방화벽 비활성화"

# > /dev/sda 직접 리다이렉션
echo "$CMD" | grep -Eq '>[[:space:]]*/dev/(sd[a-z]|nvme|hd[a-z]|xvd)' \
  && deny "디스크 디바이스로 리다이렉션"

# 통과
printf '{"decision":"allow"}\n'

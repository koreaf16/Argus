import re, json, sys

logfile = sys.argv[1] if len(sys.argv) > 1 else r'C:/Dev/Argus/oracle-test.log'
with open(logfile, 'rb') as f:
    raw = f.read().decode('utf-16-le', errors='replace')

parts = re.split(r'(?=\{"ts":")', raw)
events = []
for p in parts:
    p = p.strip().replace('\n','').replace('\r','')
    if not p.startswith('{"ts":"'):
        continue
    try:
        obj, _ = json.JSONDecoder().raw_decode(p)
        events.append(obj)
    except:
        continue

print(f"=== 총 이벤트: {len(events)} ===\n")

# 타임라인 요약
SHOW_TYPES = {
    'turn.start', 'turn.done', 'tool.call.start', 'tool.call.done',
    'aidebug.decision', 'aidebug.tool_approval_request', 'error', 'exit'
}
issues = []
tool_stats = {}

for i, e in enumerate(events):
    t = e.get('type', '')
    d = e.get('data', {})

    if t == 'tool.call.start':
        name = d.get('tool', d.get('name', '?'))
        servers = d.get('servers', [])
        inp = d.get('input', {})
        tool_stats[name] = tool_stats.get(name, 0) + 1
        print(f"  CALL  {name:25s} servers={servers}")

    elif t == 'tool.call.done':
        name = d.get('tool', d.get('name', '?'))
        dur = d.get('duration_ms', '?')
        err = d.get('error', '')
        status = 'OK' if not err else f'ERR:{err[:60]}'
        print(f"  DONE  {name:25s} {dur}ms  {status}")

    elif t == 'aidebug.decision':
        tool = d.get('tool', '?')
        decision = d.get('decision', '?')
        handled = d.get('handled_by', '')
        reason = d.get('reason', '')
        print(f"  GATE  {tool:25s} {decision}  {handled}  {reason[:50]}")
        if decision == 'deny':
            issues.append(f"[AUTO-DENY] {tool}: {reason}")

    elif t == 'error':
        stage = d.get('stage', '')
        err = d.get('error', '')
        print(f"  ERR   {stage:25s} {err[:80]}")
        issues.append(f"[ERROR] {stage}: {err[:80]}")

    elif t == 'turn.done':
        reason = d.get('stop_reason', '')
        turns = d.get('turn_index', '')
        print(f"\n--- turn.done turn={turns} stop={reason} ---\n")

    elif t == 'turn.start':
        inp = d.get('user_input', '')[:60]
        print(f"\n=== TURN START: {inp!r} ===\n")

    elif t == 'exit':
        print(f"\n=== EXIT code={d.get('code')} ===")

print("\n=== 도구 호출 통계 ===")
for k, v in sorted(tool_stats.items(), key=lambda x: -x[1]):
    print(f"  {k:30s}: {v}회")

print(f"\n=== 발견된 이슈 ({len(issues)}개) ===")
for iss in issues:
    print(f"  {iss}")

# lane-debug
lane_lines = []
for p in re.split(r'(?=\{"ts":")', raw):
    p = p.strip()
    for line in p.split('\n'):
        if '[channel-debug]' in line:
            lane_lines.append(line.strip())

if lane_lines:
    print(f"\n=== LANE DEBUG ({len(lane_lines)}줄) ===")
    for l in lane_lines[:30]:
        print(f"  {l}")

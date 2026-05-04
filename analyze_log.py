import json
import re
from collections import Counter

path = r'C:/Users/jhkwa/AppData/Local/Temp/claude/C--Dev-Argus/8562f50f-c337-4a7d-be60-1ad8e3e52228/tasks/bmgr9o70a.output'
with open(path, 'r', encoding='utf-8', errors='replace') as f:
    raw = f.read()

# PowerShell line-wrapped each JSON record at col 174, injecting \n inside strings.
# Split on the start-of-record marker, strip injected newlines, parse each chunk.
parts = re.split(r'(?=\{"ts":")', raw)
events = []
for p in parts:
    p = p.strip()
    if not p.startswith('{"ts":"'):
        continue
    # Strip noise after JSON ends — keep up to first balanced JSON.
    cleaned = p.replace('\n', '').replace('\r', '')
    dec = json.JSONDecoder()
    try:
        obj, _ = dec.raw_decode(cleaned)
        events.append(obj)
    except json.JSONDecodeError:
        continue

print(f"Total events parsed: {len(events)}")
print()
types = Counter(e.get("type") for e in events)
for t, c in types.most_common():
    print(f"  {t}: {c}")

print()
print("=== Tool calls (start) ===")
for e in events:
    if e.get("type") == "tool.call.start":
        d = e.get("data", {})
        tool = d.get("tool", "?")
        inp = d.get("input", {})
        srv = inp.get("server", "?")
        cmd = str(inp.get("command", inp.get("path", inp.get("source", inp.get("source_path", inp.get("src", "")))))).replace("\n", " ")[:100]
        as_user = inp.get("as_user", "")
        au = f" as={as_user}" if as_user else ""
        print(f"  [{e['ts'][11:19]}] seq={e.get('seq'):>3} {tool:18} server={srv:<15}{au} {cmd}")

print()
print("=== Errors ===")
for e in events:
    if e.get("type") == "error":
        d = e.get("data", {})
        print(f"  [{e['ts'][11:19]}] seq={e.get('seq')} stage={d.get('stage','?')} tool={d.get('tool','?')}")
        print(f"     error: {str(d.get('error',''))[:400]}")

print()
print("=== Tool outputs hinting failure ===")
for e in events:
    if e.get("type") == "tool.call.output":
        d = e.get("data", {})
        out = str(d.get("output", ""))
        lo = out.lower()
        if any(k in lo for k in ["\"code\":1", "stderr", "error", "fail", "denied", "not found", "no such", "permission"]):
            print(f"  [{e['ts'][11:19]}] seq={e.get('seq')}")
            print(f"     {out[:500]}")
            print()

print()
print("=== Notices ===")
for e in events:
    if e.get("type") == "notice":
        d = e.get("data", {})
        msg = d.get("message", "")
        if any(k in msg.lower() for k in ["error", "fail", "warn", "block"]):
            print(f"  [{e['ts'][11:19]}] {d.get('category','?')}: {msg[:300]}")

print()
print("=== Last 3 events ===")
for e in events[-3:]:
    print(f"  [{e['ts'][11:19]}] type={e.get('type')} seq={e.get('seq')}")
    d = e.get("data", {})
    s = str(d)[:400]
    print(f"     {s}")

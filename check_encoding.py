import re, json
with open(r'C:/Dev/Argus/mutation-test2.log', 'rb') as f:
    raw_bytes = f.read()
raw = raw_bytes.decode('utf-16-le', errors='replace')
parts = re.split(r'(?=\{\"ts\":\")', raw)
for p in parts:
    p = p.strip()
    if not p.startswith('{"ts":"'):
        continue
    cleaned = p.replace('\n', '').replace('\r', '')
    try:
        obj, _ = json.JSONDecoder().raw_decode(cleaned)
    except Exception:
        continue
    if obj.get('type') == 'turn.start':
        ui = obj.get('data', {}).get('user_input', '')
        with open(r'C:/Dev/Argus/_input_check.txt', 'w', encoding='utf-8') as out:
            out.write(ui)
        print('utf-8 hex:', ui.encode('utf-8').hex())
        print('chars:', [hex(ord(c)) for c in ui[:25]])
        print('text:', ui)
        break

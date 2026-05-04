import requests
import json

url = "http://192.168.0.3:11435/v1/chat/completions"
payload = {
    "model": "/models/gemma-4-26B-A4B-it-AWQ-4bit",
    "messages": [{"role": "user", "content": "1부터 100까지 소수를 찾는 파이썬 코드를 짜줘. 단계별로 생각해서 작성해."}],
    "stream": True
}

try:
    response = requests.post(url, json=payload, stream=True, timeout=20)
    with open("scratch/gemma_stream_extended.txt", "w", encoding="utf-8") as f:
        count = 0
        for line in response.iter_lines():
            if line:
                decoded_line = line.decode('utf-8')
                f.write(decoded_line + "\n")
                count += 1
                if count > 300: # 300개 패킷 캡처
                    break
    print(f"Captured {count} packets to scratch/gemma_stream_extended.txt")
except Exception as e:
    print(f"Error: {e}")

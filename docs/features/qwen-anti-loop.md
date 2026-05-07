# Qwen3.5/3.6 추론 무한루프 대응 가이드

## 개요

Qwen3.5/3.6 시리즈(Qwen3.6-35B-A3B 포함)는 thinking 단계에서 같은 문장을 무한 반복하다 closing tag를 못 뱉는 알려진 결함이 있다. 깨진 응답, 절단된 tool_call JSON, 컨텍스트 토큰 소진을 유발한다.

이 문서는 다음 두 축의 대응책을 정리한다.

1. **Argus 측 변경** (이미 적용 — 이 저장소 코드)
2. **추론 서버측 권장 설정** (사용자가 ollama/vLLM/llama.cpp에 적용)

## Argus 측 변경 (적용 완료)

### 1. Qwen3.5/3.6 sampling default 주입

`internal/services/llm/openai.go`의 `isQwen36Model` 분기에서 다음 값을 자동 주입한다.

| 파라미터 | 값 | 근거 |
|---|---|---|
| temperature | 0.2 | greedy(=0) 금지하면서 deterministic 유지 |
| top_p | 0.95 | Qwen3 공식 권장 |
| top_k | 20 | Qwen3 공식 권장 |
| min_p | 0.0 | Qwen3 공식 권장 |
| presence_penalty | 1.5 | Qwen3.6-35B-A3B 모델카드 권장 |
| repetition_penalty | 1.0 | Qwen3 공식 권장 |

> Qwen2.x 등 레거시 Qwen은 별도 분기에서 기존 동작(presence_penalty=0)을 유지한다.

### 2. greedy decoding 강제 제거

`internal/query/engine_run.go`에서 `Temperature=0.0`, `TopP=1.0` 하드코딩을 제거. 어댑터가 모델별 default를 적용하도록 위임.

### 3. ModelEntry.Sampling override 지원

`.Argus/models.json`에서 모델별 sampling을 override할 수 있다.

```json
{
  "alias": "models-qwen3-6-35b-a3b-...",
  "model_id": "/models/Qwen3.6-35B-A3B-...",
  "sampling": {
    "temperature": 0.2,
    "top_p": 0.95,
    "top_k": 20,
    "min_p": 0.0,
    "presence_penalty": 1.5,
    "repetition_penalty": 1.0
  }
}
```

우선순위: **Request 필드 > ModelEntry.Sampling > 모델별 default**.

## 추론 서버측 권장 설정

Argus에서 보낸 sampling parameter는 추론 서버가 그대로 따라야 의미가 있다. 또한 chat template 측 알려진 버그는 서버 측에서만 수정 가능하다.

### A. Chat template 교체 (vLLM/llama.cpp 모두)

Qwen3.6 공식 chat template은 다음 버그를 가진다.

- empty `<think>` block을 historical assistant turn마다 emit → prompt drift, KV-cache invalidation
  ([QwenLM/Qwen3.6 issue #131](https://github.com/QwenLM/Qwen3.6/issues/131))
- tool_call 중간에 thinking 끼어들면 opening tag 안 닫고 closing tag만 generate
  ([HF Qwen3.5-35B-A3B discussion #4](https://huggingface.co/Qwen/Qwen3.5-35B-A3B/discussions/4))

수정 템플릿 옵션:

- **`allanchan339/vLLM-Qwen3-3.5-3.6-chat-template-fix`** — Qwen3.5/3.6 전용, vLLM과 호환
- **`froggeric/Qwen-Fixed-Chat-Templates`** — "21-fix" 버전, llama.cpp/Open WebUI/vLLM 모두 호환

### B. preserve_thinking=true 활성화

ollama Modelfile 또는 vLLM serving args에서 활성화. tool_use + looping 동시 해결 보고 ([HF Qwen3.6-35B-A3B discussion #51](https://huggingface.co/Qwen/Qwen3.6-35B-A3B/discussions/51)).

#### Ollama Modelfile 예시

```
FROM /models/Qwen3.6-35B-A3B-Claude-4.7-Opus-Reasoning-Distilled-AWQ-INT4
PARAMETER preserve_thinking true
PARAMETER temperature 0.2
PARAMETER top_p 0.95
PARAMETER top_k 20
PARAMETER min_p 0.0
PARAMETER presence_penalty 1.5
PARAMETER repetition_penalty 1.0
```

#### vLLM serving args 예시

```
--preserve-thinking \
--enable-reasoning \
--reasoning-parser deepseek_r1
```

### C. Reasoning budget 제한 (llama.cpp 백엔드)

```
--reasoning-budget 4096
--reasoning-budget-message "OK, I've thought long enough. Let's answer."
```

thinking 토큰이 한도에 도달하면 자연스럽게 종료시켜 무한 thinking을 막는다. ollama가 llama.cpp 기반이라면 `OLLAMA_*` env 또는 Modelfile PARAMETER로 노출되는지 ollama 버전에 따라 확인 필요.

### D. Argus와 추론 서버의 sampling 값 일치

Argus가 보낸 값이 우선이지만, 서버 default를 같은 값으로 설정해두면 다른 클라이언트(직접 API 호출 등)에서도 일관된 동작을 얻고, Argus 패치 누락 시 안전망 역할을 한다.

## 적용 대상 환경

| 항목 | 값 |
|---|---|
| 추론 서버 호스트 | `192.168.0.3:11434` (Ollama 추정) |
| 활성 모델 | `Qwen3.6-35B-A3B-Claude-4.7-Opus-Reasoning-Distilled-AWQ-INT4` |
| 백엔드 | Ollama (llama.cpp 기반) |

## 검증 방법

### Argus 측 (패치 적용 후)

```powershell
.\Argus.exe --aidebug -p "Oracle PDB 정보 조회 sqlplus 명령어 알려줘"
```

기대: thinking이 한 번만 나오고 정상 종료. 응답이 합리적 길이.

회귀 (패치 적용 전): 같은 thinking 텍스트("The user is asking about PDB...")가 수십 번 반복되며 응답 깨짐.

### 추론 서버 측

1. **Chat template 검증**: 같은 multi-turn 대화로 두 번 호출 → request payload 캡처 → empty `<think>` block 누적 여부 확인.
2. **preserve_thinking 검증**: tool_call 시나리오에서 arguments JSON 절단 빈도 측정.
3. **Reasoning budget 검증**: 매우 어려운 추론 prompt로 thinking이 한도에서 자연스럽게 종료되는지 확인.

## 참고 출처

- [Qwen3.6 Issue #88 — repetition/looping in LiveCodeBench](https://github.com/QwenLM/Qwen3.6/issues/88)
- [Qwen3.6 Issue #131 — chat template empty `<think>` blocks](https://github.com/QwenLM/Qwen3.6/issues/131)
- [Qwen3.6 Issue #145 — endless reasoning loops with provided sampling parameters](https://github.com/QwenLM/Qwen3.6/issues/145)
- [HF Qwen3.6-35B-A3B discussion #19/#20/#51](https://huggingface.co/Qwen/Qwen3.6-35B-A3B/discussions)
- [HF Qwen3.5-35B-A3B discussion #4 — chat template tool calling broken](https://huggingface.co/Qwen/Qwen3.5-35B-A3B/discussions/4)
- [allanchan339/vLLM-Qwen3-3.5-3.6-chat-template-fix](https://github.com/allanchan339/vLLM-Qwen3-3.5-3.6-chat-template-fix)
- [froggeric/Qwen-Fixed-Chat-Templates](https://huggingface.co/froggeric/Qwen-Fixed-Chat-Templates)
- [Qwen3 best practices (sampling)](https://qwenlm.github.io/blog/qwen3/)

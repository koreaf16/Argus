# Phase 7 설계도 — RAG + 로컬 자가학습 벡터 검색

## 목표
사용자가 대화로 외부 DB의 벡터 테이블을 지식 소스로 등록.
**Phase 6의 knowledge_base에 벡터 임베딩을 추가하여, 로컬 SQLite에서도 벡터 유사도 검색 가능.**
검색 우선순위: 로컬 지식(0순위) → 외부 RAG(1순위) → 웹(2순위) → LLM(3순위).

## 선행: Phase 6 완료

---

## 추가/변경 파일

```
internal/
├── rag/
│   ├── manager.go          # RAG 소스 관리 (등록/삭제/우선순위)
│   ├── search.go           # 벡터 유사도 검색 실행
│   ├── embedding.go        # 임베딩 생성 (Ollama/외부 API)
│   └── local_search.go     # 로컬 knowledge_base 벡터 검색
├── tools/rag_search.go     # Phase 6에서 준비한 도구에 실제 로직 연결
├── agent/knowledge.go      # knowledge_base 임베딩 생성 + 벡터 검색 통합
└── storage/sqlite.go       # rag_sources 테이블 + sqlite-vec 초기화
```

---

## RAG 소스 등록 흐름

```
사용자: "사내 매뉴얼이 DB에 있어, RAG로 쓸게"
  ↓
LLM 질문 (OnInput 콜백):
  ? 어떤 서버의 DB? → db-server
  ? DB 종류? → Oracle
  ? 접속 계정? → rag_user / ****
  ↓
SSH → db-server: sqlplus로 접속, 벡터 컬럼이 있는 테이블 조회:
  SELECT table_name, column_name, data_type 
  FROM user_tab_columns 
  WHERE data_type = 'VECTOR'
  (또는 BLOB 타입 + 테이블명에 'vec', 'embed' 포함하는 것 탐색)
  ↓
결과 목록 표시:
  1. OPS_MANUAL_VECTORS (doc_text, embedding, category)
  2. INCIDENT_HISTORY_VEC (description, vec_embedding, resolution)
  ↓
사용자: "둘 다 등록"
  ↓
각 테이블에 대해:
  ? 검색할 텍스트 컬럼: doc_text
  ? 벡터 컬럼: embedding
  ? 결과에 포함할 컬럼: doc_text, category
  ? 용도 설명: 사내 운영 매뉴얼
  ↓
SQLite rag_sources 테이블에 저장
  ↓
rag_search 도구 활성화
  ↓
"앞으로 다음 순서로 검색합니다:
  1순위: OPS_MANUAL_VECTORS, INCIDENT_HISTORY_VEC
  2순위: 웹 검색
  3순위: LLM 지식"
```

---

## 임베딩 생성

사용자 질문을 벡터로 변환하여 DB에서 유사도 검색에 사용.

### 방법 1: Ollama 임베딩 모델
```
POST http://localhost:11434/api/embeddings
{
  "model": "all-minilm",
  "prompt": "ORA-01555 해결 방법"
}
→ { "embedding": [0.123, -0.456, ...] }
```

### 방법 2: 외부 임베딩 API
OpenAI, Cohere 등의 임베딩 API 호출.
설정으로 선택:
```yaml
embedding:
  provider: "ollama"           # ollama | openai | custom
  endpoint: "http://localhost:11434"
  model: "all-minilm"
```

### 임베딩 모델은 RAG 소스의 임베딩 모델과 동일해야 함
등록 시 "어떤 임베딩 모델로 생성된 벡터인가요?" 확인 필요.
불일치 시 경고.

---

## 벡터 검색 실행

### Oracle 벡터 검색
```sql
SELECT {result_columns}
FROM {table_name}
ORDER BY VECTOR_DISTANCE({vector_column}, :query_vector, COSINE)
FETCH FIRST 5 ROWS ONLY
```
- query_vector: 사용자 질문의 임베딩
- 벡터를 SQL에 전달하는 방법: VECTOR() 함수 또는 바인드 변수

### MySQL 벡터 검색 (MySQL 9.0+ HeatWave)
```sql
SELECT {result_columns}, 
  DISTANCE({vector_column}, STRING_TO_VECTOR(:query_vector), 'cosine') as dist
FROM {table_name}
ORDER BY dist LIMIT 5
```

### PostgreSQL 벡터 검색 (pgvector)
```sql
SELECT {result_columns}
FROM {table_name}
ORDER BY {vector_column} <=> :query_vector
LIMIT 5
```

### 실행 방식
SSH → 서버: 해당 DB CLI 도구로 쿼리 실행
벡터 값을 쉘 변수로 전달하거나, 임시 스크립트 생성 후 실행.

---

## 검색 우선순위 엔진

### rag_search 도구 내부 로직
```
1. RAG 소스 목록 (우선순위순)에서 순차 검색
2. 각 소스에 대해:
   a. 사용자 질문 → 임베딩 생성
   b. SSH로 대상 DB에 벡터 검색 쿼리 실행
   c. 결과가 있으면 수집
3. 모든 RAG 소스 결과를 합산
4. 결과를 LLM에 전달
```

### LLM 판단
- RAG 결과가 충분하면 → 그대로 답변
- RAG 결과가 불충분하면 → LLM이 web_search 추가 호출 판단
- RAG 소스가 없으면 → "RAG 소스가 등록되지 않았습니다" 반환

### 시스템 프롬프트에 추가
```
## Knowledge Search Priority
1. rag_search — ALWAYS try this first for troubleshooting, procedures, incident history
2. web_search — Only if RAG has no results
3. Your own knowledge — Last resort

Registered RAG sources:
- OPS_MANUAL_VECTORS (사내 운영 매뉴얼) @ db-server/Oracle
- INCIDENT_HISTORY_VEC (과거 장애 이력) @ db-server/Oracle
```

---

## 로컬 자가학습 벡터 검색 (knowledge_base 벡터화)

Phase 6에서 키워드 기반으로 동작하던 knowledge_base에 **벡터 임베딩**을 추가하여,
의미적 유사도 검색을 가능하게 한다.

### sqlite-vec 확장

로컬 SQLite에서 벡터 유사도 검색을 수행하기 위해 `sqlite-vec` 확장을 사용.

```
의존성: github.com/asg017/sqlite-vec (CGO) 또는 순수 Go 대안

대안 1: sqlite-vec (최적 성능)
  - C 확장, CGO 필요
  - ANN (Approximate Nearest Neighbor) 지원
  - 대량 데이터에서도 빠른 검색

대안 2: 순수 Go 코사인 유사도 (간단)
  - CGO 불필요 (modernc.org/sqlite와 호환)
  - BLOB으로 벡터 저장 → Go에서 코사인 유사도 계산
  - knowledge_base 규모가 수백~수천 건이므로 충분한 성능
  - 배포 단순성 유지 (CGO 불필요)
```

**권장: 대안 2** (순수 Go 코사인 유사도)
- InfraCtl의 knowledge_base는 수백~수천 건 수준이므로 ANN이 불필요
- modernc.org/sqlite와의 일관성 유지 (CGO 없는 단일 바이너리)
- 벡터를 BLOB으로 저장하고, 조회 시 Go로 코사인 유사도 계산

### 임베딩 생성 시점

```
knowledge_base에 새 지식 저장 시 (Phase 6의 자동 학습 또는 수동 등록)
  ↓
situation + resolution 텍스트를 임베딩 모델에 전달
  ↓
생성된 벡터를 embedding BLOB 컨럼에 저장
  ↓
이후 유사 상황 발생 시 벡터 유사도 검색으로 찾음
```

### 로컬 벡터 검색 로직

```go
// 의사코드
func SearchLocalKnowledge(query string, topK int) []KnowledgeEntry {
    // 1. 사용자 질문을 임베딩으로 변환
    queryVec := embedding.Generate(query)
    
    // 2. knowledge_base에서 모든 임베딩 로드
    entries := db.Query("SELECT id, situation, resolution, embedding FROM knowledge_base WHERE embedding IS NOT NULL")
    
    // 3. 코사인 유사도 계산 + 상위 K건 반환
    results := cosineSimilarityTopK(queryVec, entries, topK)
    
    // 4. 신뢰도 가중치 적용
    for r := range results {
        r.Score *= r.Confidence  // confidence가 높을수록 우선
    }
    return results
}
```

---

## 검색 우선순위 엔진 (변경)

### 기존 (Phase 6 시점)
```
1순위: 외부 RAG (DB 벡터 테이블)
2순위: 웹 검색
3순위: LLM 자체 지식
```

### 변경 (Phase 7 완료 후)
```
0순위: 로컬 knowledge_base 벡터 검색 (자가학습 지식)  ← 새로 추가
1순위: 외부 RAG (사내 매뉴얼/장애이력 DB)
2순위: 웹 검색
3순위: LLM 자체 지식
```

### 로컬 지식을 0순위로 하는 이유
- **가장 빠름**: 로컬 SQLite에서 즉시 검색 (SSH/네트워크 불필요)
- **가장 정확**: 이 시스템에서 실제로 경험한 에러/해결 패턴
- **비용 없음**: LLM API 호출이나 외부 DB 접속 불필요
- **결과 없으면** 자동으로 다음 순위로 넘어감

### rag_search 도구 통합 로직
```
1. 로컬 knowledge_base 벡터 검색 (similarity ≥ 0.7)
   → 결과 있으면: LLM에 "로컬 지식" 레이블로 전달
2. 외부 RAG 소스 순차 검색 (우선순위순)
   → SSH → DB 벡터 쿼리
3. 모든 결과 합산 → LLM에 전달
4. LLM이 부족하다고 판단하면 web_search 추가
```

---

## 에이전트 루프 통합: 도구 실행 전 지식 검색

Phase 7 완료 후, 에이전트 루프에 "도구 실행 전 로컬 지식 검색" 단계가 통합된다:

```
사용자 입력
  ↓
LLM이 도구 호출 결정
  ↓
[도구 실행 전] knowledge_base 벡터 검색:
  "이 도구/상황에 관련된 과거 학습 내용이 있습니다:
   - ORA-00942 에러 시 DBA_ 뷰 대신 USER_ 뷰 사용 (3회 활용)
   - 이 서버에서는 sysdba OS 인증이 부가능 (1회 학습)"
  ↓
LLM이 이 컨텍스트를 참고하여 처음부터 최적 명령 생성
  ↓
도구 실행
  ↓
결과를 execution_logs에 기록
  ↓
실패→성공 체인 발생 시 → knowledge_base에 새 지식 저장 (임베딩 포함)
```

### 시스템 프롬프트 변경

```
## Knowledge Search Priority
0. local_knowledge — ALWAYS check local knowledge_base first (learned patterns, tips)
1. rag_search — Registered external RAG sources for procedures, incident history
2. web_search — Only if local + RAG have no results
3. Your own knowledge — Last resort

Registered RAG sources:
- OPS_MANUAL_VECTORS (사내 운영 매뉴얼) @ db-server/Oracle
- INCIDENT_HISTORY_VEC (과거 장애 이력) @ db-server/Oracle

Local Knowledge Stats:
- 47 learned patterns, 12 tips, 3 procedures
- Most referenced: "ORA-00942 → USER_ 뷰 사용" (23회 활용)
```

---

## 검증 시나리오

1. "사내 매뉴얼 RAG 등록" → DB 접속 → 테이블 탐지 → 컬럼 확인 → 등록
2. "ORA-01555 해결법" → 로컬 지식 검색 → RAG 검색 → 매뉴얼 결과 → 답변
3. "Kafka 설정 방법" → 로컬/RAG에 없음 → 자동으로 웹 검색
4. "/rag" → 소스 목록
5. "우선순위 변경" → 동작
6. 이전에 ORA-00942로 실패한 적 있는 상황 → 로컬 벡터 검색으로 유사 패턴 찾음 → LLM이 처음부터 올바른 쿼리 생성
7. `/knowledge` → 로컬 지식 47건, 활용 통계 표시

---

## 완료 기준
- [ ] 대화로 외부 DB 벡터 테이블 RAG 등록
- [ ] 임베딩 생성 (Ollama 또는 외부)
- [ ] SSH로 원격 DB 벡터 검색 쿼리 실행
- [ ] 검색 우선순위: 로컬 지식(0순위) → RAG(1순위) → 웹(2순위) → LLM(3순위)
- [ ] /rag 관리 명령
- [ ] LLM이 RAG 결과 활용하여 답변
- [ ] knowledge_base 벡터 임베딩 생성 + 저장
- [ ] 로컬 SQLite 벡터 유사도 검색 (코사인 유사도)
- [ ] 에이전트 루프에 "도구 실행 전 지식 검색" 통합
- [ ] 로컬 지식 통계 표시 (use_count, 수량)

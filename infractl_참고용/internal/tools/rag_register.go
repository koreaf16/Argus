// Package tools
// File: rag_register.go
// Description: rag_register 도구 — 외부 DB 벡터 테이블을 RAG 소스로 등록
// Responsibility: LLM이 대화로 수집한 접속 정보를 rag_sources에 저장

package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/rag"
	"github.com/yourorg/infractl/internal/store"
)

// RAGRegisterTool은 외부 DB 벡터 테이블을 RAG 소스로 등록하는 도구이다.
type RAGRegisterTool struct {
	Store       store.RAGSourceStore
	ExecManager *executor.Manager
	Embedder    rag.EmbeddingGenerator
}

var _ Tool = (*RAGRegisterTool)(nil)

func (t *RAGRegisterTool) Name() string     { return "rag_register" }
func (t *RAGRegisterTool) IsReadOnly() bool { return false }
func (t *RAGRegisterTool) IsEnabled() bool  { return true }

func (t *RAGRegisterTool) Description() string {
	return "Register an external DB vector table as a RAG source with schema probe and dimension checks. " +
		"사용자가 '사내 매뉴얼 RAG로 등록', 'RAG 소스 추가' 등을 요청할 때 사용합니다. " +
		"등록 후 rag_search 도구가 해당 DB에서 벡터 검색을 수행합니다."
}

func (t *RAGRegisterTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{
				"type":        "string",
				"description": "RAG 소스 이름 (테이블명 또는 용도명)",
			},
			"server_name": map[string]interface{}{
				"type":        "string",
				"description": "대상 서버 이름 (server_list로 확인)",
			},
			"db_type": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"oracle", "mysql", "postgresql"},
				"description": "DB 종류",
			},
			"db_name": map[string]interface{}{
				"type":        "string",
				"description": "Oracle SID 또는 MySQL/PostgreSQL DB명",
			},
			"db_host": map[string]interface{}{
				"type":        "string",
				"description": "DB host (미입력 시 server_name 기반 기본값)",
			},
			"db_port": map[string]interface{}{
				"type":        "integer",
				"description": "DB port (미입력 시 DB 타입별 기본 포트)",
			},
			"schema_name": map[string]interface{}{
				"type":        "string",
				"description": "스키마명 (postgres/oracle 권장)",
			},
			"table_name": map[string]interface{}{
				"type":        "string",
				"description": "벡터 테이블명",
			},
			"metadata_table": map[string]interface{}{
				"type":        "string",
				"description": "메타데이터 테이블명 (옵션)",
			},
			"table_join_key": map[string]interface{}{
				"type":        "string",
				"description": "벡터 테이블 조인키 컬럼명 (옵션, metadata_table 사용 시 필수)",
			},
			"metadata_join_key": map[string]interface{}{
				"type":        "string",
				"description": "메타 테이블 조인키 컬럼명 (옵션, metadata_table 사용 시 필수)",
			},
			"metadata_columns": map[string]interface{}{
				"type":        "string",
				"description": "메타 테이블에서 결과에 붙일 컬럼 목록 (콤마 구분, 예: doc_name,doc_version)",
			},
			"text_column": map[string]interface{}{
				"type":        "string",
				"description": "검색 텍스트 컬럼명",
			},
			"vector_column": map[string]interface{}{
				"type":        "string",
				"description": "벡터 임베딩 컬럼명",
			},
			"result_columns": map[string]interface{}{
				"type":        "string",
				"description": "결과에 포함할 컬럼 목록 (콤마 구분, 예: doc_text,category)",
			},
			"embedding_model": map[string]interface{}{
				"type":        "string",
				"description": "벡터 생성에 사용된 임베딩 모델명 (예: all-minilm). 현재 설정과 불일치 시 경고.",
			},
			"description": map[string]interface{}{
				"type":        "string",
				"description": "용도 설명 (예: 사내 운영 매뉴얼)",
			},
			"username": map[string]interface{}{
				"type":        "string",
				"description": "DB 접속 계정",
			},
			"password": map[string]interface{}{
				"type":        "string",
				"description": "DB 접속 비밀번호",
			},
			"role": map[string]interface{}{
				"type":        "string",
				"description": "Oracle role (예: sysdba)",
			},
			"priority": map[string]interface{}{
				"type":        "integer",
				"description": "검색 우선순위 (1=가장 먼저, 숫자가 클수록 나중). 기본값: 1",
			},
		},
		"required": []string{"name", "server_name", "db_type", "db_name", "table_name",
			"text_column", "vector_column", "result_columns", "embedding_model", "username", "password"},
	}
}

func (t *RAGRegisterTool) Execute(ctx context.Context, args map[string]interface{}, _ executor.Executor) (ToolOutcome, error) {
	name, _ := args["name"].(string)
	serverName, _ := args["server_name"].(string)
	dbType, _ := args["db_type"].(string)
	dbName, _ := args["db_name"].(string)
	dbHost, _ := args["db_host"].(string)
	dbPort := argInt(args, "db_port", 0)
	schemaName, _ := args["schema_name"].(string)
	tableName, _ := args["table_name"].(string)
	metadataTable, _ := args["metadata_table"].(string)
	tableJoinKey, _ := args["table_join_key"].(string)
	metadataJoinKey, _ := args["metadata_join_key"].(string)
	metadataColumns, _ := args["metadata_columns"].(string)
	textCol, _ := args["text_column"].(string)
	vecCol, _ := args["vector_column"].(string)
	resultCols, _ := args["result_columns"].(string)
	embModel, _ := args["embedding_model"].(string)
	desc, _ := args["description"].(string)
	username, _ := args["username"].(string)
	password, _ := args["password"].(string)
	role, _ := args["role"].(string)
	dbType = strings.ToLower(strings.TrimSpace(dbType))
	serverName = strings.TrimSpace(serverName)
	dbName = strings.TrimSpace(dbName)
	tableName = strings.TrimSpace(tableName)
	textCol = strings.TrimSpace(textCol)
	vecCol = strings.TrimSpace(vecCol)

	if name == "" || serverName == "" || dbType == "" || tableName == "" ||
		username == "" || password == "" {
		msg := "필수 파라미터(name, server_name, db_type, table_name, username, password)가 누락되었습니다"
		return ToolOutcome{Content: msg, Success: false, ErrorMessage: msg}, nil
	}
	if metadataTable != "" && (tableJoinKey == "" || metadataJoinKey == "") {
		msg := "metadata_table 사용 시 table_join_key, metadata_join_key를 함께 지정해야 합니다"
		return ToolOutcome{Content: msg, Success: false, ErrorMessage: msg}, nil
	}
	if dbHost == "" {
		dbHost = serverName
	}

	priority := 1
	if v, ok := args["priority"].(float64); ok && int(v) > 0 {
		priority = int(v)
	}
	metadataCols := strings.TrimSpace(metadataColumns)

	probeReq := ragProbeRequest{
		ServerName:      serverName,
		DBType:          dbType,
		DBHost:          dbHost,
		DBPort:          dbPort,
		DBName:          dbName,
		SchemaName:      schemaName,
		TableName:       tableName,
		TextColumn:      textCol,
		VectorColumn:    vecCol,
		MetadataTable:   metadataTable,
		MetadataColumns: metadataCols,
		TableJoinKey:    tableJoinKey,
		MetadataJoinKey: metadataJoinKey,
		Username:        username,
		Password:        password,
		Role:            role,
	}
	probeResult, err := probeRAGSource(ctx, t.ExecManager, probeReq)
	if err != nil {
		msg := fmt.Sprintf("RAG 소스 등록 전 probe 실패: %v", err)
		return ToolOutcome{Content: msg, Success: false, ErrorMessage: msg}, nil
	}
	if len(probeResult.MissingColumns) > 0 {
		msg := fmt.Sprintf("RAG 소스 등록 실패: 메인 테이블 컬럼 누락 (%s)", strings.Join(probeResult.MissingColumns, ", "))
		return ToolOutcome{Content: msg, Success: false, ErrorMessage: msg}, nil
	}
	if len(probeResult.MissingMetaCols) > 0 {
		msg := fmt.Sprintf("RAG 소스 등록 실패: 메타 테이블 컬럼 누락 (%s)", strings.Join(probeResult.MissingMetaCols, ", "))
		return ToolOutcome{Content: msg, Success: false, ErrorMessage: msg}, nil
	}

	dimMessage := "unknown"
	if t.Embedder != nil {
		v, e := t.Embedder.Generate(ctx, "dimension probe")
		if e != nil {
			msg := fmt.Sprintf("RAG 소스 등록 실패: 임베딩 생성 실패 (%v)", e)
			return ToolOutcome{Content: msg, Success: false, ErrorMessage: msg}, nil
		}
		dimMessage = fmt.Sprintf("%d", len(v))
		if probeResult.VectorDimension > 0 && len(v) != probeResult.VectorDimension {
			msg := fmt.Sprintf("RAG 소스 등록 실패: 벡터 차원 불일치 (embedder=%d, db=%d)", len(v), probeResult.VectorDimension)
			return ToolOutcome{Content: msg, Success: false, ErrorMessage: msg}, nil
		}
	}

	source := store.RAGSource{
		Name:            name,
		ServerName:      serverName,
		DBType:          dbType,
		DBHost:          dbHost,
		DBPort:          dbPort,
		DBName:          dbName,
		SchemaName:      schemaName,
		TableName:       tableName,
		MetadataTable:   metadataTable,
		TableJoinKey:    tableJoinKey,
		MetadataJoinKey: metadataJoinKey,
		MetadataColumns: metadataCols,
		TextColumn:      textCol,
		VectorColumn:    vecCol,
		ResultColumns:   resultCols,
		EmbeddingModel:  embModel,
		Description:     desc,
		Priority:        priority,
		Credentials: store.RAGSourceCredentials{
			Username: username,
			Password: password,
			Role:     role,
		},
	}

	id, err := t.Store.SaveRAGSource(ctx, source)
	if err != nil {
		msg := fmt.Sprintf("RAG 소스 등록 실패: %s", err)
		return ToolOutcome{Content: msg, Success: false, ErrorMessage: msg}, nil
	}

	joinMsg := "미사용"
	if metadataTable != "" {
		joinMsg = fmt.Sprintf("%s (t.%s = m.%s)", metadataTable, tableJoinKey, metadataJoinKey)
	}
	content := fmt.Sprintf(
		"✓ RAG 소스 등록 완료 (ID: %d)\n"+
			"이름: %s\n서버: %s / %s / %s\n"+
			"DB: host=%s port=%d schema=%s\n"+
			"메타 조인: %s\n"+
			"벡터 차원: db=%d, embedder=%s\n"+
			"우선순위: %d순위\n"+
			"앞으로 rag_search 도구가 이 소스를 %d순위로 검색합니다.",
		id, name, serverName, dbType, tableName, dbHost, dbPort, schemaName, joinMsg,
		probeResult.VectorDimension, dimMessage, priority, priority,
	)
	return ToolOutcome{Content: content, Success: true}, nil
}

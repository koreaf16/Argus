// Package infrainit
// File: wizard.go
// Description: infractl init 대화형 LLM 등록 위저드 전체 흐름 조율
// Responsibility: 단계별 탐색(뒤로 가기) 및 기존 설정을 지원하는 설정 위저드

package infrainit

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/yourorg/infractl/internal/config"
	"github.com/yourorg/infractl/internal/tui"
)

type Wizard struct {
	ctx      context.Context
	cfg      *config.Config
	multiLLM bool
	steps    []func() (bool, error) // true=next, false=back
	curStep  int
}

func Run(ctx context.Context) error {
	dir, err := config.DefaultConfigDir()
	if err != nil {
		return fmt.Errorf("get config dir: %w", err)
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	// 기존 설정 로드
	cfg, err := config.Load()
	if err != nil {
		cfg = &config.Config{}
	}

	w := &Wizard{
		ctx: ctx,
		cfg: cfg,
	}

	fmt.Println()
	borderColor := lipgloss.NewStyle().Foreground(tui.ColorGeminiBox)
	width := 50
	sep := strings.Repeat("─", width)
	fmt.Println(borderColor.Render("╭"+sep+"╮"))
	fmt.Println(borderColor.Render("│") + " " +
		tui.StyleGeminiHeader.Render("infractl") +
		tui.StyleGeminiSubDesc.Render(" — 초기 설정 위저드"))
	fmt.Println(borderColor.Render("╰"+sep+"╯"))
	fmt.Println(tui.StyleGeminiHint.Render("  힌트: 언제든 Ctrl+B를 눌러 이전 단계로 돌아갈 수 있습니다."))
	fmt.Println()

	// 단계 정의
	w.steps = []func() (bool, error){
		w.stepLLMMode,
		w.stepGeneralLLM,
		w.stepReasoningLLM,
		w.stepFastLLM,
		w.stepEmbedding,
		w.stepReranker,
	}

	for w.curStep < len(w.steps) {
		next, err := w.steps[w.curStep]()
		if err != nil {
			return err
		}
		if next {
			w.curStep++
		} else {
			if w.curStep > 0 {
				w.curStep--
			}
		}
	}

	if err := config.Save(w.cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	path := filepath.Join(dir, "config.yaml")
	printSuccess("설정 저장 완료: " + path)
	return nil
}

func (w *Wizard) stepLLMMode() (bool, error) {
	opts := []tui.SelectOption{
		{
			Label:       "단일 LLM",
			Description: "모든 작업에 하나의 모델을 사용합니다. 일반적인 용도에 충분합니다.",
			HideOther:   true,
		},
		{
			Label:       "멀티 LLM (티어 분리)",
			Description: "reasoning / general / fast 세 역할에 각각 다른 모델을 설정합니다.",
			HideOther:   true,
		},
	}

	result := tui.RunSelect("[1단계] LLM 구성 방식을 선택하세요", opts, 80)
	if result.IsBack {
		return false, nil // 첫 단계라 뒤로 가기 의미 없지만 일관성 유지
	}
	w.multiLLM = result.Index == 1
	return true, nil
}

func (w *Wizard) stepGeneralLLM() (bool, error) {
	title := "LLM 설정 (모든 작업에 사용)"
	if w.multiLLM {
		title = "General LLM 설정 (필수 — 에이전트의 기본 실행 모델)"
	}
	printSectionHeader("2단계", title)

	defEndpoint := "http://localhost:11434/v1"
	defModel := "qwen3:7b"
	defKey := ""

	if w.cfg.Models.General != nil {
		defEndpoint = w.cfg.Models.General.Endpoint
		defModel = w.cfg.Models.General.Model
		defKey = w.cfg.Models.General.APIKey
	} else if w.cfg.LLM.Endpoint != "" {
		defEndpoint = w.cfg.LLM.Endpoint
		defModel = w.cfg.LLM.Model
		defKey = w.cfg.LLM.APIKey
	}

	cfg, back := w.promptLLMConfig(defEndpoint, defModel, defKey)
	if back {
		return false, nil
	}
	w.cfg.Models.General = cfg
	return true, nil
}

func (w *Wizard) stepReasoningLLM() (bool, error) {
	if !w.multiLLM {
		return true, nil // 건너뜀
	}
	printSectionHeader("3단계", "Reasoning LLM 설정 (선택 — 복잡한 분석·추론 작업용)")

	defEndpoint := w.cfg.Models.General.Endpoint
	defModel := "qwen3:27b"
	defKey := w.cfg.Models.General.APIKey
	useReasoning := false

	if w.cfg.Models.Reasoning != nil {
		defEndpoint = w.cfg.Models.Reasoning.Endpoint
		defModel = w.cfg.Models.Reasoning.Model
		defKey = w.cfg.Models.Reasoning.APIKey
		useReasoning = true
	}

	ok, back := promptYN("별도 설정하시겠습니까?", useReasoning)
	if back {
		return false, nil
	}
	if !ok {
		w.cfg.Models.Reasoning = nil
		return true, nil
	}

	cfg, back := w.promptLLMConfig(defEndpoint, defModel, defKey)
	if back {
		return false, nil
	}
	w.cfg.Models.Reasoning = cfg
	return true, nil
}

func (w *Wizard) stepFastLLM() (bool, error) {
	if !w.multiLLM {
		return true, nil // 건너뜀
	}
	printSectionHeader("4단계", "Fast LLM 설정 (선택 — 단순 조회·포맷팅용 경량 모델)")

	defEndpoint := w.cfg.Models.General.Endpoint
	defModel := "qwen3:1.7b"
	defKey := w.cfg.Models.General.APIKey
	useFast := false

	if w.cfg.Models.Fast != nil {
		defEndpoint = w.cfg.Models.Fast.Endpoint
		defModel = w.cfg.Models.Fast.Model
		defKey = w.cfg.Models.Fast.APIKey
		useFast = true
	}

	ok, back := promptYN("별도 설정하시겠습니까?", useFast)
	if back {
		return false, nil
	}
	if !ok {
		w.cfg.Models.Fast = nil
		return true, nil
	}

	cfg, back := w.promptLLMConfig(defEndpoint, defModel, defKey)
	if back {
		return false, nil
	}
	w.cfg.Models.Fast = cfg
	return true, nil
}

func (w *Wizard) stepEmbedding() (bool, error) {
	step := "5단계"
	if !w.multiLLM {
		step = "3단계"
	}
	printSectionHeader(step, "임베딩 설정 (선택 — RAG 지식 베이스 검색용)")

	useEmb := w.cfg.Embedding.Endpoint != ""
	ok, back := promptYN("사용하시겠습니까?", useEmb)
	if back {
		return false, nil
	}
	if !ok {
		w.cfg.Embedding = config.EmbeddingConfig{}
		return true, nil
	}

	defEndpoint := "http://localhost:11434"
	defModel := "all-minilm"
	defKey := ""
	if w.cfg.Embedding.Endpoint != "" {
		defEndpoint = w.cfg.Embedding.Endpoint
		defModel = w.cfg.Embedding.Model
		defKey = w.cfg.Embedding.APIKey
	}

	cfg, back := w.promptEmbeddingConfig(defEndpoint, defModel, defKey)
	if back {
		return false, nil
	}
	w.cfg.Embedding = cfg
	return true, nil
}

func (w *Wizard) stepReranker() (bool, error) {
	step := "6단계"
	if !w.multiLLM {
		step = "4단계"
	}
	printSectionHeader(step, "리랭커 설정 (선택 — 검색 결과 품질 향상용)")

	useRerank := w.cfg.Reranker.Endpoint != ""
	ok, back := promptYN("사용하시겠습니까?", useRerank)
	if back {
		return false, nil
	}
	if !ok {
		w.cfg.Reranker = config.RerankerConfig{}
		return true, nil
	}

	defEndpoint := "http://localhost:8787"
	defModel := "bge-reranker-v2-m3"
	defKey := ""
	if w.cfg.Reranker.Endpoint != "" {
		defEndpoint = w.cfg.Reranker.Endpoint
		defModel = w.cfg.Reranker.Model
		defKey = w.cfg.Reranker.APIKey
	}

	cfg, back := w.promptRerankerConfig(defEndpoint, defModel, defKey)
	if back {
		return false, nil
	}
	w.cfg.Reranker = cfg
	return true, nil
}

func (w *Wizard) promptLLMConfig(defaultEndpoint, defaultModel, defaultKey string) (*config.LLMConfig, bool) {
	rawURL, back := promptText("Endpoint (예: http://192.168.1.10:11434)", defaultEndpoint)
	if back {
		return nil, true
	}
	apiKey, back := promptSecret("API Key (없으면 엔터)")
	if back {
		return nil, true
	}

	printProgress("서버 탐색 중...")
	result, err := DiscoverServer(w.ctx, rawURL, apiKey)
	if err != nil {
		slog.Warn("서버 탐색 실패, 직접 입력으로 전환합니다", "url", rawURL, "err", err)
		printProgressResult("", false)
	} else {
		printProgressResult(string(result.Type), true)
	}

	modelIdx, back := promptSelect("모델을 선택하세요", result.Models)
	if back {
		return nil, true
	}
	model := result.Models[modelIdx]

	return &config.LLMConfig{
		Endpoint: result.Endpoint,
		Model:    model,
		APIKey:   apiKey,
		Mode:     "full",
		Timeout:  60,
	}, false
}

func (w *Wizard) promptEmbeddingConfig(defaultEndpoint, defaultModel, defaultKey string) (config.EmbeddingConfig, bool) {
	rawURL, back := promptText("Endpoint (예: http://192.168.1.10:8080)", defaultEndpoint)
	if back {
		return config.EmbeddingConfig{}, true
	}
	apiKey, back := promptSecret("API Key (없으면 엔터)")
	if back {
		return config.EmbeddingConfig{}, true
	}

	printProgress("임베딩 서버 탐색 중...")
	result, err := DiscoverServer(w.ctx, rawURL, apiKey)
	if err != nil {
		slog.Warn("임베딩 서버 탐색 실패, 직접 입력으로 전환합니다", "url", rawURL, "err", err)
		printProgressResult("", false)
	} else {
		printProgressResult(string(result.Type), true)
	}

	modelIdx, back := promptSelect("임베딩 모델을 선택하세요", result.Models)
	if back {
		return config.EmbeddingConfig{}, true
	}
	model := result.Models[modelIdx]

	provider := inferProvider(result.Type)
	fmt.Println(tui.StyleGeminiSubDesc.Render("  Provider: ") + tui.StyleGeminiOption.Render(provider) + tui.StyleGeminiSubDesc.Render(" (자동 감지)"))

	return config.EmbeddingConfig{
		Provider: provider,
		Endpoint: result.Endpoint,
		Model:    model,
		APIKey:   apiKey,
	}, false
}

func (w *Wizard) promptRerankerConfig(defaultEndpoint, defaultModel, defaultKey string) (config.RerankerConfig, bool) {
	rawURL, back := promptText("Endpoint (예: http://192.168.1.10:8787)", defaultEndpoint)
	if back {
		return config.RerankerConfig{}, true
	}
	apiKey, back := promptSecret("API Key (없으면 엔터)")
	if back {
		return config.RerankerConfig{}, true
	}

	printProgress("리랭커 서버 탐색 중...")
	result, err := DiscoverServer(w.ctx, rawURL, apiKey)
	if err != nil {
		slog.Warn("리랭커 서버 탐색 실패, 직접 입력으로 전환합니다", "url", rawURL, "err", err)
		printProgressResult("", false)
	} else {
		printProgressResult(string(result.Type), true)
	}

	modelIdx, back := promptSelect("리랭커 모델을 선택하세요", result.Models)
	if back {
		return config.RerankerConfig{}, true
	}
	model := result.Models[modelIdx]

	defTopK := "5"
	if w.cfg.Reranker.TopK > 0 {
		defTopK = strconv.Itoa(w.cfg.Reranker.TopK)
	}
	topKStr, back := promptText("상위 결과 수 (top_k)", defTopK)
	if back {
		return config.RerankerConfig{}, true
	}
	topK, err := strconv.Atoi(topKStr)
	if err != nil || topK < 1 {
		topK = 5
	}

	return config.RerankerConfig{
		Endpoint: result.Endpoint,
		Model:    model,
		APIKey:   apiKey,
		TopK:     topK,
	}, false
}

func inferProvider(t ServerType) string {
	switch t {
	case ServerOllama:
		return "ollama"
	case ServerOpenAI:
		return "openai"
	default:
		return "custom"
	}
}

func printProgress(msg string) {
	fmt.Print(tui.StyleGeminiSubDesc.Render("  ⟳ "+msg))
}

func printProgressResult(serverType string, ok bool) {
	if ok {
		fmt.Println(" " + tui.StyleSuccess.Render("✓") + " " + tui.StyleGeminiOption.Render(serverType))
	} else {
		fmt.Println(" " + tui.StyleError.Render("✗ 실패"))
	}
}

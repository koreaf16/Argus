// Package main — interactive init menu for LLM server/model registration.
//
// 파일 역할: `argus --init`에서 서버 등록, 모델 discovery/import,
//
//	모델 제거, 서버 제거를 수행하는 대화형 메뉴를 제공한다.
//
// 포함 모듈:
//   - initSession: init 메뉴 상태와 입출력을 관리하는 세션.
//   - run/addServerFlow/importModelsFromServerFlow: 주요 메뉴 동작.
//   - prompt/selection helpers: 번호 선택, 정수 입력, 확인 프롬프트 처리.
//
// 호출/사용 방식:
//   - cmd/argus/init.go 의 runInit()이 initSession.run()을 호출한다.
//   - 외부에 노출되는 진입점은 없다.
//
// 연결:
//   - import 하는 주요 패키지 (internal/...): services/llm.
//   - 이 파일을 import 하는 주요 패키지: 없음.
package main

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/koreaf16/argus/internal/components/logo"
	"github.com/koreaf16/argus/internal/services/llm"
)

var (
	titleStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#4492e6")).Bold(true).PaddingLeft(1)
	menuItemStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("252")).PaddingLeft(2)
	promptStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("70")).Bold(true)
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	boxStyle      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Padding(1, 2)
)

type initSession struct {
	reader    lineReader
	out       io.Writer
	registry  *llm.Registry
	modelPath string
}

type lineReader interface {
	ReadString(byte) (string, error)
}

func (s *initSession) run() error {
	for {
		s.printOverview()

		menu := []string{
			"1. 카탈로그 목록 표시",
			"2. 서버 추가",
			"3. 기존 서버에서 모델 추가",
			"4. 모델 제거",
			"5. 서버 제거",
			"6. 완료",
		}

		fmt.Fprintln(s.out, titleStyle.Render("메인 메뉴"))
		for _, item := range menu {
			fmt.Fprintln(s.out, menuItemStyle.Render(item))
		}
		fmt.Fprintln(s.out)

		choice, err := s.prompt("작업 선택", "6")
		if err != nil {
			return err
		}

		fmt.Fprintln(s.out)
		switch strings.TrimSpace(choice) {
		case "1":
			s.listCatalog()
		case "2":
			if err := s.addServerFlow(); err != nil {
				return err
			}
		case "3":
			if err := s.importModelsFromExistingServerFlow(); err != nil {
				return err
			}
		case "4":
			if err := s.removeModelFlow(); err != nil {
				return err
			}
		case "5":
			if err := s.removeServerFlow(); err != nil {
				return err
			}
		case "6", "q", "quit", "exit":
			fmt.Fprintln(s.out, promptStyle.Render("✔")+" 초기화가 완료되었습니다.")
			return nil
		default:
			fmt.Fprintf(s.out, "알 수 없는 선택: %s\n\n", choice)
		}
	}
}

func (s *initSession) printOverview() {
	active, _ := s.registry.ActiveEntry()

	logoView := logo.Render(logo.Data{
		Version:      "초기화 모드",
		ModelDisplay: active.Display,
		ProviderName: string(active.Provider),
		Columns:      80,
	}, false)

	fmt.Fprintln(s.out, logoView)

	overview := fmt.Sprintf("활성 모델: %s (%s)\n서버: %d, 사용자 모델: %d",
		titleStyle.Render(active.Alias),
		dimStyle.Render(active.Display),
		len(s.registry.ListServers()),
		len(s.registry.ListUserModels()))

	fmt.Fprintln(s.out, boxStyle.Render(overview))
	fmt.Fprintln(s.out)
}

func (s *initSession) listCatalog() {
	servers := s.registry.ListServers()
	if len(servers) == 0 {
		fmt.Fprintln(s.out, "서버: 없음")
	} else {
		fmt.Fprintln(s.out, "서버:")
		fmt.Fprintln(s.out, s.registry.FormatServerTable())
	}
	fmt.Fprintln(s.out, "모델:")
	fmt.Fprintln(s.out, s.registry.FormatTable())
}

func (s *initSession) addServerFlow() error {
	provider, err := s.promptProvider()
	if err != nil {
		return err
	}

	alias, err := s.promptRequired("서버 별칭(alias)", "")
	if err != nil {
		return err
	}

	server := llm.ServerEntry{
		Alias:    alias,
		Provider: provider,
		Display:  alias,
	}

	switch provider {
	case llm.ProviderOpenAICompat:
		serverURL, err := s.promptRequired("서버 URL", "http://127.0.0.1:11434")
		if err != nil {
			return err
		}
		apiKeyEnv, err := s.prompt("API 키 환경변수 (인증 없으면 비워둠)", "")
		if err != nil {
			return err
		}
		server.BaseURL = serverURL
		server.APIKeyEnv = apiKeyEnv

		probedServer, models, err := s.registry.ProbeOpenAICompatServer(context.Background(), server)
		if err != nil {
			return err
		}
		server = probedServer
		if err := s.registry.AddServer(server); err != nil {
			return err
		}
		if err := s.registry.Save(s.modelPath); err != nil {
			return err
		}
		fmt.Fprintf(s.out, "서버 추가됨: %s (%s)\n", server.Alias, server.EffectiveBaseURL())
		return s.selectAndImportModels(server, models)
	case llm.ProviderAnthropic:
		apiKeyEnv, err := s.promptRequired("API 키 환경변수", "ANTHROPIC_API_KEY")
		if err != nil {
			return err
		}
		baseURL, err := s.prompt("기본(base) URL", llm.DefaultServerBaseURL(provider))
		if err != nil {
			return err
		}
		server.APIKeyEnv = apiKeyEnv
		server.BaseURL = baseURL
	case llm.ProviderGemini:
		apiKeyEnv, err := s.promptRequired("API 키 환경변수", "GEMINI_API_KEY")
		if err != nil {
			return err
		}
		baseURL, err := s.prompt("기본(base) URL", llm.DefaultServerBaseURL(provider))
		if err != nil {
			return err
		}
		server.APIKeyEnv = apiKeyEnv
		server.BaseURL = baseURL
	}

	if err := s.registry.AddServer(server); err != nil {
		return err
	}
	if err := s.registry.Save(s.modelPath); err != nil {
		return err
	}
	fmt.Fprintf(s.out, "서버 추가됨: %s (%s)\n", server.Alias, server.EffectiveBaseURL())
	return s.importModelsFromServerFlow(server.Alias)
}

func (s *initSession) importModelsFromExistingServerFlow() error {
	servers := s.registry.ListServers()
	if len(servers) == 0 {
		fmt.Fprintln(s.out, "등록된 서버가 없습니다.")
		return nil
	}
	fmt.Fprintln(s.out, s.registry.FormatServerTable())
	token, err := s.promptRequired("서버 별칭 또는 인덱스", "")
	if err != nil {
		return err
	}
	return s.importModelsFromServerFlow(token)
}

func (s *initSession) importModelsFromServerFlow(token string) error {
	server, err := s.registry.ResolveServerToken(token)
	if err != nil {
		return err
	}

	for {
		models, err := s.registry.DiscoverModels(context.Background(), server)
		if err == nil && len(models) > 0 {
			return s.selectAndImportModels(server, models)
		}

		if err != nil {
			fmt.Fprintf(s.out, "검색 실패: %v\n", err)
		} else {
			fmt.Fprintln(s.out, "서버에서 반환된 모델이 없습니다.")
		}
		action, promptErr := s.prompt("작업 [재시도(retry)/수동(manual)/취소(cancel)]", "manual")
		if promptErr != nil {
			return promptErr
		}
		switch strings.ToLower(strings.TrimSpace(action)) {
		case "retry", "r":
			continue
		case "manual", "m":
			return s.manualImportFlow(server)
		default:
			return nil
		}
	}
}

func (s *initSession) selectAndImportModels(server llm.ServerEntry, models []llm.DiscoveredModel) error {
	fmt.Fprintln(s.out, "검색된 모델:")
	fmt.Fprintf(s.out, "%-4s %-34s %-42s %-10s\n", "#", "Display", "ModelID", "Context")
	for i, model := range models {
		ctxText := "-"
		if model.ContextWin > 0 {
			ctxText = strconv.Itoa(model.ContextWin)
		}
		fmt.Fprintf(s.out, "%-4d %-34s %-42s %-10s\n", i+1, model.Display, model.ModelID, ctxText)
	}

	selection, err := s.prompt("인덱스 선택 (쉼표 목록, all, 공백=취소)", "")
	if err != nil {
		return err
	}
	selection = strings.TrimSpace(selection)
	if selection == "" {
		return nil
	}

	indexes, err := parseIndexSelection(selection, len(models))
	if err != nil {
		return err
	}
	added := 0
	for _, idx := range indexes {
		if err := s.importSingleModel(server, models[idx]); err != nil {
			return err
		}
		added++
	}
	if added > 0 {
		if err := s.registry.Save(s.modelPath); err != nil {
			return err
		}
	}
	fmt.Fprintf(s.out, "%d개의 모델을 가져왔습니다.\n", added)
	return nil
}

func (s *initSession) manualImportFlow(server llm.ServerEntry) error {
	raw, err := s.prompt("모델 ID 목록 (쉼표 구분, 공백=취소)", "")
	if err != nil {
		return err
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	parts := strings.Split(raw, ",")
	added := 0
	for _, part := range parts {
		modelID := strings.TrimSpace(part)
		if modelID == "" {
			continue
		}
		discovered := llm.DiscoveredModel{
			ModelID: modelID,
			Display: modelID,
			Caps:    llm.DefaultCapsForProvider(server.Provider),
		}
		if err := s.importSingleModel(server, discovered); err != nil {
			return err
		}
		added++
	}
	if added > 0 {
		if err := s.registry.Save(s.modelPath); err != nil {
			return err
		}
	}
	fmt.Fprintf(s.out, "%d개의 모델을 가져왔습니다.\n", added)
	return nil
}

func (s *initSession) importSingleModel(server llm.ServerEntry, discovered llm.DiscoveredModel) error {
	alias := sanitizeAlias(discovered.ModelID)
	if alias == "" {
		alias = "model"
	}
	if _, exists := s.registry.Get(alias); exists {
		alias, err := s.promptRequired("별칭이 이미 존재합니다. 다른 별칭을 입력하세요", s.uniqueModelAlias(alias))
		if err != nil {
			return err
		}
		alias = sanitizeAlias(alias)
		if alias == "" {
			return fmt.Errorf("별칭이 필요합니다")
		}
		for {
			if _, exists := s.registry.Get(alias); !exists {
				break
			}
			alias, err = s.promptRequired("별칭이 이미 존재합니다. 다른 별칭을 입력하세요", s.uniqueModelAlias(alias))
			if err != nil {
				return err
			}
			alias = sanitizeAlias(alias)
			if alias == "" {
				return fmt.Errorf("별칭이 필요합니다")
			}
		}
	}

	display := discovered.Display
	if strings.TrimSpace(display) == "" {
		display = discovered.ModelID
	}

	contextDefault := discovered.ContextWin
	if contextDefault <= 0 {
		contextDefault = 0
	}
	contextWin, err := s.promptInt("최대 컨텍스트 토큰(max context tokens)", contextDefault, true)
	if err != nil {
		return err
	}

	entry := llm.EntryFromServer(server, discovered, alias, display, contextWin)
	if err := s.registry.Add(entry); err != nil {
		return err
	}
	fmt.Fprintf(s.out, "모델 추가됨: %s -> %s (ctx=%d)\n", entry.Alias, entry.ModelID, entry.ContextWin)
	return nil
}

func (s *initSession) removeModelFlow() error {
	models := s.registry.ListUserModels()
	if len(models) == 0 {
		fmt.Fprintln(s.out, "제거할 사용자 모델이 없습니다.")
		return nil
	}

	fmt.Fprintf(s.out, "%-4s %-22s %-18s %-20s %-34s\n", "#", "Alias", "Provider", "Server", "ModelID")
	for i, model := range models {
		serverAlias := model.ServerAlias
		if serverAlias == "" {
			serverAlias = "-"
		}
		fmt.Fprintf(s.out, "%-4d %-22s %-18s %-20s %-34s\n", i+1, model.Alias, model.Provider, serverAlias, model.ModelID)
	}

	token, err := s.promptRequired("모델 별칭 또는 인덱스", "")
	if err != nil {
		return err
	}
	alias := strings.TrimSpace(token)
	if idx, convErr := strconv.Atoi(alias); convErr == nil {
		if idx < 1 || idx > len(models) {
			return fmt.Errorf("인덱스 범위를 벗어남: %d", idx)
		}
		alias = models[idx-1].Alias
	}
	if err := s.registry.Remove(alias); err != nil {
		return err
	}
	if err := s.registry.Save(s.modelPath); err != nil {
		return err
	}
	fmt.Fprintf(s.out, "모델 제거됨: %s\n", alias)
	return nil
}

func (s *initSession) removeServerFlow() error {
	servers := s.registry.ListServers()
	if len(servers) == 0 {
		fmt.Fprintln(s.out, "제거할 서버가 없습니다.")
		return nil
	}

	fmt.Fprintln(s.out, s.registry.FormatServerTable())
	token, err := s.promptRequired("서버 별칭 또는 인덱스", "")
	if err != nil {
		return err
	}
	server, err := s.registry.ResolveServerToken(token)
	if err != nil {
		return err
	}

	linked := s.registry.ModelsForServer(server.Alias)
	fmt.Fprintf(s.out, "서버 %s를 제거하면 연결된 %d개의 모델도 제거됩니다.\n", server.Alias, len(linked))
	for _, model := range linked {
		fmt.Fprintf(s.out, "  - %s (%s)\n", model.Alias, model.ModelID)
	}
	ok, err := s.confirm("계속하시겠습니까?", false)
	if err != nil {
		return err
	}
	if !ok {
		fmt.Fprintln(s.out, "취소되었습니다.")
		return nil
	}

	if err := s.registry.RemoveServer(server.Alias); err != nil {
		return err
	}
	if err := s.registry.Save(s.modelPath); err != nil {
		return err
	}
	fmt.Fprintf(s.out, "서버 제거됨: %s\n", server.Alias)
	return nil
}

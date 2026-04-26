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
			"1. List catalog",
			"2. Add server",
			"3. Add models from existing server",
			"4. Remove model",
			"5. Remove server",
			"6. Done",
		}

		fmt.Fprintln(s.out, titleStyle.Render("Main Menu"))
		for _, item := range menu {
			fmt.Fprintln(s.out, menuItemStyle.Render(item))
		}
		fmt.Fprintln(s.out)

		choice, err := s.prompt("select action", "6")
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
			fmt.Fprintln(s.out, promptStyle.Render("✔")+" init complete.")
			return nil
		default:
			fmt.Fprintf(s.out, "unknown selection: %s\n\n", choice)
		}
	}
}

func (s *initSession) printOverview() {
	active, _ := s.registry.ActiveEntry()

	logoView := logo.Render(logo.Data{
		Version:      "init mode",
		ModelDisplay: active.Display,
		ProviderName: string(active.Provider),
		Columns:      80,
	}, false)

	fmt.Fprintln(s.out, logoView)

	overview := fmt.Sprintf("Active Model: %s (%s)\nServers: %d, User Models: %d",
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
		fmt.Fprintln(s.out, "Servers: none")
	} else {
		fmt.Fprintln(s.out, "Servers:")
		fmt.Fprintln(s.out, s.registry.FormatServerTable())
	}
	fmt.Fprintln(s.out, "Models:")
	fmt.Fprintln(s.out, s.registry.FormatTable())
}

func (s *initSession) addServerFlow() error {
	provider, err := s.promptProvider()
	if err != nil {
		return err
	}

	alias, err := s.promptRequired("server alias", "")
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
		serverURL, err := s.promptRequired("server url", "http://127.0.0.1:11434")
		if err != nil {
			return err
		}
		apiKeyEnv, err := s.prompt("api key env (blank for no auth)", "")
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
		fmt.Fprintf(s.out, "added server: %s (%s)\n", server.Alias, server.EffectiveBaseURL())
		return s.selectAndImportModels(server, models)
	case llm.ProviderAnthropic:
		apiKeyEnv, err := s.promptRequired("api key env", "ANTHROPIC_API_KEY")
		if err != nil {
			return err
		}
		baseURL, err := s.prompt("base url", llm.DefaultServerBaseURL(provider))
		if err != nil {
			return err
		}
		server.APIKeyEnv = apiKeyEnv
		server.BaseURL = baseURL
	case llm.ProviderGemini:
		apiKeyEnv, err := s.promptRequired("api key env", "GEMINI_API_KEY")
		if err != nil {
			return err
		}
		baseURL, err := s.prompt("base url", llm.DefaultServerBaseURL(provider))
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
	fmt.Fprintf(s.out, "added server: %s (%s)\n", server.Alias, server.EffectiveBaseURL())
	return s.importModelsFromServerFlow(server.Alias)
}

func (s *initSession) importModelsFromExistingServerFlow() error {
	servers := s.registry.ListServers()
	if len(servers) == 0 {
		fmt.Fprintln(s.out, "no servers registered.")
		return nil
	}
	fmt.Fprintln(s.out, s.registry.FormatServerTable())
	token, err := s.promptRequired("server alias or index", "")
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
			fmt.Fprintf(s.out, "discovery failed: %v\n", err)
		} else {
			fmt.Fprintln(s.out, "no models returned by server.")
		}
		action, promptErr := s.prompt("action [retry/manual/cancel]", "manual")
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
	fmt.Fprintln(s.out, "Discovered models:")
	fmt.Fprintf(s.out, "%-4s %-34s %-42s %-10s\n", "#", "Display", "ModelID", "Context")
	for i, model := range models {
		ctxText := "-"
		if model.ContextWin > 0 {
			ctxText = strconv.Itoa(model.ContextWin)
		}
		fmt.Fprintf(s.out, "%-4d %-34s %-42s %-10s\n", i+1, model.Display, model.ModelID, ctxText)
	}

	selection, err := s.prompt("select indexes (comma list, all, blank=cancel)", "")
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
	fmt.Fprintf(s.out, "imported %d model(s).\n", added)
	return nil
}

func (s *initSession) manualImportFlow(server llm.ServerEntry) error {
	raw, err := s.prompt("model ids (comma separated, blank=cancel)", "")
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
	fmt.Fprintf(s.out, "imported %d model(s).\n", added)
	return nil
}

func (s *initSession) importSingleModel(server llm.ServerEntry, discovered llm.DiscoveredModel) error {
	alias := sanitizeAlias(discovered.ModelID)
	if alias == "" {
		alias = "model"
	}
	if _, exists := s.registry.Get(alias); exists {
		alias, err := s.promptRequired("alias exists, enter another alias", s.uniqueModelAlias(alias))
		if err != nil {
			return err
		}
		alias = sanitizeAlias(alias)
		if alias == "" {
			return fmt.Errorf("alias is required")
		}
		for {
			if _, exists := s.registry.Get(alias); !exists {
				break
			}
			alias, err = s.promptRequired("alias exists, enter another alias", s.uniqueModelAlias(alias))
			if err != nil {
				return err
			}
			alias = sanitizeAlias(alias)
			if alias == "" {
				return fmt.Errorf("alias is required")
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
	contextWin, err := s.promptInt("max context tokens", contextDefault, true)
	if err != nil {
		return err
	}

	entry := llm.EntryFromServer(server, discovered, alias, display, contextWin)
	if err := s.registry.Add(entry); err != nil {
		return err
	}
	fmt.Fprintf(s.out, "added model: %s -> %s (ctx=%d)\n", entry.Alias, entry.ModelID, entry.ContextWin)
	return nil
}

func (s *initSession) removeModelFlow() error {
	models := s.registry.ListUserModels()
	if len(models) == 0 {
		fmt.Fprintln(s.out, "no user models to remove.")
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

	token, err := s.promptRequired("model alias or index", "")
	if err != nil {
		return err
	}
	alias := strings.TrimSpace(token)
	if idx, convErr := strconv.Atoi(alias); convErr == nil {
		if idx < 1 || idx > len(models) {
			return fmt.Errorf("index out of range: %d", idx)
		}
		alias = models[idx-1].Alias
	}
	if err := s.registry.Remove(alias); err != nil {
		return err
	}
	if err := s.registry.Save(s.modelPath); err != nil {
		return err
	}
	fmt.Fprintf(s.out, "removed model: %s\n", alias)
	return nil
}

func (s *initSession) removeServerFlow() error {
	servers := s.registry.ListServers()
	if len(servers) == 0 {
		fmt.Fprintln(s.out, "no servers to remove.")
		return nil
	}

	fmt.Fprintln(s.out, s.registry.FormatServerTable())
	token, err := s.promptRequired("server alias or index", "")
	if err != nil {
		return err
	}
	server, err := s.registry.ResolveServerToken(token)
	if err != nil {
		return err
	}

	linked := s.registry.ModelsForServer(server.Alias)
	fmt.Fprintf(s.out, "removing server %s will remove %d linked model(s).\n", server.Alias, len(linked))
	for _, model := range linked {
		fmt.Fprintf(s.out, "  - %s (%s)\n", model.Alias, model.ModelID)
	}
	ok, err := s.confirm("continue", false)
	if err != nil {
		return err
	}
	if !ok {
		fmt.Fprintln(s.out, "cancelled.")
		return nil
	}

	if err := s.registry.RemoveServer(server.Alias); err != nil {
		return err
	}
	if err := s.registry.Save(s.modelPath); err != nil {
		return err
	}
	fmt.Fprintf(s.out, "removed server: %s\n", server.Alias)
	return nil
}

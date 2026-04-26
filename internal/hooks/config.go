package hooks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/koreaf16/argus/internal/constants"
	"github.com/koreaf16/argus/internal/types"
)

// LoadConfig 는 홈 설정과 프로젝트 설정을 병합하여 반환한다.
// 홈: ~/.argus/settings.json, 프로젝트: {cwd}/.Argus/settings.json
// 어느 쪽이 없거나 hooks 필드가 없으면 빈 config (에러 없음).
func LoadConfig() (HooksConfig, error) {
	homeCfg, err := loadConfigFromFile(homeSettingsPath())
	if err != nil {
		return nil, err
	}
	projectCfg, err := loadConfigFromFile(constants.SettingsPath())
	if err != nil {
		return nil, err
	}
	return mergeHooksConfig(homeCfg, projectCfg), nil
}

// homeSettingsPath 는 ~/.argus/settings.json 경로를 반환한다.
func homeSettingsPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".argus", "settings.json")
}

// loadConfigFromFile 는 지정된 settings.json 파일에서 hooks 필드를 로드한다.
func loadConfigFromFile(path string) (HooksConfig, error) {
	if path == "" {
		return HooksConfig{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return HooksConfig{}, nil
		}
		return nil, fmt.Errorf("hooks: read %s: %w", path, err)
	}

	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("hooks: parse %s: %w", path, err)
	}

	raw, ok := root["hooks"]
	if !ok {
		return HooksConfig{}, nil
	}

	var rawMap map[string]json.RawMessage
	if err := json.Unmarshal(raw, &rawMap); err != nil {
		return nil, fmt.Errorf("hooks: parse hooks field in %s: %w", path, err)
	}

	cfg := make(HooksConfig, len(rawMap))
	for eventName, matchersRaw := range rawMap {
		event := types.HookEvent(eventName)
		if !types.IsHookEvent(string(event)) {
			continue
		}
		var matchers []HookMatcher
		if err := json.Unmarshal(matchersRaw, &matchers); err != nil {
			return nil, fmt.Errorf("hooks: parse matchers for %s in %s: %w", eventName, path, err)
		}
		cfg[event] = matchers
	}
	return cfg, nil
}

// mergeHooksConfig 는 두 HooksConfig 를 병합한다.
// base(홈) matchers 가 먼저, overlay(프로젝트) matchers 가 뒤에 배치된다.
func mergeHooksConfig(base, overlay HooksConfig) HooksConfig {
	if len(base) == 0 {
		return overlay
	}
	if len(overlay) == 0 {
		return base
	}
	result := make(HooksConfig, len(base))
	for event, matchers := range base {
		result[event] = append([]HookMatcher(nil), matchers...)
	}
	for event, matchers := range overlay {
		result[event] = append(result[event], matchers...)
	}
	return result
}

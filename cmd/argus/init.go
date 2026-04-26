// Package main — interactive Argus initialization entrypoint.
//
// 파일 역할: `argus --init` 실행 시 ~/.argus 기본 파일을 보장하고,
//
//	이후 대화형 LLM server/model 관리 메뉴를 시작한다.
//
// 포함 모듈:
//   - runInit: init 진입점. 파일 생성 후 registry 로드 및 메뉴 실행.
//   - ensureInitFiles: settings/models/history 기본 파일 보장.
//   - writeIfMissing: 파일이 없을 때만 생성.
//
// 호출/사용 방식:
//   - cmd/argus/main.go 의 main()이 `--init` 플래그에서 호출한다.
//   - 외부 진입점 함수는 runInit(in, out) 이다.
//
// 연결:
//   - import 하는 주요 패키지 (internal/...): constants, services/llm.
//   - 이 파일을 import 하는 주요 패키지: 없음.
package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/koreaf16/argus/internal/constants"
	"github.com/koreaf16/argus/internal/services/llm"
)

func runInit(in io.Reader, out io.Writer) error {
	if in == nil {
		in = os.Stdin
	}
	if out == nil {
		out = os.Stdout
	}
	if err := ensureInitFiles(out); err != nil {
		return err
	}

	reg := llm.NewRegistry()
	if err := reg.Load(constants.ModelsPath()); err != nil {
		return fmt.Errorf("load model catalog: %w", err)
	}

	session := &initSession{
		reader:    bufio.NewReader(in),
		out:       out,
		registry:  reg,
		modelPath: constants.ModelsPath(),
	}
	return session.run()
}

func ensureInitFiles(out io.Writer) error {
	dir := constants.ConfigDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	fmt.Fprintf(out, "config dir ready: %s\n", dir)

	for _, file := range []struct {
		path     string
		contents string
	}{
		{constants.SettingsPath(), defaultSettingsJSON},
		{constants.ModelsPath(), defaultModelsJSON},
	} {
		if err := writeIfMissing(file.path, file.contents, out); err != nil {
			return err
		}
	}

	historyPath := constants.HistoryPath()
	if _, err := os.Stat(historyPath); os.IsNotExist(err) {
		if err := os.WriteFile(historyPath, nil, 0o600); err != nil {
			return fmt.Errorf("create history file: %w", err)
		}
		fmt.Fprintf(out, "created: %s\n", historyPath)
	}
	fmt.Fprintln(out)
	return nil
}

func writeIfMissing(path, contents string, out io.Writer) error {
	if _, err := os.Stat(path); err == nil {
		fmt.Fprintf(out, "exists: %s\n", path)
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat %s: %w", path, err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create parent dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	fmt.Fprintf(out, "created: %s\n", path)
	return nil
}

const defaultSettingsJSON = `{
  "activeModel": "",
  "permissions": {
    "defaultMode": "ask",
    "allow": [],
    "deny": []
  },
  "web": {
    "searxng": {
      "base_url": "http://192.168.0.3:30080/search",
      "max_results": 10
    },
    "crawl4ai": {
      "base_url": "http://192.168.0.3:30090",
      "timeout_ms": 60000
    }
  },
  "ui": {
    "theme": "argus_ui_demo",
    "variant": "argus-signature",
    "motion": {
      "enabled": true,
      "level": "restrained",
      "tick_ms": 100,
      "reduced": false,
      "signature": true
    },
    "streaming": {
      "mode": "line-stable",
      "hide_unstable_markdown_tail": true,
      "flush_plain_text_partial": true,
      "render_code_blocks_stable": true
    }
  }
}
`

const defaultModelsJSON = `{
  "servers": [],
  "userModels": []
}
`

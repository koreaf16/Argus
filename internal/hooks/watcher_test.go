package hooks

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/koreaf16/argus/internal/types"
)

func TestNewFileWatcher_nilWhenNoFileChangedHooks(t *testing.T) {
	d := makeDispatcher(nil)
	fw, err := NewFileWatcher(context.Background(), d)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fw != nil {
		t.Error("expected nil watcher when no FileChanged hooks configured")
	}
}

func TestCollectWatchPaths_parsePipeSeparated(t *testing.T) {
	dir := t.TempDir()
	cfg := HooksConfig{
		types.HookEventFileChanged: {
			{Matcher: ".envrc|.env", Hooks: []HookCommand{
				{Type: HookCommandTypeCommand, Command: "echo changed"},
			}},
		},
	}
	d := NewDispatcher(cfg, "sess", dir)

	paths := collectWatchPaths(d)
	if len(paths) != 2 {
		t.Fatalf("expected 2 paths, got %d: %v", len(paths), paths)
	}

	want1 := filepath.Join(dir, ".envrc")
	want2 := filepath.Join(dir, ".env")
	found1, found2 := false, false
	for _, p := range paths {
		if p == want1 {
			found1 = true
		}
		if p == want2 {
			found2 = true
		}
	}
	if !found1 || !found2 {
		t.Errorf("paths %v missing expected entries %q, %q", paths, want1, want2)
	}
}

func TestFileWatcher_firesOnFileChange(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fsnotify event latency on Windows is unreliable in CI; integration covered by Linux")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "watch.txt")

	// 파일 미리 생성 (watcher Add가 성공하려면 파일이 존재해야 함)
	if err := os.WriteFile(target, []byte("init"), 0644); err != nil {
		t.Fatal(err)
	}

	// HTTP 서버로 Dispatch 호출 여부를 캡처한다.
	fired := make(chan struct{}, 1)
	srv := newHTTPSink(t, func() {
		select {
		case fired <- struct{}{}:
		default:
		}
	})
	defer srv.Close()

	cfg := HooksConfig{
		types.HookEventFileChanged: {
			{Matcher: "watch.txt", Hooks: []HookCommand{
				{Type: HookCommandTypeHTTP, URL: srv.URL, Timeout: 2},
			}},
		},
	}
	d := NewDispatcher(cfg, "sess", dir)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	fw, err := NewFileWatcher(ctx, d)
	if err != nil {
		t.Fatalf("NewFileWatcher failed: %v", err)
	}
	if fw == nil {
		t.Fatal("expected non-nil FileWatcher")
	}
	fw.Start()
	defer fw.Stop()

	// 짧은 대기 후 파일 수정
	time.Sleep(100 * time.Millisecond)
	if err := os.WriteFile(target, []byte("changed"), 0644); err != nil {
		t.Fatal(err)
	}

	// 최대 2초 대기 — 이벤트 도달 확인
	select {
	case <-fired:
		// 성공
	case <-time.After(2 * time.Second):
		t.Error("expected FileChanged hook to have fired within 2s")
	}
}

// newHTTPSink 는 요청을 받으면 onRequest 를 호출하는 테스트 HTTP 서버를 반환한다.
func newHTTPSink(t *testing.T, onRequest func()) *httpTestServer {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		onRequest()
		w.WriteHeader(http.StatusOK)
	})
	srv := &http.Server{Handler: mux}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go srv.Serve(ln)
	return &httpTestServer{Server: srv, URL: "http://" + ln.Addr().String()}
}

type httpTestServer struct {
	*http.Server
	URL string
}

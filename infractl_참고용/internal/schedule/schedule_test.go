// Package schedule
// File: schedule_test.go
// Description: schedule 패키지 단위 테스트
// Responsibility: masker / log / retention / oneshot 동작 검증

package schedule

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yourorg/infractl/internal/store"
)

// ─── Masker ──────────────────────────────────────────────────────────────────

func TestMaskCredentials_Password(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"password=secret123", "password=***"},
		{"PASSWORD: mysecret", "PASSWORD: ***"},
		{"token=abc.def.ghi", "token=***"},
		{"api_key=xyz", "api_key=***"},
		{"API-KEY: hello", "API-KEY: ***"},
		{"secret=abc", "secret=***"},
		{"auth=mytoken123", "auth=***"},
		{"no credentials here", "no credentials here"},
	}
	for _, c := range cases {
		got := MaskCredentials(c.input)
		if got != c.want {
			t.Errorf("MaskCredentials(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

func TestMaskCredentials_NoFalsePositive(t *testing.T) {
	input := "server: db01 port: 5432 user: admin"
	got := MaskCredentials(input)
	if got != input {
		t.Errorf("MaskCredentials falsely masked %q → %q", input, got)
	}
}

// ─── Logger ──────────────────────────────────────────────────────────────────

func TestLogger_WriteAndRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "schedule.log")

	l := NewLogger(path, DefaultLogMaxSize)
	l.Write("test-job", "check disk", "ok result", nil)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "schedule=test-job") {
		t.Error("log missing schedule name")
	}
	if !strings.Contains(content, "status=ok") {
		t.Error("log missing status=ok")
	}
	if !strings.Contains(content, "check disk") {
		t.Error("log missing prompt")
	}
	if !strings.Contains(content, "ok result") {
		t.Error("log missing result")
	}
}

func TestLogger_WriteError(t *testing.T) {
	dir := t.TempDir()
	l := NewLogger(filepath.Join(dir, "sched.log"), DefaultLogMaxSize)
	l.Write("err-job", "bad prompt", "", errors.New("connection refused"))

	data, err := os.ReadFile(filepath.Join(dir, "sched.log"))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "status=error") {
		t.Error("log missing status=error")
	}
	if !strings.Contains(content, "connection refused") {
		t.Error("log missing error message")
	}
}

func TestLogger_MaskingApplied(t *testing.T) {
	dir := t.TempDir()
	l := NewLogger(filepath.Join(dir, "sched.log"), DefaultLogMaxSize)
	l.Write("mask-job", "password=supersecret", "token=abc123", nil)

	data, err := os.ReadFile(filepath.Join(dir, "sched.log"))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	content := string(data)

	if strings.Contains(content, "supersecret") {
		t.Error("password not masked in prompt")
	}
	if strings.Contains(content, "abc123") {
		t.Error("token not masked in result")
	}
}

func TestLogger_Rotation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sched.log")

	// maxSize를 1바이트로 설정 — 첫 기록 직후 회전
	l := NewLogger(path, 1)
	l.Write("job1", "p", "r", nil)
	l.Write("job2", "p", "r", nil) // 회전 발생

	// .1 파일이 생성됐는지 확인
	if _, err := os.Stat(path + ".1"); os.IsNotExist(err) {
		t.Error("rotated log file .1 not found")
	}
}

// ─── rotateLogFile ────────────────────────────────────────────────────────────

func TestRotateLogFile_Sequential(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "app.log")

	// 기존 파일 생성
	if err := os.WriteFile(base, []byte("base"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(base+".1", []byte("rot1"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := rotateLogFile(base); err != nil {
		t.Fatalf("rotateLogFile: %v", err)
	}

	// base → .1, .1 → .2
	if _, err := os.Stat(base + ".2"); os.IsNotExist(err) {
		t.Error(".2 not created")
	}
	data1, _ := os.ReadFile(base + ".1")
	if string(data1) != "base" {
		t.Errorf(".1 should contain 'base', got %q", data1)
	}
	data2, _ := os.ReadFile(base + ".2")
	if string(data2) != "rot1" {
		t.Errorf(".2 should contain 'rot1', got %q", data2)
	}
}

// ─── Retention ───────────────────────────────────────────────────────────────

// mockScheduleStore는 테스트용 인메모리 ScheduleStore이다.
type mockScheduleStore struct {
	schedules map[int64]store.Schedule
	nextID    int64
}

func newMockStore() *mockScheduleStore {
	return &mockScheduleStore{schedules: make(map[int64]store.Schedule), nextID: 1}
}

func (m *mockScheduleStore) SaveSchedule(_ context.Context, s store.Schedule) (int64, error) {
	id := m.nextID
	m.nextID++
	s.ID = id
	m.schedules[id] = s
	return id, nil
}

func (m *mockScheduleStore) GetSchedule(_ context.Context, id int64) (store.Schedule, error) {
	if s, ok := m.schedules[id]; ok {
		return s, nil
	}
	return store.Schedule{}, errors.New("not found")
}

func (m *mockScheduleStore) GetScheduleByName(_ context.Context, name string) (store.Schedule, error) {
	for _, s := range m.schedules {
		if s.Name == name {
			return s, nil
		}
	}
	return store.Schedule{}, errors.New("not found")
}

func (m *mockScheduleStore) ListSchedules(_ context.Context) ([]store.Schedule, error) {
	var result []store.Schedule
	for _, s := range m.schedules {
		result = append(result, s)
	}
	return result, nil
}

func (m *mockScheduleStore) SetScheduleEnabled(_ context.Context, id int64, enabled bool) error {
	if s, ok := m.schedules[id]; ok {
		s.Enabled = enabled
		m.schedules[id] = s
	}
	return nil
}

func (m *mockScheduleStore) UpdateLastRun(_ context.Context, id int64, result string) error {
	if s, ok := m.schedules[id]; ok {
		now := time.Now()
		s.LastRun = &now
		s.LastResult = result
		m.schedules[id] = s
	}
	return nil
}

func (m *mockScheduleStore) DeleteSchedule(_ context.Context, id int64) error {
	delete(m.schedules, id)
	return nil
}

func TestPruneOneshotSchedules_ByKeepDays(t *testing.T) {
	ms := newMockStore()
	ctx := context.Background()

	old := time.Now().AddDate(0, 0, -10) // 10일 전 완료
	ms.schedules[1] = store.Schedule{ID: 1, Name: "old", Oneshot: true, Enabled: false, LastRun: &old}

	recent := time.Now().AddDate(0, 0, -1) // 1일 전 완료
	ms.schedules[2] = store.Schedule{ID: 2, Name: "recent", Oneshot: true, Enabled: false, LastRun: &recent}

	cfg := RetentionConfig{KeepDays: 7, MaxResults: 100}
	if err := PruneOneshotSchedules(ctx, ms, cfg); err != nil {
		t.Fatalf("prune: %v", err)
	}

	if _, ok := ms.schedules[1]; ok {
		t.Error("old oneshot should be deleted")
	}
	if _, ok := ms.schedules[2]; !ok {
		t.Error("recent oneshot should be kept")
	}
}

func TestPruneOneshotSchedules_ByMaxResults(t *testing.T) {
	ms := newMockStore()
	ctx := context.Background()

	// 5개 완료된 oneshot 생성
	for i := int64(1); i <= 5; i++ {
		t := time.Now().AddDate(0, 0, -int(i)) // i일 전
		ms.schedules[i] = store.Schedule{
			ID: i, Name: "job", Oneshot: true, Enabled: false, LastRun: &t,
		}
	}

	cfg := RetentionConfig{KeepDays: 0, MaxResults: 3}
	if err := PruneOneshotSchedules(ctx, ms, cfg); err != nil {
		t.Fatalf("prune: %v", err)
	}

	if len(ms.schedules) != 3 {
		t.Errorf("want 3 remaining, got %d", len(ms.schedules))
	}
}

func TestPruneOneshotSchedules_SkipActiveAndCron(t *testing.T) {
	ms := newMockStore()
	ctx := context.Background()

	old := time.Now().AddDate(0, 0, -10)
	// 활성 oneshot (아직 실행 안 됨) — 건드리면 안 됨
	ms.schedules[1] = store.Schedule{ID: 1, Oneshot: true, Enabled: true, LastRun: &old}
	// cron 스케줄 (oneshot=false) — 건드리면 안 됨
	ms.schedules[2] = store.Schedule{ID: 2, Oneshot: false, Enabled: false, LastRun: &old}

	cfg := RetentionConfig{KeepDays: 7, MaxResults: 100}
	if err := PruneOneshotSchedules(ctx, ms, cfg); err != nil {
		t.Fatalf("prune: %v", err)
	}

	if len(ms.schedules) != 2 {
		t.Errorf("active/cron schedules must not be pruned, got %d remaining", len(ms.schedules))
	}
}

// ─── OneshotManager ──────────────────────────────────────────────────────────

func TestOneshotManager_AddAndRun(t *testing.T) {
	ms := newMockStore()
	called := make(chan string, 1)
	runner := AgentRunner(func(_ context.Context, prompt string) (string, error) {
		called <- prompt
		return "done", nil
	})

	mgr := NewOneshotManager(ms, runner, nil)

	ctx := context.Background()
	at := time.Now().Add(50 * time.Millisecond)
	id, err := mgr.Add(ctx, at, "test-oneshot", "my prompt")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	select {
	case got := <-called:
		if got != "my prompt" {
			t.Errorf("runner got %q, want %q", got, "my prompt")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout: oneshot runner not called")
	}

	// 완료 후 Enabled=false 확인 (약간 지연 허용)
	time.Sleep(20 * time.Millisecond)
	sch, err := ms.GetSchedule(ctx, id)
	if err != nil {
		t.Fatalf("get schedule: %v", err)
	}
	if sch.Enabled {
		t.Error("oneshot should be disabled after run")
	}
}

func TestOneshotManager_Cancel(t *testing.T) {
	ms := newMockStore()
	called := make(chan struct{}, 1)
	runner := AgentRunner(func(_ context.Context, _ string) (string, error) {
		called <- struct{}{}
		return "", nil
	})

	mgr := NewOneshotManager(ms, runner, nil)
	ctx := context.Background()

	at := time.Now().Add(200 * time.Millisecond)
	id, err := mgr.Add(ctx, at, "cancel-test", "p")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	mgr.Cancel(id)

	select {
	case <-called:
		t.Error("runner should not be called after cancel")
	case <-time.After(350 * time.Millisecond):
		// 정상 — 취소 후 실행 안 됨
	}
}

func TestOneshotManager_PastTimeFiredImmediately(t *testing.T) {
	ms := newMockStore()
	called := make(chan struct{}, 1)
	runner := AgentRunner(func(_ context.Context, _ string) (string, error) {
		called <- struct{}{}
		return "", nil
	})

	mgr := NewOneshotManager(ms, runner, nil)
	ctx := context.Background()

	// 과거 시각 — 즉시 실행 기대
	at := time.Now().Add(-1 * time.Second)
	if _, err := mgr.Add(ctx, at, "past", "p"); err != nil {
		t.Fatalf("Add: %v", err)
	}

	select {
	case <-called:
		// 정상
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout: past oneshot should fire immediately")
	}
}

// Package background
// File: manager.go
// Description: 諛깃렇?쇱슫???묒뾽 ?앸챸二쇨린 愿由ъ옄
// Responsibility: ?묒뾽 ?깅줉, goroutine ?ㅽ뻾, ?꾨즺 ?뚮┝, 紐⑸줉/痍⑥냼/?ㅽ듃由щ컢 ?쒓났

package background

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"
)

// NotifyFunc???묒뾽 ?꾨즺 ???몄텧?섎뒗 肄쒕갚 ??낆씠??
type NotifyFunc func(jobID int, description string, success bool)

// Manager??諛깃렇?쇱슫???묒뾽???앸챸二쇨린瑜?愿由ы븳??
type Manager struct {
	mu                 sync.Mutex
	jobs               map[int]*jobEntry
	nextID             int
	notifyFunc         NotifyFunc
	watchdog           WatchdogNotifier
	watchdogStops      map[int]context.CancelFunc
	watchdogStallAfter time.Duration
	storageDir         string // Phase F: file streaming path
	maxFileSize        int64  // Phase F: max file size before rotation
}

type jobEntry struct {
	Job
	cancel context.CancelFunc
}

// NewManager????Manager瑜??앹꽦?쒕떎. storageDir媛 鍮꾩뼱 ?덉쑝硫??뚯씪 ?ㅽ듃由щ컢??鍮꾪솢?깊솕?쒕떎.
func NewManager(storageDir string, maxFileSize int64) *Manager {
	return &Manager{
		jobs:               make(map[int]*jobEntry),
		nextID:             1,
		watchdogStops:      make(map[int]context.CancelFunc),
		watchdogStallAfter: 45 * time.Second,
		storageDir:         storageDir,
		maxFileSize:        maxFileSize,
	}
}

// SetNotifyFunc???묒뾽 ?꾨즺 ???몄텧??肄쒕갚???ㅼ젙?쒕떎.
func (m *Manager) SetNotifyFunc(fn NotifyFunc) {
	m.mu.Lock()
	m.notifyFunc = fn
	m.mu.Unlock()
}

// SetWatchdog sets a soft-notice watchdog for streaming background jobs.
func (m *Manager) SetWatchdog(n WatchdogNotifier) {
	m.mu.Lock()
	m.watchdog = n
	m.mu.Unlock()
}

// SetWatchdogStallAfter overrides the default stall threshold.
func (m *Manager) SetWatchdogStallAfter(d time.Duration) {
	if d <= 0 {
		d = 45 * time.Second
	}
	m.mu.Lock()
	m.watchdogStallAfter = d
	m.mu.Unlock()
}

// Submit? fn??諛깃렇?쇱슫??goroutine?쇰줈 ?ㅽ뻾?섍퀬 ?묒뾽 ID瑜?諛섑솚?쒕떎.
// 湲곗〈 memory-only ?몄텧??RAG ?? 蹂댄샇瑜??꾪빐 ?좎??쒕떎.
func (m *Manager) Submit(ctx context.Context, description string, fn func(ctx context.Context) (string, error)) int {
	m.mu.Lock()
	id := m.nextID
	m.nextID++
	jobCtx, cancel := context.WithCancel(ctx)
	e := &jobEntry{
		Job: Job{
			ID:          id,
			Description: description,
			Status:      StatusRunning,
			StartedAt:   time.Now(),
		},
		cancel: cancel,
	}
	m.jobs[id] = e
	m.mu.Unlock()

	go func() {
		result, err := fn(jobCtx)
		now := time.Now()

		m.mu.Lock()
		e.CompletedAt = &now
		if err != nil {
			e.Status = StatusFailed
			e.Error = err.Error()
		} else {
			e.Status = StatusCompleted
			e.Result = result
		}
		notify := m.notifyFunc
		m.mu.Unlock()

		success := err == nil
		slog.Info("background job completed", "id", id, "success", success)
		if notify != nil {
			notify(id, description, success)
		}
	}()

	return id
}

// SubmitStreaming? fn??諛깃렇?쇱슫?쒕줈 ?ㅽ뻾?섍퀬 stdout/stderr瑜??뚯씪??湲곕줉?쒕떎.
// storageDir媛 鍮꾩뼱 ?덉쑝硫??뚯씪 ?놁씠 ?ㅽ뻾?섍퀬 streams??writer??io.Discard濡??泥댄븳??
// fn? ?꾨즺 ??streams???곌린瑜?以묐떒?댁빞 ?쒕떎.
func (m *Manager) SubmitStreaming(ctx context.Context, description string, fn func(ctx context.Context, streams *JobStreams) error) int {
	m.mu.Lock()
	id := m.nextID
	m.nextID++
	jobCtx, cancel := context.WithCancel(ctx)
	e := &jobEntry{
		Job: Job{
			ID:          id,
			Description: description,
			Status:      StatusRunning,
			StartedAt:   time.Now(),
			Streamed:    true,
		},
		cancel: cancel,
	}
	if m.storageDir != "" {
		e.StoragePath = jobStdoutPath(m.storageDir, id)
	}
	m.jobs[id] = e
	watchdog := m.watchdog
	watchdogStallAfter := m.watchdogStallAfter
	storagePath := e.StoragePath
	m.mu.Unlock()

	if watchdog != nil && storagePath != "" {
		watchdogCtx, watchdogCancel := context.WithCancel(context.Background())
		m.mu.Lock()
		m.watchdogStops[id] = watchdogCancel
		m.mu.Unlock()
		go RunStallWatchdog(watchdogCtx, StallWatchdogConfig{
			JobID:       id,
			StoragePath: storagePath,
			Notifier:    watchdog,
			PollEvery:   5 * time.Second,
			StallAfter:  watchdogStallAfter,
			Cooldown:    40 * time.Second,
			TailBytes:   1024,
		})
	}

	go func() {
		streams, stdoutCloser, stderrCloser, openErr := m.openStreams(id)
		if openErr != nil {
			slog.Error("failed to open job stream files", "id", id, "err", openErr)
			// ?뚯씪???댁? 紐삵빐??fn? ?ㅽ뻾 ??streams??io.Discard ?泥?
		}

		err := fn(jobCtx, streams)
		now := time.Now()

		// ?뚯씪 ?リ린 諛??곹깭 湲곕줉
		if stdoutCloser != nil {
			stdoutCloser.Close()
		}
		if stderrCloser != nil {
			stderrCloser.Close()
		}
		if m.storageDir != "" {
			errMsg := ""
			status := StatusCompleted
			if err != nil {
				status = StatusFailed
				errMsg = err.Error()
			}
			if writeErr := writeStatusFile(m.storageDir, id, status, errMsg); writeErr != nil {
				slog.Warn("failed to write status file", "id", id, "err", writeErr)
			}
		}

		if stop := m.takeWatchdogStop(id); stop != nil {
			stop()
		}

		m.mu.Lock()
		e.CompletedAt = &now
		if err != nil {
			e.Status = StatusFailed
			e.Error = err.Error()
		} else {
			e.Status = StatusCompleted
		}
		notify := m.notifyFunc
		m.mu.Unlock()

		success := err == nil
		slog.Info("background streaming job completed", "id", id, "success", success)
		if notify != nil {
			notify(id, description, success)
		}
	}()

	return id
}

// RegisterPending? ?대? ?몃? goroutine?먯꽌 ?ㅽ뻾 以묒씤 ?묒뾽??Manager???깅줉?쒕떎.
// 諛섑솚??complete ?⑥닔瑜?goroutine 醫낅즺 ??諛섎뱶???몄텧?댁빞 ?쒕떎.
// Auto-promotion?먯꽌 30s 珥덇낵 ??湲곗〈 goroutine 寃곌낵瑜?異붿쟻?섎뒗 ???ъ슜?쒕떎.
func (m *Manager) RegisterPending(description string) (id int, complete func(result string, err error)) {
	m.mu.Lock()
	id = m.nextID
	m.nextID++
	e := &jobEntry{
		Job: Job{
			ID:          id,
			Description: description,
			Status:      StatusRunning,
			StartedAt:   time.Now(),
		},
		cancel: func() {}, // ?몃? goroutine? Manager媛 痍⑥냼?????놁쓬
	}
	m.jobs[id] = e
	m.mu.Unlock()

	complete = func(result string, err error) {
		now := time.Now()
		m.mu.Lock()
		e.CompletedAt = &now
		if err != nil {
			e.Status = StatusFailed
			e.Error = err.Error()
		} else {
			e.Status = StatusCompleted
			e.Result = result
		}
		notify := m.notifyFunc
		m.mu.Unlock()

		success := err == nil
		slog.Info("promoted background job completed", "id", id, "success", success)
		if notify != nil {
			notify(id, description, success)
		}
	}
	return id, complete
}

// openStreams??storageDir???뚯씪???닿퀬 JobStreams瑜??앹꽦?쒕떎.
// storageDir媛 鍮꾩뼱 ?덉쑝硫?io.Discard 湲곕컲 streams瑜?諛섑솚?쒕떎.
func (m *Manager) openStreams(id int) (streams *JobStreams, stdoutCloser, stderrCloser io.Closer, err error) {
	if m.storageDir == "" {
		return &JobStreams{Stdout: io.Discard, Stderr: io.Discard}, nil, nil, nil
	}
	so, se, openErr := openJobFiles(m.storageDir, id, m.maxFileSize)
	if openErr != nil {
		return &JobStreams{Stdout: io.Discard, Stderr: io.Discard}, nil, nil, openErr
	}
	return &JobStreams{Stdout: so, Stderr: se}, so, se, nil
}

// AddMonitorBytes??Monitor ?꾧뎄媛 jobID?????諛⑹텧??諛붿씠???섎? ?꾩쟻?쒕떎.
// 諛섑솚媛믪? 媛깆떊???꾩쟻?됱씠??
func (m *Manager) AddMonitorBytes(id, n int) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.jobs[id]
	if !ok {
		return 0
	}
	e.MonitorBytesEmitted += n
	return e.MonitorBytesEmitted
}

// StorageDir???꾩옱 ?ㅼ젙???ㅽ듃由щ컢 ???寃쎈줈瑜?諛섑솚?쒕떎.
func (m *Manager) StorageDir() string {
	return m.storageDir
}

// List??紐⑤뱺 ?묒뾽???ㅻ깄?룹쓣 諛섑솚?쒕떎.
func (m *Manager) List() []Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Job, 0, len(m.jobs))
	for _, e := range m.jobs {
		out = append(out, e.Job)
	}
	return out
}

// Get? ID濡??묒뾽??議고쉶?쒕떎.
func (m *Manager) Get(id int) (Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.jobs[id]
	if !ok {
		return Job{}, fmt.Errorf("?묒뾽 #%d 瑜?李얠쓣 ???놁뒿?덈떎", id)
	}
	return e.Job, nil
}

// Cancel? ?ㅽ뻾 以묒씤 ?묒뾽??痍⑥냼?쒕떎.
func (m *Manager) Cancel(id int) error {
	m.mu.Lock()
	e, ok := m.jobs[id]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("?묒뾽 #%d 瑜?李얠쓣 ???놁뒿?덈떎", id)
	}
	if e.Status != StatusRunning {
		m.mu.Unlock()
		return fmt.Errorf("?묒뾽 #%d ???대? 醫낅즺?섏뿀?듬땲??(%s)", id, e.Status)
	}
	e.cancel()
	e.Status = StatusCancelled
	now := time.Now()
	e.CompletedAt = &now

	if m.storageDir != "" {
		if writeErr := writeStatusFile(m.storageDir, id, StatusCancelled, "cancelled by user"); writeErr != nil {
			slog.Warn("failed to write cancel status file", "id", id, "err", writeErr)
		}
	}
	m.mu.Unlock()
	if stop := m.takeWatchdogStop(id); stop != nil {
		stop()
	}
	return nil
}

func (m *Manager) takeWatchdogStop(id int) context.CancelFunc {
	m.mu.Lock()
	defer m.mu.Unlock()
	stop, ok := m.watchdogStops[id]
	if ok {
		delete(m.watchdogStops, id)
	}
	return stop
}

// CleanStorage??蹂닿? ?뺤콉???곕씪 ?ㅻ옒???뚯씪????젣?쒕떎.
func (m *Manager) CleanStorage(keepDays, maxResults int) error {
	if m.storageDir == "" {
		return nil
	}
	return cleanOldFiles(m.storageDir, keepDays, maxResults)
}

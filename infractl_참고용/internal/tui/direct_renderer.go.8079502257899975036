package tui

import (
	"sync"
	"time"
)

type DirectRenderer struct {
	state    *SessionState
	box      *liveArea
	md       *mdRenderer
	progress *progressTree
	mu       sync.Mutex

	tokens      string
	lastRender  time.Time
	streamCache stableCache
	isStreaming bool

	shellArgs     map[string]any
	shellLines    []string
	shellTotal    int
	shellToolName string
	startedAt     time.Time

	shimmer       shimmerState
	shimmerMu     sync.Mutex
	shimmerOn     bool
	shimmerGen    int
	shimmerDone   chan struct{}
	thinkingLabel string
	thinkBuf      string
	width         int
}

func NewDirectRenderer(state *SessionState) *DirectRenderer {
	w := state.Width
	if w < 40 {
		w = 80
	}
	return &DirectRenderer{
		state:    state,
		box:      newLiveArea(w),
		md:       newMdRenderer(w - 6),
		progress: newProgressTree(),
		width:    w,
	}
}

func (r *DirectRenderer) StartShimmer(text string) {
	r.shimmerMu.Lock()
	if r.shimmerOn {
		r.shimmerMu.Unlock()
		return
	}
	r.shimmer = newShimmer()
	r.shimmer.width = r.width
	r.shimmer.Start(text)
	r.shimmerOn = true
	r.shimmerGen++
	gen := r.shimmerGen
	done := make(chan struct{})
	r.shimmerDone = done
	r.shimmerMu.Unlock()
	go r.runShimmerWithGen(gen, done)
}

func (r *DirectRenderer) StopShimmer() {
	r.shimmerMu.Lock()
	wasOn := r.shimmerOn
	r.shimmerOn = false
	r.shimmerMu.Unlock()
	if wasOn && r.shimmerDone != nil {
		<-r.shimmerDone
	}
}

func (r *DirectRenderer) runShimmerWithGen(gen int, done chan struct{}) {
	defer close(done)
	for {
		r.shimmerMu.Lock()
		if !r.shimmerOn || r.shimmerGen != gen {
			r.shimmerMu.Unlock()
			return
		}
		r.shimmer.pos++
		view := r.shimmer.View()
		r.shimmerMu.Unlock()

		r.mu.Lock()
		r.shimmerMu.Lock()
		shouldDraw := r.shimmerOn && r.shimmerGen == gen
		r.shimmerMu.Unlock()
		if shouldDraw {
			r.box.SetHeader(view)
			r.box.Redraw()
		}
		r.mu.Unlock()

		time.Sleep(100 * time.Millisecond)
	}
}

func (r *DirectRenderer) PauseForPrompt() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stopShimmerInternal()
	r.box.Clear()
	r.box.Reset()
}

func (r *DirectRenderer) ResumeAfterPrompt(label string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.startShimmerInternal(label)
}

func (r *DirectRenderer) RecordActivity() {
	r.shimmerMu.Lock()
	r.shimmer.RecordActivity()
	r.shimmerMu.Unlock()
}

func (r *DirectRenderer) SetShimmerText(text string) {
	r.shimmerMu.Lock()
	r.shimmer.SetText(text)
	r.shimmerMu.Unlock()
}

func (r *DirectRenderer) SetShimmerHint(hint string) {
	r.shimmerMu.Lock()
	r.shimmer.hint = hint
	r.shimmerMu.Unlock()
}

func (r *DirectRenderer) stopShimmerInternal() {
	r.shimmerMu.Lock()
	r.shimmerOn = false
	r.shimmerMu.Unlock()
}

func (r *DirectRenderer) startShimmerInternal(text string) {
	r.shimmerMu.Lock()
	if r.shimmerOn {
		r.shimmerMu.Unlock()
		return
	}
	r.shimmer = newShimmer()
	r.shimmer.width = r.width
	r.shimmer.Start(text)
	r.shimmerOn = true
	r.shimmerGen++
	gen := r.shimmerGen
	done := make(chan struct{})
	r.shimmerDone = done
	r.shimmerMu.Unlock()
	go r.runShimmerWithGen(gen, done)
}

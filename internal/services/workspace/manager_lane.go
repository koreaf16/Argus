package workspace

import (
	"context"
	"strings"
	"time"

	"github.com/koreaf16/argus/internal/services/workspace/channel"
)

// LaneInfo is a workspace-level view of a privilege lane snapshot, derived
// from the channel manager. The name is preserved for backward compatibility
// with active_target_sync.go and the TUI status bar; future surfaces should
// prefer ChannelManager.Snapshots() directly.
type LaneInfo struct {
	Alias        string
	AccountStack []string
	CWD          string
	PrivState    string
	LastUsed     time.Time
}

// LaneInfo returns the most-recent exec channel snapshot for alias as a
// LaneInfo, or (zero, false) if no exec channel has been opened. The newest
// channel by LastUsed wins so callers always see the privilege lane the next
// command will land on.
func (m *Manager) LaneInfo(alias string) (LaneInfo, bool) {
	alias = m.ResolveAlias(alias)
	hostAlias := alias
	if target, err := m.ResolveExecutionTarget(alias); err == nil {
		hostAlias = target.HostAlias
	}
	if !m.IsRemoteAlias(alias) || m.chanMgr == nil {
		return LaneInfo{}, false
	}
	snaps := m.chanMgr.Snapshots()
	var newest *channel.ChannelSnapshot
	for i := range snaps {
		s := snaps[i]
		if s.Key.Alias != hostAlias || s.Key.Purpose != channel.PurposeExec {
			continue
		}
		if newest == nil || s.LastUsed.After(newest.LastUsed) {
			newest = &s
		}
	}
	if newest == nil {
		return LaneInfo{}, false
	}
	info := laneInfoFromSnapshot(*newest)
	info.Alias = alias
	return info, true
}

// LaneInfos returns one LaneInfo per (alias, exec-purpose) channel snapshot.
// When an alias has both a default and one or more privilege channels, the
// caller sees one entry per channel so the TUI can render the full chain.
func (m *Manager) LaneInfos() []LaneInfo {
	if m.chanMgr == nil {
		return nil
	}
	snaps := m.chanMgr.Snapshots()
	out := make([]LaneInfo, 0, len(snaps))
	for _, s := range snaps {
		if s.Key.Purpose != channel.PurposeExec {
			continue
		}
		out = append(out, laneInfoFromSnapshot(s))
	}
	return out
}

func laneInfoFromSnapshot(s channel.ChannelSnapshot) LaneInfo {
	stack := make([]string, 0, len(s.AccountStack))
	for _, u := range s.AccountStack {
		if strings.TrimSpace(u) != "" {
			stack = append(stack, u)
		}
	}
	return LaneInfo{
		Alias:        s.Key.Alias,
		AccountStack: stack,
		CWD:          s.CWD,
		PrivState:    string(s.State),
		LastUsed:     s.LastUsed,
	}
}

// WarmUpLane은 서버 연결 직후 비동기로 exec channel(lane)을 미리 열어둔다.
// 첫 bash 명령 실행 시 SSH 세션 + PTY 초기화가 이미 완료되어 있어
// priming 지연(~15–21초)이 사라진다.
// 실패 시 무시한다 — 다음 Exec 호출에서 정상적으로 생성된다.
func (m *Manager) WarmUpLane(alias string) {
	alias = m.ResolveAlias(alias)
	if !m.IsRemoteAlias(alias) {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		// channel backbone이 활성이면 channel 경로로 warm-up
		if m.shouldRouteViaChannel(alias, ExecOptions{}) {
			_, _ = m.execViaChannel(ctx, alias, ":", ExecOptions{
				Timeout: 10 * time.Second,
			})
			return
		}

		// legacy account shell 경로
		_, _ = m.ExecWithOptions(ctx, alias, ":", ExecOptions{
			Timeout: 10 * time.Second,
		}, nil)
	}()
}


package query

import (
	"github.com/koreaf16/argus/internal/services/inventory"
	"github.com/koreaf16/argus/internal/services/llm"
	"github.com/koreaf16/argus/internal/services/workspace"
)

// inventorySystemBlocks injects the server inventory into the LLM system prompt.
// Returns nil when no inventory exists or collection is disabled.
func inventorySystemBlocks(manager *workspace.Manager) []llm.SystemBlock {
	if manager == nil {
		return nil
	}
	active := manager.ActiveAlias()

	snap, ok := manager.GetInventorySnapshot(active)
	if !ok {
		if active == workspace.LocalAlias {
			return nil
		}
		return []llm.SystemBlock{{
			Type: "text",
			Text: "서버 인벤토리: 아직 수집되지 않았습니다. server_inventory 도구로 수집하거나 잠시 후 재시도하세요.",
		}}
	}
	switch snap.Status {
	case inventory.StatusDisabled:
		return nil
	case inventory.StatusPending:
		return []llm.SystemBlock{{
			Type: "text",
			Text: "서버 인벤토리: 백그라운드 수집 진행 중. 다음 턴에서 자동 갱신됩니다.",
		}}
	}

	return []llm.SystemBlock{{
		Type:         "text",
		Text:         inventory.FormatSummaryForPrompt(snap),
		CacheControl: map[string]any{"type": "ephemeral"},
	}}
}

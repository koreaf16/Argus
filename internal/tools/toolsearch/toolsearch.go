package toolsearch

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	tool "github.com/koreaf16/argus/internal/tools"
)

type Tool struct{}

type request struct {
	Query              string   `json:"query"`
	EvidenceCategories []string `json:"evidence_categories"`
	Limit              int      `json:"limit"`
}

type candidate struct {
	Name               string   `json:"name"`
	Description        string   `json:"description,omitempty"`
	EvidenceCategories []string `json:"evidence_categories,omitempty"`
}

type response struct {
	Query      string      `json:"query"`
	Candidates []candidate `json:"candidates"`
}

func New() *Tool { return &Tool{} }

func (t *Tool) Name() string { return "tool_search" }

func (t *Tool) Description(ctx tool.Context) string {
	return "사용자 의도나 증거 카테고리에 따라 사용 가능한 도구 목록을 검색합니다. 도구를 찾기만 하며 실행하지는 않습니다."
}

func (t *Tool) InputSchema() tool.ToolInputJSONSchema {
	return tool.ToolInputJSONSchema{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "찾고자 하는 작업, 필요한 증거 또는 도구 기능에 대한 짧은 설명입니다.",
			},
			"evidence_categories": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "string",
					"enum": []string{
						tool.EvidenceLocalCode,
						tool.EvidenceExternalFresh,
						tool.EvidenceServerState,
						tool.EvidenceUserDecision,
						tool.EvidenceMutation,
						tool.EvidencePlanning,
					},
				},
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "반환할 최대 도구 수입니다. 기본값은 8입니다.",
			},
		},
	}
}

func (t *Tool) IsReadOnly() bool { return true }

func (t *Tool) MaxResultSizeChars() int { return 20000 }

func (t *Tool) CheckPermission(ctx tool.Context, input json.RawMessage) (tool.PermissionResult, error) {
	return tool.DefaultAllowPermission(), nil
}

func (t *Tool) Call(ctx tool.Context, input json.RawMessage) (<-chan tool.ToolEvent, error) {
	var req request
	if len(input) > 0 {
		if err := json.Unmarshal(input, &req); err != nil {
			return nil, err
		}
	}
	ch := make(chan tool.ToolEvent, 2)
	go func() {
		defer close(ch)
		if ctx.Registry == nil {
			ch <- tool.NewErrorEvent(fmt.Errorf("tool_search를 위해 도구 레지스트리가 필요합니다"))
			return
		}
		limit := req.Limit
		if limit <= 0 {
			limit = 8
		}
		if limit > 20 {
			limit = 20
		}
		candidates := search(ctx, req, limit)
		payload, err := json.Marshal(response{
			Query:      strings.TrimSpace(req.Query),
			Candidates: candidates,
		})
		if err != nil {
			ch <- tool.NewErrorEvent(err)
			return
		}
		ch <- tool.NewOutputEvent(string(payload))
		ch <- tool.NewDoneEvent()
	}()
	return ch, nil
}

type scoredCandidate struct {
	item  candidate
	score int
}

func search(ctx tool.Context, req request, limit int) []candidate {
	queryTerms := splitTerms(req.Query)
	categoryFilter := make(map[string]bool)
	for _, cat := range req.EvidenceCategories {
		cat = strings.TrimSpace(cat)
		if cat != "" {
			categoryFilter[cat] = true
		}
	}

	scored := make([]scoredCandidate, 0)
	for _, tl := range ctx.Registry.List() {
		if tl == nil {
			continue
		}
		if v, ok := tl.(tool.VisibleFilter); ok && !v.IsVisible(ctx) {
			continue
		}
		name := strings.TrimSpace(tl.Name())
		if name == "" || tool.CanonicalName(name) == "tool_search" {
			continue
		}
		desc := strings.TrimSpace(tl.Description(ctx))
		cats := tool.ToolEvidenceCategories(name, tl.IsReadOnly())
		score := scoreCandidate(name, desc, cats, queryTerms, categoryFilter)
		if score <= 0 && len(queryTerms) > 0 {
			continue
		}
		if len(categoryFilter) > 0 && !matchesAnyCategory(cats, categoryFilter) {
			continue
		}
		scored = append(scored, scoredCandidate{
			item: candidate{
				Name:               name,
				Description:        desc,
				EvidenceCategories: cats,
			},
			score: score,
		})
	}

	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		return scored[i].item.Name < scored[j].item.Name
	})
	if len(scored) > limit {
		scored = scored[:limit]
	}
	out := make([]candidate, 0, len(scored))
	for _, item := range scored {
		out = append(out, item.item)
	}
	return out
}

func splitTerms(query string) []string {
	fields := strings.Fields(strings.ToLower(query))
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.Trim(field, ".,;:!?()[]{}\"'")
		if len(field) < 2 {
			continue
		}
		out = append(out, field)
	}
	return out
}

func scoreCandidate(name string, desc string, cats []string, terms []string, categoryFilter map[string]bool) int {
	haystack := strings.ToLower(name + " " + desc + " " + strings.Join(cats, " "))
	score := 0
	for _, term := range terms {
		if strings.Contains(strings.ToLower(name), term) {
			score += 5
			continue
		}
		if strings.Contains(haystack, term) {
			score += 2
		}
	}
	for _, cat := range cats {
		if categoryFilter[cat] {
			score += 4
		}
	}
	if len(terms) == 0 && len(categoryFilter) == 0 {
		score = 1
	}
	return score
}

func matchesAnyCategory(cats []string, filter map[string]bool) bool {
	if len(filter) == 0 {
		return true
	}
	for _, cat := range cats {
		if filter[cat] {
			return true
		}
	}
	return false
}

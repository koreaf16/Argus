package context_test

import (
	"testing"

	ctxpkg "github.com/koreaf16/argus/internal/context"
)

// --- Graph 테스트 ---

func TestGraph_AppendAndLen(t *testing.T) {
	g := ctxpkg.NewGraph()
	if g.Len() != 0 {
		t.Fatalf("expected 0, got %d", g.Len())
	}
	g.AppendUser("hello")
	g.AppendAssistant("hi")
	if g.Len() != 2 {
		t.Fatalf("expected 2, got %d", g.Len())
	}
}

func TestGraph_ToolPair(t *testing.T) {
	g := ctxpkg.NewGraph()
	g.AppendUser("run something")
	g.AppendAssistant("ok")
	g.AppendToolUse("call-1", "bash", []byte(`{"command":"ls"}`))
	g.AppendToolResult("call-1", "bash", "file1.go\nfile2.go", ctxpkg.ProjectionFull, false, "", "")

	nodes := g.Nodes()
	if len(nodes) != 4 {
		t.Fatalf("expected 4 nodes, got %d", len(nodes))
	}
	// tool_result의 ToolUseSeq이 tool_use의 Seq과 같아야 함
	tu := nodes[2]
	tr := nodes[3]
	if tu.Kind != ctxpkg.NodeKindToolUse {
		t.Errorf("node[2] should be tool_use")
	}
	if tr.Kind != ctxpkg.NodeKindToolResult {
		t.Errorf("node[3] should be tool_result")
	}
	if tr.ToolUseSeq != tu.Seq {
		t.Errorf("tool_result.ToolUseSeq=%d should match tool_use.Seq=%d", tr.ToolUseSeq, tu.Seq)
	}
}

func TestGraph_MarkProtected_LastTwoTurns(t *testing.T) {
	g := ctxpkg.NewGraph()
	// turn 1
	g.AppendUser("turn1")
	g.AppendAssistant("reply1")
	// turn 2
	g.AppendUser("turn2")
	g.AppendAssistant("reply2")
	// turn 3 (current)
	g.AppendUser("current turn")
	g.AppendAssistant("reply3")

	g.MarkProtected("current turn")

	nodes := g.Nodes()
	// 최신 2 user 이후 노드들은 Protected되어야 함
	protected := 0
	for _, n := range nodes {
		if n.Protected {
			protected++
		}
	}
	// 최소 최신 2개 user + 해당 assistant는 보호되어야 함
	if protected < 4 {
		t.Errorf("expected at least 4 protected nodes, got %d", protected)
	}
}

func TestGraph_MarkProtected_RecalculatesFromScratch(t *testing.T) {
	g := ctxpkg.NewGraph()
	// turn 1
	g.AppendUser("turn1")
	g.AppendAssistant("reply1")
	// turn 2
	g.AppendUser("turn2")
	g.AppendAssistant("reply2")
	// turn 3
	g.AppendUser("turn3")
	g.AppendAssistant("reply3")

	g.MarkProtected("turn3")
	nodes := g.Nodes()
	if !nodes[2].Protected {
		t.Fatalf("expected turn2 user node to be protected after first pass")
	}

	// Add one more turn and recalculate. turn2 should no longer be protected.
	g.AppendUser("turn4")
	g.AppendAssistant("reply4")
	g.MarkProtected("turn4")
	nodes = g.Nodes()
	if nodes[2].Protected {
		t.Fatalf("expected turn2 user node to be unprotected after recalculation")
	}
}

func TestGraph_MarkProtected_DoesNotBlanketProtectFileRead(t *testing.T) {
	g := ctxpkg.NewGraph()
	// turn 1: old fileread result
	g.AppendUser("turn1")
	g.AppendAssistant("reply1")
	g.AppendToolUse("call-fileread", "fileread", []byte(`{"path":"README.md"}`))
	g.AppendToolResult("call-fileread", "fileread", "plain file content", ctxpkg.ProjectionFull, false, "", "")
	// turn 2
	g.AppendUser("turn2")
	g.AppendAssistant("reply2")
	// turn 3
	g.AppendUser("turn3")
	g.AppendAssistant("reply3")
	// turn 4: latest tool pair
	g.AppendUser("turn4")
	g.AppendAssistant("reply4")
	g.AppendToolUse("call-bash", "bash", []byte(`{"command":"pwd"}`))
	g.AppendToolResult("call-bash", "bash", "/tmp/project", ctxpkg.ProjectionFull, false, "", "")

	g.MarkProtected("check /tmp/target.txt")

	nodes := g.Nodes()
	var oldFileReadResult *ctxpkg.Node
	for _, n := range nodes {
		if n.CallID == "call-fileread" && n.Kind == ctxpkg.NodeKindToolResult {
			oldFileReadResult = n
			break
		}
	}
	if oldFileReadResult == nil {
		t.Fatalf("expected fileread tool result node")
	}
	if oldFileReadResult.Protected {
		t.Fatalf("expected old fileread result to stay unprotected when no path match exists")
	}
}

func TestGraph_ForceConsolidate(t *testing.T) {
	g := ctxpkg.NewGraph()
	// 오래된 turn 3개 (최신 ProtectedTurns=2 밖에 있음)
	g.AppendUser("old message 1")
	g.AppendAssistant("old reply 1")
	g.AppendUser("old message 2")
	g.AppendAssistant("old reply 2")
	g.AppendUser("old message 3")
	g.AppendAssistant("old reply 3")
	// 보호될 최신 2 turn
	g.AppendUser("recent 1")
	g.AppendAssistant("recent reply 1")
	g.AppendUser("current")
	g.AppendAssistant("current reply")

	g.MarkProtected("current")
	g.ForceConsolidate("요약된 이전 대화")

	nodes := g.Nodes()
	hasSummary := false
	for _, n := range nodes {
		if n.Kind == ctxpkg.NodeKindSummary {
			hasSummary = true
			if n.Text != "요약된 이전 대화" {
				t.Errorf("summary text mismatch: %s", n.Text)
			}
		}
	}
	if !hasSummary {
		t.Error("expected summary node after ForceConsolidate")
	}
}

// --- TokenEstimator 테스트 ---

func TestTokenEstimator_EstimateText(t *testing.T) {
	est := ctxpkg.NewTokenEstimator()
	// 400 chars → ~100 tokens
	text := make([]byte, 400)
	for i := range text {
		text[i] = 'a'
	}
	tokens := est.EstimateText(string(text))
	if tokens < 90 || tokens > 110 {
		t.Errorf("expected ~100 tokens, got %d", tokens)
	}
}

func TestTokenEstimator_BudgetFor(t *testing.T) {
	est := ctxpkg.NewTokenEstimator()
	// contextWindow=200000, system=3000, currentUserChars=400
	budget := est.BudgetFor(200_000, 3_000, 400)
	// reserved = 40000, userTokens = 100
	// expected = 200000 - 3000 - 40000 - 100 = 156900
	if budget < 150_000 || budget > 160_000 {
		t.Errorf("budget out of expected range: %d", budget)
	}
}

// --- Distiller 테스트 ---

func TestDistiller_FULL_SmallOutput(t *testing.T) {
	store := ctxpkg.NewArtifactStore(t.TempDir(), "test-session")
	mf := ctxpkg.NewArtifactManifest()
	dist := ctxpkg.NewDistiller(store, mf, nil)

	small := "hello world"
	result := dist.Distill(1, "bash", "call-1", small, false)

	if result.Projection != ctxpkg.ProjectionFull {
		t.Errorf("expected FULL, got %s", result.Projection)
	}
	if result.InlineText != small {
		t.Errorf("expected inline text = %q, got %q", small, result.InlineText)
	}
}

func TestDistiller_PARTIAL_LargeOutput(t *testing.T) {
	store := ctxpkg.NewArtifactStore(t.TempDir(), "test-session")
	mf := ctxpkg.NewArtifactManifest()
	dist := ctxpkg.NewDistiller(store, mf, nil)

	// 25000자 이상 → PARTIAL 이상
	large := make([]byte, 25_001)
	for i := range large {
		large[i] = 'x'
	}
	result := dist.Distill(1, "bash", "call-1", string(large), false)

	if result.Projection != ctxpkg.ProjectionPartial && result.Projection != ctxpkg.ProjectionSummary {
		t.Errorf("expected PARTIAL or SUMMARY, got %s", result.Projection)
	}
	if result.ArtifactID == "" {
		t.Error("expected artifact to be saved")
	}
}

func TestDistiller_ErrorPrefix_Preserved(t *testing.T) {
	store := ctxpkg.NewArtifactStore(t.TempDir(), "test-session")
	mf := ctxpkg.NewArtifactManifest()
	dist := ctxpkg.NewDistiller(store, mf, nil)

	// 에러 출력: 첫 문단에 에러 메시지 포함, 이후 대량의 내용
	errContent := "Error: permission denied\nfile: /etc/shadow\n\n" + string(make([]byte, 25_000))
	result := dist.Distill(1, "bash", "call-err", errContent, true)

	// 에러 첫 문단이 inline text에 포함되어야 함
	if !containsPrefix(result.InlineText, "Error: permission denied") {
		t.Errorf("error prefix not preserved in inline text")
	}
}

func containsPrefix(text, prefix string) bool {
	return len(text) >= len(prefix) && text[:len(prefix)] == prefix
}

// --- Renderer 테스트 ---

func TestRenderForLLM_EmptyGraph(t *testing.T) {
	g := ctxpkg.NewGraph()
	est := ctxpkg.NewTokenEstimator()
	msgs := ctxpkg.RenderForLLM(g, est, 1000, 200_000, "")
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages, got %d", len(msgs))
	}
}

func TestRenderForLLM_ToolAdjacency(t *testing.T) {
	g := ctxpkg.NewGraph()
	g.AppendUser("run it")
	g.AppendAssistant("ok")
	g.AppendToolUse("c1", "bash", []byte(`{}`))
	g.AppendToolResult("c1", "bash", "done", ctxpkg.ProjectionFull, false, "", "")

	est := ctxpkg.NewTokenEstimator()
	msgs := ctxpkg.RenderForLLM(g, est, 500, 200_000, "run it")

	// tool_use + tool_result가 모두 포함되어야 함
	hasTU, hasTR := false, false
	for _, m := range msgs {
		for _, b := range m.Content {
			if b.Type == "tool_use" {
				hasTU = true
			}
			if b.Type == "tool_result" {
				hasTR = true
			}
		}
	}
	if !hasTU || !hasTR {
		t.Errorf("tool_use=%v tool_result=%v — both must be present", hasTU, hasTR)
	}
}

func TestRenderForLLM_FirstMessageMustBeUser(t *testing.T) {
	g := ctxpkg.NewGraph()
	// assistant가 먼저 오는 비정상 케이스
	g.AppendAssistant("stray assistant")
	g.AppendUser("hello")
	g.AppendAssistant("hi")

	est := ctxpkg.NewTokenEstimator()
	msgs := ctxpkg.RenderForLLM(g, est, 1000, 200_000, "hello")

	if len(msgs) == 0 {
		t.Fatal("expected messages")
	}
	if msgs[0].Role != "user" {
		t.Errorf("first message must be user, got %s", msgs[0].Role)
	}
}

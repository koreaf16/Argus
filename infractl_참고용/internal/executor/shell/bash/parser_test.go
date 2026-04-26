package bash

import (
	"context"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

func TestParse_simple(t *testing.T) {
	f, err := Parse(context.Background(), `echo hello`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f.Stmts) != 1 {
		t.Fatalf("expected 1 stmt, got %d", len(f.Stmts))
	}
}

func TestParse_pipe(t *testing.T) {
	f, err := Parse(context.Background(), `ls -la | grep foo | wc -l`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f.Stmts) == 0 {
		t.Fatal("expected statements")
	}
}

func TestParse_subshell(t *testing.T) {
	f, err := Parse(context.Background(), `echo $(whoami)`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var found bool
	Walk(f, func(n syntax.Node) bool {
		if _, ok := n.(*syntax.CmdSubst); ok {
			found = true
		}
		return true
	})
	if !found {
		t.Fatal("expected CmdSubst node")
	}
}

func TestParse_procSubst(t *testing.T) {
	f, err := Parse(context.Background(), `diff <(ls dir1) <(ls dir2)`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var count int
	Walk(f, func(n syntax.Node) bool {
		if _, ok := n.(*syntax.ProcSubst); ok {
			count++
		}
		return true
	})
	if count != 2 {
		t.Fatalf("expected 2 ProcSubst, got %d", count)
	}
}

func TestParse_heredoc(t *testing.T) {
	src := "cat <<EOF\nhello\nEOF"
	f, err := Parse(context.Background(), src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var found bool
	Walk(f, func(n syntax.Node) bool {
		if rd, ok := n.(*syntax.Redirect); ok && rd.Op == syntax.Hdoc {
			found = true
		}
		return true
	})
	if !found {
		t.Fatal("expected heredoc Redirect node")
	}
}

func TestParse_invalid(t *testing.T) {
	_, err := Parse(context.Background(), `echo $(`)
	if err == nil {
		t.Fatal("expected parse error for invalid bash")
	}
}

func TestWalk_count_calls(t *testing.T) {
	src := `echo foo; ls -la; cat file | grep pat`
	f, err := Parse(context.Background(), src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var n int
	Walk(f, func(node syntax.Node) bool {
		if _, ok := node.(*syntax.CallExpr); ok {
			n++
		}
		return true
	})
	if n < 4 {
		t.Fatalf("expected at least 4 CallExpr, got %d", n)
	}
}

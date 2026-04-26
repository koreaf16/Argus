package bash

import (
	"context"
	"testing"
)

func TestExtract_none(t *testing.T) {
	src := `echo hello`
	f, _ := Parse(context.Background(), src)
	hs := Extract(f, src)
	if len(hs) != 0 {
		t.Fatalf("expected 0 heredocs, got %d", len(hs))
	}
}

func TestExtract_basic_unquoted(t *testing.T) {
	src := "cat <<EOF\nhello world\nEOF"
	f, err := Parse(context.Background(), src)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	hs := Extract(f, src)
	if len(hs) != 1 {
		t.Fatalf("expected 1 heredoc, got %d", len(hs))
	}
	h := hs[0]
	if h.Delim != "EOF" {
		t.Errorf("delimiter: got %q, want %q", h.Delim, "EOF")
	}
	if h.Quoted {
		t.Error("expected Quoted=false for unquoted heredoc")
	}
	if h.Tabbed {
		t.Error("expected Tabbed=false for << (not <<-)")
	}
	if h.Body == "" {
		t.Error("expected non-empty body")
	}
}

func TestExtract_quoted_single(t *testing.T) {
	src := "cat <<'EOF'\nhello $world\nEOF"
	f, err := Parse(context.Background(), src)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	hs := Extract(f, src)
	if len(hs) != 1 {
		t.Fatalf("expected 1 heredoc, got %d", len(hs))
	}
	h := hs[0]
	if !h.Quoted {
		t.Error("expected Quoted=true for <<'EOF'")
	}
	if h.Delim != "EOF" {
		t.Errorf("delimiter: got %q, want %q", h.Delim, "EOF")
	}
}

func TestExtract_dash_tabbed(t *testing.T) {
	src := "cat <<-EOF\n\thello\nEOF"
	f, err := Parse(context.Background(), src)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	hs := Extract(f, src)
	if len(hs) != 1 {
		t.Fatalf("expected 1 heredoc, got %d", len(hs))
	}
	if !hs[0].Tabbed {
		t.Error("expected Tabbed=true for <<-")
	}
}

func TestExtract_quoted_double(t *testing.T) {
	src := `cat <<"EOF"` + "\nhello\nEOF"
	f, err := Parse(context.Background(), src)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	hs := Extract(f, src)
	if len(hs) != 1 {
		t.Fatalf("expected 1 heredoc, got %d", len(hs))
	}
	if !hs[0].Quoted {
		t.Error("expected Quoted=true for <<\"EOF\"")
	}
}

func TestExtract_cmd_subst_in_unquoted_body(t *testing.T) {
	// 비따옴표 heredoc 본문에 명령 치환 — Body 에 포함되어야 함
	src := "cat <<EOF\n$(whoami)\nEOF"
	f, err := Parse(context.Background(), src)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	hs := Extract(f, src)
	if len(hs) != 1 {
		t.Fatalf("expected 1 heredoc, got %d", len(hs))
	}
	if hs[0].Quoted {
		t.Error("expected Quoted=false for unquoted heredoc with $()")
	}
}

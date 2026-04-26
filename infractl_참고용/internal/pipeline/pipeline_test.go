package pipeline

import (
	"strings"
	"testing"
)

func TestStripANSI(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"\x1b[31mred\x1b[0m", "red"},
		{"\x1b[1;32mbold green\x1b[0m text", "bold green text"},
		{"no escape", "no escape"},
		{"", ""},
	}
	for _, tc := range cases {
		got := StripANSI(tc.input)
		if got != tc.want {
			t.Fatalf("StripANSI(%q) = %q; want %q", tc.input, got, tc.want)
		}
	}
}

func TestCompressWhitespace(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"  hello   world  ", "hello world"},
		{"a\t\tb", "a b"},
		{"no   extra", "no extra"},
		{"", ""},
	}
	for _, tc := range cases {
		got := CompressWhitespace(tc.input)
		if got != tc.want {
			t.Fatalf("CompressWhitespace(%q) = %q; want %q", tc.input, got, tc.want)
		}
	}
}

func TestStripSQLPlusNoise(t *testing.T) {
	noiseLines := []string{
		"SQL*Plus: Release 19.0.0.0.0 - Production on Mon Apr 10 09:00:00 2026",
		"Copyright (c) 1982, 2021, Oracle.",
		"Connected to:",
		"SQL>",
		"SQL> SELECT 1 FROM DUAL;",
		"------------------------------",
	}
	for _, line := range noiseLines {
		if got := StripSQLPlusNoise(line); got != "" {
			t.Fatalf("StripSQLPlusNoise(%q) = %q; want empty string", line, got)
		}
	}

	kept := "EMPNO ENAME"
	if got := StripSQLPlusNoise(kept); got != kept {
		t.Fatalf("StripSQLPlusNoise(%q) = %q; want unchanged", kept, got)
	}
}

func TestStripMySQLNoise(t *testing.T) {
	noiseLines := []string{
		"mysql: [Warning] Using a password on the command line is insecure.",
		"Welcome to the MySQL monitor.",
		"mysql>",
		"+----+------+",
	}
	for _, line := range noiseLines {
		if got := StripMySQLNoise(line); got != "" {
			t.Fatalf("StripMySQLNoise(%q) = %q; want empty", line, got)
		}
	}
}

func TestStripPSQLNoise(t *testing.T) {
	noiseLines := []string{
		"psql (14.0)",
		`Type "help" for help.`,
		"SSL connection (protocol: TLSv1.3)",
		"mydb=# SELECT 1;",
		"Time: 0.123 ms",
	}
	for _, line := range noiseLines {
		if got := StripPSQLNoise(line); got != "" {
			t.Fatalf("StripPSQLNoise(%q) = %q; want empty", line, got)
		}
	}
}

func TestSmartTruncate_NoTruncation(t *testing.T) {
	lines := []string{"line1", "line2", "line3"}
	result := SmartTruncate(lines, 10000, 50, 50)
	if !strings.Contains(result, "line1") || !strings.Contains(result, "line3") {
		t.Fatalf("unexpected truncation: %q", result)
	}
}

func TestSmartTruncate_HeadAndTailPreserved(t *testing.T) {
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = "XX"
	}
	lines[0] = "HEAD"
	lines[99] = "TAIL"

	result := SmartTruncate(lines, 200, 5, 5)
	if !strings.Contains(result, "HEAD") {
		t.Fatal("HEAD line should be preserved")
	}
	if !strings.Contains(result, "TAIL") {
		t.Fatal("TAIL line should be preserved")
	}
	if !strings.Contains(result, "...") {
		t.Fatal("omission message should be present")
	}
}

func TestSmartTruncate_ExactBoundary(t *testing.T) {
	lines := []string{"ab", "cd"}
	joined := "ab\ncd"
	result := SmartTruncate(lines, 5, 50, 50)
	if result != joined {
		t.Fatalf("exact fit: got %q; want %q", result, joined)
	}
}

func TestSmartTruncateString(t *testing.T) {
	s := strings.Repeat("x", 200)
	result := SmartTruncateString(s, 100, 10, 10)
	if len(result) <= 100 {
		t.Fatalf("result length=%d; want > 100", len(result))
	}
}

func TestSmartTruncate_TailOnly(t *testing.T) {
	lines := make([]string, 20)
	for i := range lines {
		lines[i] = "mid"
	}
	lines[19] = "ERROR: something failed"

	result := SmartTruncate(lines, 200, 2, 5)
	if !strings.Contains(result, "ERROR: something failed") {
		t.Fatal("tail error line should be preserved")
	}
}

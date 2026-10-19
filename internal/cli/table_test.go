package cli

import (
	"bytes"
	"testing"
)

func TestVisibleWidth(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"hello", 5},
		{"", 0},
		{"\x1b[31mhello\x1b[0m", 5},
		{"\x1b[1;32mworld\x1b[0m", 5},
		{"café", 4},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := visibleWidth(tt.input)
			if got != tt.expected {
				t.Errorf("visibleWidth(%q) = %d; want %d", tt.input, got, tt.expected)
			}
		})
	}
}

func TestRenderTable(t *testing.T) {
	rows := [][]string{
		{"NAME", "STATUS", "HOSTS"},
		{"web-prod", "\x1b[32mrunning\x1b[0m", "prod-1, prod-2"},
		{"db-main", "stopped", "db-1"},
	}

	var buf bytes.Buffer
	renderTable(&buf, rows)

	expected := "NAME      STATUS   HOSTS\nweb-prod  \x1b[32mrunning\x1b[0m  prod-1, prod-2\ndb-main   stopped  db-1\n"
	if buf.String() != expected {
		t.Errorf("renderTable() =\n%q\nwant:\n%q", buf.String(), expected)
	}
}

func BenchmarkRenderTable(b *testing.B) {
	rows := [][]string{
		{"NAME", "STATUS", "WINDOWS", "HOSTS"},
		{"backend-prod", "\x1b[32mrunning\x1b[0m", "3 windows", "api-1.example.com, api-2.example.com"},
		{"frontend-dev", "stopped", "1 window", "dev.example.com"},
		{"database-cluster", "\x1b[32mrunning\x1b[0m", "5 windows", "db-primary, db-replica-1, db-replica-2"},
		{"staging-environment", "stopped", "2 windows", "staging.example.com"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		renderTable(&buf, rows)
	}
}

func BenchmarkVisibleWidth_Plain(b *testing.B) {
	s := "backend-prod-api-1.example.com"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = visibleWidth(s)
	}
}

func BenchmarkVisibleWidth_ANSI(b *testing.B) {
	s := "\x1b[32mrunning\x1b[0m"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = visibleWidth(s)
	}
}

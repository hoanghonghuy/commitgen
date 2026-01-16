package app

import "testing"

func TestShouldIgnore(t *testing.T) {
	tests := []struct {
		pattern string
		ignores []string
		want    bool
	}{
		{"go.sum", []string{"go.sum"}, true},
		{"pkg/go.sum", []string{"go.sum"}, true}, // base match
		{"README.md", []string{"go.sum"}, false},
		{"foo.map", []string{"*.map"}, true},
		{"bar.svg", []string{"*.svg"}, true},
		{"src/logo.svg", []string{"*.svg"}, true},
		{"pnpm-lock.yaml", []string{"pnpm-lock.yaml"}, true},
	}

	for _, tt := range tests {
		got := shouldIgnore(tt.pattern, tt.ignores)
		if got != tt.want {
			t.Errorf("shouldIgnore(%q, %v) = %v; want %v", tt.pattern, tt.ignores, got, tt.want)
		}
	}
}

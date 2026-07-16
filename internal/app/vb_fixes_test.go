package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/hoanghonghuy/commitgen/internal/i18n"
)

func TestShouldRunSuggestHeadless(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want bool
	}{
		{"tui default", Config{}, false},
		{"print", Config{Print: true}, true},
		{"dry-run", Config{DryRun: true}, true},
		{"hook alone", Config{HookFile: "/tmp/COMMIT_EDITMSG"}, true},
		{"hook spaces", Config{HookFile: "  "}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldRunSuggestHeadless(tt.cfg); got != tt.want {
				t.Fatalf("got %v want %v", got, tt.want)
			}
		})
	}
}

func TestResolveConfigAction(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{nil, ""},
		{[]string{"config"}, ""},
		{[]string{"config", "show"}, "show"},
		{[]string{"config", "path"}, "path"},
		{[]string{"show"}, "show"},
		{[]string{"path"}, "path"},
		{[]string{"other"}, ""},
	}
	for _, tt := range tests {
		if got := ResolveConfigAction(tt.args); got != tt.want {
			t.Errorf("args=%v got %q want %q", tt.args, got, tt.want)
		}
	}
}

func TestReview_CopyDoneReturnsToQuickWhenQuickMode(t *testing.T) {
	tr := i18n.New(i18n.LocaleEN)
	m := newReviewModel(context.Background(), fakeProvider{}, baseMsgs(), 0.7, 5*time.Second, true, tr)
	m.state = reviewStateCopied
	m.isQuickMode = true
	u, _ := m.Update(reviewCopyDoneMsg{})
	rm := u.(reviewModel)
	if rm.state != reviewStateQuickDone {
		t.Errorf("got state %v; want quick done", rm.state)
	}
}

func TestScrollHintText_UsesTranslator(t *testing.T) {
	tr := i18n.New(i18n.LocaleEN)
	s := scrollHintText(tr, 50, false, false)
	if !strings.Contains(s, "50%") {
		t.Fatalf("got %q", s)
	}
	if !strings.Contains(strings.ToLower(s), "copy") {
		t.Fatalf("expected copy hint, got %q", s)
	}
}

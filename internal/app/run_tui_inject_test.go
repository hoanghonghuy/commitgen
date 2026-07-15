package app

import (
	"context"
	"errors"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// withStubTUI temporarily replaces runTUI and restores it after the test.
func withStubTUI(t *testing.T, fn func(context.Context, tea.Model) (tea.Model, error)) {
	t.Helper()
	orig := runTUI
	runTUI = fn
	t.Cleanup(func() { runTUI = orig })
}

func TestRun_SuggestSuccess(t *testing.T) {
	dir := stagedRepo(t)
	withStubTUI(t, func(_ context.Context, model tea.Model) (tea.Model, error) {
		// Return the model as-is (no error) to simulate a clean TUI session.
		return model, nil
	})

	cfg := Config{
		Command:  "suggest",
		RepoArg:  dir,
		Provider: "openai",
		Model:    "gpt-4o",
		APIKey:   "k",
		RecentN:  2,
		MaxFiles: 5,
		Timeout:  time.Second,
	}
	if err := Run(context.Background(), cfg); err != nil {
		t.Fatalf("Run(suggest) error: %v", err)
	}
}

func TestRun_SuggestModelError(t *testing.T) {
	dir := stagedRepo(t)
	withStubTUI(t, func(_ context.Context, model tea.Model) (tea.Model, error) {
		m := model.(tuiModel)
		m.err = errors.New("tui blew up")
		return m, nil
	})

	cfg := Config{Command: "suggest", Provider: "openai", Model: "gpt-4o", APIKey: "k", RepoArg: dir, MaxFiles: 5, Timeout: time.Second}
	if err := Run(context.Background(), cfg); err == nil {
		t.Error("expected error when final tui model carries an error")
	}
}

func TestRun_SuggestProgramError(t *testing.T) {
	dir := stagedRepo(t)
	withStubTUI(t, func(_ context.Context, model tea.Model) (tea.Model, error) {
		return model, errors.New("program crashed")
	})

	cfg := Config{Command: "suggest", Provider: "openai", Model: "gpt-4o", APIKey: "k", RepoArg: dir, MaxFiles: 5, Timeout: time.Second}
	if err := Run(context.Background(), cfg); err == nil {
		t.Error("expected error when runTUI returns an error")
	}
}

func TestRun_SuggestMissingProvider(t *testing.T) {
	dir := stagedRepo(t)
	// No model configured → newProvider fails before runTUI is called.
	cfg := Config{Command: "suggest", Provider: "openai", RepoArg: dir, MaxFiles: 5, Timeout: time.Second}
	if err := Run(context.Background(), cfg); err == nil {
		t.Error("expected provider configuration error")
	}
}

func TestRun_ReviewSuccess(t *testing.T) {
	dir := stagedRepo(t)
	withStubTUI(t, func(_ context.Context, model tea.Model) (tea.Model, error) {
		return model, nil
	})

	cfg := Config{
		Command:        "review",
		RepoArg:        dir,
		Provider:       "openai",
		Model:          "gpt-4o",
		APIKey:         "k",
		ReviewLanguage: "vi",
		MaxFiles:       5,
		Timeout:        time.Second,
	}
	if err := Run(context.Background(), cfg); err != nil {
		t.Fatalf("Run(review) error: %v", err)
	}
}

func TestRun_ReviewSwitchToSuggest(t *testing.T) {
	dir := stagedRepo(t)
	calls := 0
	withStubTUI(t, func(_ context.Context, model tea.Model) (tea.Model, error) {
		calls++
		if rm, ok := model.(reviewModel); ok {
			// First call is the review model → request switch to suggest.
			rm.switchToSuggest = true
			return rm, nil
		}
		// Second call is the follow-up suggest model.
		return model, nil
	})

	cfg := Config{Command: "review", Provider: "openai", Model: "gpt-4o", APIKey: "k", RepoArg: dir, MaxFiles: 5, Timeout: time.Second}
	if err := Run(context.Background(), cfg); err != nil {
		t.Fatalf("Run(review→suggest) error: %v", err)
	}
	if calls != 2 {
		t.Errorf("expected runTUI called twice (review then suggest), got %d", calls)
	}
}

func TestRun_ReviewModelError(t *testing.T) {
	dir := stagedRepo(t)
	withStubTUI(t, func(_ context.Context, model tea.Model) (tea.Model, error) {
		rm := model.(reviewModel)
		rm.err = errors.New("review failed")
		return rm, nil
	})

	cfg := Config{Command: "review", Provider: "openai", Model: "gpt-4o", APIKey: "k", RepoArg: dir, MaxFiles: 5, Timeout: time.Second}
	if err := Run(context.Background(), cfg); err == nil {
		t.Error("expected error when review model carries an error")
	}
}

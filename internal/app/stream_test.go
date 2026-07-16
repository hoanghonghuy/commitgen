package app

import (
	"context"
	"errors"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hoanghonghuy/commitgen/internal/i18n"
	"github.com/hoanghonghuy/commitgen/internal/vscodeprompt"
)

// fakeStreamProvider implements both ai.Provider and ai.StreamProvider.
type fakeStreamProvider struct {
	deltas []string
	err    error
}

func (f fakeStreamProvider) Generate(_ context.Context, _ []vscodeprompt.VSCodeMessage, _ float64) (string, error) {
	full := ""
	for _, d := range f.deltas {
		full += d
	}
	return full, f.err
}

func (f fakeStreamProvider) GenerateStream(_ context.Context, _ []vscodeprompt.VSCodeMessage, _ float64, onDelta func(string)) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	full := ""
	for _, d := range f.deltas {
		full += d
		if onDelta != nil {
			onDelta(d)
		}
	}
	return full, nil
}

func TestTui_StreamDeltaAccumulates(t *testing.T) {
	tr := i18n.New(i18n.LocaleEN)
	m := newTuiModel(context.Background(),"/repo", fakeStreamProvider{}, baseMsgs(), 0.7, time.Second, false, "", tr, nil)

	u, cmd := m.Update(streamEvent{delta: "feat: "})
	tm := u.(tuiModel)
	if tm.streamView != "feat: " {
		t.Errorf("streamView = %q", tm.streamView)
	}
	if cmd == nil {
		t.Error("delta should re-issue a wait command")
	}

	u, _ = tm.Update(streamEvent{delta: "add x"})
	tm = u.(tuiModel)
	if tm.streamView != "feat: add x" {
		t.Errorf("streamView = %q", tm.streamView)
	}
}

func TestTui_StreamDoneFinalizes(t *testing.T) {
	tr := i18n.New(i18n.LocaleEN)
	m := newTuiModel(context.Background(),"/repo", fakeStreamProvider{}, baseMsgs(), 0.7, time.Second, false, "", tr, nil)
	m.streamView = "partial"

	u, _ := m.Update(streamEvent{done: true, full: "```text\nfeat: streamed\n```"})
	tm := u.(tuiModel)
	if tm.state != stateConfirm {
		t.Errorf("expected confirm state, got %v", tm.state)
	}
	if tm.commitMsg != "feat: streamed" {
		t.Errorf("commitMsg = %q", tm.commitMsg)
	}
	if tm.streamView != "" {
		t.Error("streamView should be cleared after done")
	}
}

func TestTui_StreamDoneError(t *testing.T) {
	tr := i18n.New(i18n.LocaleEN)
	m := newTuiModel(context.Background(),"/repo", fakeStreamProvider{}, baseMsgs(), 0.7, time.Second, false, "", tr, nil)
	u, cmd := m.Update(streamEvent{done: true, err: errors.New("stream broke")})
	tm := u.(tuiModel)
	if tm.state != stateDone || tm.err == nil || cmd == nil {
		t.Errorf("stream error should move to done+quit; state=%v err=%v", tm.state, tm.err)
	}
}

func TestTui_StreamGenerateCmdEndToEnd(t *testing.T) {
	// startStreamCmd should push deltas then a final done event onto the channel.
	sp := fakeStreamProvider{deltas: []string{"feat: ", "stream"}}
	ch := make(chan streamEvent, 16)
	cmd := startStreamCmd(context.Background(), sp, ch, baseMsgs(), 0.7, time.Second, 1)
	cmd() // launches the goroutine

	var got string
	deadline := time.After(2 * time.Second)
	for {
		select {
		case ev := <-ch:
			if ev.done {
				if ev.full != "feat: stream" {
					t.Errorf("full = %q", ev.full)
				}
				return
			}
			got += ev.delta
		case <-deadline:
			t.Fatalf("timed out; accumulated %q", got)
		}
	}
}

func TestTui_StreamViewRendersPartial(t *testing.T) {
	tr := i18n.New(i18n.LocaleEN)
	m := newTuiModel(context.Background(), "/repo", fakeStreamProvider{}, baseMsgs(), 0.7, time.Second, false, "", tr, nil)
	m.state = stateGenerating
	m.streamView = "feat: partial message"
	view := m.View()
	if view == "" {
		t.Error("streaming view should render partial content")
	}
}

var _ tea.Model = tuiModel{}

type waitCtxStreamProvider struct{}

func (waitCtxStreamProvider) Generate(context.Context, []vscodeprompt.VSCodeMessage, float64) (string, error) {
	return "", errors.New("not used")
}

func (waitCtxStreamProvider) GenerateStream(ctx context.Context, _ []vscodeprompt.VSCodeMessage, _ float64, _ func(string)) (string, error) {
	<-ctx.Done()
	return "", ctx.Err()
}

func TestStartStreamCmd_RespectsCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ch := make(chan streamEvent, 2)
	_ = startStreamCmd(ctx, waitCtxStreamProvider{}, ch, baseMsgs(), 0.7, time.Second, 1)()

	select {
	case ev := <-ch:
		if ev.err == nil {
			t.Fatal("expected canceled error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for stream event")
	}
}

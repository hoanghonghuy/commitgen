package i18n

import (
	"testing"
)

func TestNormalizeLocale(t *testing.T) {
	tests := []struct {
		in   string
		want Locale
	}{
		{"vi_VN.UTF-8", LocaleVI},
		{"ja_JP", LocaleJA},
		{"zh_CN", LocaleZH},
		{"en_US", LocaleEN},
		{"C", ""},
		{"de_DE", ""},
	}
	for _, tt := range tests {
		if got := NormalizeLocale(tt.in); got != tt.want {
			t.Errorf("NormalizeLocale(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestResolveLocale_Auto(t *testing.T) {
	t.Setenv("LANG", "vi_VN.UTF-8")
	if got := ResolveLocale("auto", "", "", "en"); got != LocaleVI {
		t.Fatalf("auto flag = %q, want vi", got)
	}
}

func TestResolveLocale_Priority(t *testing.T) {
	t.Setenv("LANG", "ja_JP")
	if got := ResolveLocale("", "vi", "auto", "en"); got != LocaleVI {
		t.Fatalf("env should beat file auto, got %q", got)
	}
}

func TestResolveLocale_FileAuto(t *testing.T) {
	t.Setenv("LANG", "zh_CN.UTF-8")
	if got := ResolveLocale("", "", "auto", "en"); got != LocaleZH {
		t.Fatalf("file auto = %q, want zh", got)
	}
}

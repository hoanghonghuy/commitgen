package i18n

import (
	"os"
	"strings"
)

// ResolveLocale picks the UI locale using flag > env > file > default. The value
// "auto" reads LC_ALL, LC_MESSAGES, or LANG from the environment.
func ResolveLocale(flagVal, envVal, fileVal, defaultVal string) Locale {
	for _, candidate := range []string{flagVal, envVal, fileVal, defaultVal} {
		if loc := resolveLocaleSetting(candidate); loc != "" {
			return loc
		}
	}
	return LocaleEN
}

func resolveLocaleSetting(s string) Locale {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if strings.EqualFold(s, "auto") {
		if loc := DetectFromEnvironment(); loc != "" {
			return loc
		}
		return LocaleEN
	}
	return NormalizeLocale(s)
}

// DetectFromEnvironment maps OS locale variables to a supported Locale.
func DetectFromEnvironment() Locale {
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(key); v != "" {
			if loc := NormalizeLocale(v); loc != "" {
				return loc
			}
		}
	}
	return ""
}

// NormalizeLocale maps a locale code or LANG value to a supported Locale.
func NormalizeLocale(s string) Locale {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" || s == "c" || s == "posix" {
		return ""
	}
	if i := strings.IndexAny(s, ".@"); i >= 0 {
		s = s[:i]
	}
	if i := strings.IndexByte(s, '_'); i >= 0 {
		s = s[:i]
	}
	if i := strings.IndexByte(s, '-'); i >= 0 {
		s = s[:i]
	}
	switch s {
	case "vi":
		return LocaleVI
	case "ja", "jp":
		return LocaleJA
	case "zh", "cn", "tw":
		return LocaleZH
	case "en":
		return LocaleEN
	default:
		return ""
	}
}

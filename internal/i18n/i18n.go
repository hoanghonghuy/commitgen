package i18n

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

// Locale represents a language/locale identifier.
type Locale string

const (
	LocaleEN Locale = "en"
	LocaleVI Locale = "vi"
	LocaleJA Locale = "ja"
	LocaleZH Locale = "zh"
)

//go:embed en.json
var enJSON []byte

//go:embed vi.json
var viJSON []byte

//go:embed ja.json
var jaJSON []byte

//go:embed zh.json
var zhJSON []byte

// Translator provides locale-aware message translation with English fallback.
type Translator struct {
	locale   Locale
	messages map[Locale]map[string]string
}

// New creates a Translator for the given locale. Messages are loaded from
// embedded JSON files at init time.
func New(locale Locale) *Translator {
	return &Translator{
		locale:   locale,
		messages: loadMessages(),
	}
}

// Locale returns the translator's current locale.
func (t *Translator) Locale() Locale {
	return t.locale
}

// T returns the translated message for the given key. If the key is not found
// in the current locale, it falls back to English. If still not found, the key
// itself is returned. Optional args are passed to fmt.Sprintf for formatting.
func (t *Translator) T(key string, args ...interface{}) string {
	msg, ok := t.messages[t.locale][key]
	if !ok {
		// Fallback to English
		msg = t.messages[LocaleEN][key]
	}
	if msg == "" {
		return key
	}
	if len(args) > 0 {
		return fmt.Sprintf(msg, args...)
	}
	return msg
}

// loadMessages parses all embedded locale JSON files into a nested map.
func loadMessages() map[Locale]map[string]string {
	result := make(map[Locale]map[string]string)

	load := func(loc Locale, data []byte) {
		var m map[string]string
		if err := json.Unmarshal(data, &m); err != nil {
			// Embedded files are validated at build time; a parse error here
			// indicates a programming error that should be caught in tests.
			panic(fmt.Sprintf("i18n: failed to parse %s.json: %v", loc, err))
		}
		result[loc] = m
	}

	load(LocaleEN, enJSON)
	load(LocaleVI, viJSON)
	load(LocaleJA, jaJSON)
	load(LocaleZH, zhJSON)

	return result
}

// keysForLocale returns the set of keys defined for a locale. Exported for
// test validation only.
func keysForLocale(loc Locale) map[string]struct{} {
	msgs := loadMessages()
	m, ok := msgs[loc]
	if !ok {
		return nil
	}
	keys := make(map[string]struct{}, len(m))
	for k := range m {
		keys[k] = struct{}{}
	}
	return keys
}

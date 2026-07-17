// Package i18n provides internationalization support for taichi.
//
// Design highlights:
//   - Standard library implementation, zero third-party dependencies
//   - Looks up translations by key, falling back to the key itself on miss
//   - Global translator + optional local translator, supporting concurrency-safe reads
//   - Auto-detects system locale (LANG / LC_ALL / LC_MESSAGES)
//
// Usage:
//
//	i18n.SetLocale(i18n.ZhCN)            // switch to Simplified Chinese
//	msg := i18n.T("run.short")           // look up translation
//	fmt.Println(msg)
//
// Language packs are auto-registered via init functions; callers do not need to
// register them manually.
package i18n

import (
	"os"
	"strings"
	"sync"
)

// Locale is the language region identifier.
//
// Currently supported:
//   - ZhCN: Simplified Chinese (zh-CN)
//   - EnUS: American English (en-US)
type Locale string

const (
	// ZhCN is Simplified Chinese.
	ZhCN Locale = "zh-CN"
	// EnUS is American English.
	EnUS Locale = "en-US"
	// Auto means auto-detect the system locale. Used only as a config input value;
	// it is resolved to a concrete Locale before SetLocale is called.
	Auto Locale = "auto"
)

// defaultLocale is the default language when nothing is explicitly set.
const defaultLocale = EnUS

// Translator holds the current language and its translation table.
//
// Reads are concurrency-safe (via RLock); language switching via SetLocale is serialized.
type Translator struct {
	mu       sync.RWMutex
	locale   Locale
	packages map[Locale]map[string]string
}

// global is the global translator instance.
var global = &Translator{
	locale:   defaultLocale,
	packages: make(map[Locale]map[string]string),
}

// Register registers a language pack. Repeated registration of the same Locale
// merges entries (later overwrites earlier).
//
// Should be called in an init function to ensure registration completes before
// the first translation lookup.
func Register(locale Locale, messages map[string]string) {
	global.mu.Lock()
	defer global.mu.Unlock()
	if global.packages[locale] == nil {
		global.packages[locale] = make(map[string]string, len(messages))
	}
	for k, v := range messages {
		global.packages[locale][k] = v
	}
}

// SetLocale sets the current language of the global translator.
//
// When Auto is passed, DetectLocale is called for auto-detection.
// Unregistered languages fall back to defaultLocale.
func SetLocale(locale Locale) {
	if locale == Auto {
		locale = DetectLocale()
	}
	global.mu.Lock()
	defer global.mu.Unlock()
	if _, ok := global.packages[locale]; !ok {
		locale = defaultLocale
	}
	global.locale = locale
}

// GetLocale returns the currently active language.
func GetLocale() Locale {
	global.mu.RLock()
	defer global.mu.RUnlock()
	return global.locale
}

// T looks up the translation for the given key. Falls back to the key itself on miss.
//
// Supports optional args, formatted in fmt.Sprintf style.
// Example: i18n.T("run.failed_count", 3) returns a localized message such as "3 test(s) failed" in en-US mode.
func T(key string, args ...any) string {
	return global.T(key, args...)
}

// T is the lookup method of Translator.
func (t *Translator) T(key string, args ...any) string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	msg, ok := t.packages[t.locale][key]
	if !ok {
		// Fall back to the default language.
		if t.locale != defaultLocale {
			if msg, ok = t.packages[defaultLocale][key]; !ok {
				msg = key
			}
		} else {
			msg = key
		}
	}

	if len(args) == 0 {
		return msg
	}
	return sprintf(msg, args...)
}

// DetectLocale infers the Locale from system environment variables.
//
// Check order: TAICHI_LOCALE > LC_ALL > LC_MESSAGES > LANG.
// Matching rule: starting with "zh" returns ZhCN; starting with "en" returns EnUS;
// otherwise falls back to defaultLocale.
func DetectLocale() Locale {
	for _, env := range []string{"TAICHI_LOCALE", "LC_ALL", "LC_MESSAGES", "LANG"} {
		val := os.Getenv(env)
		if val == "" {
			continue
		}
		low := strings.ToLower(val)
		switch {
		case strings.HasPrefix(low, "zh"):
			return ZhCN
		case strings.HasPrefix(low, "en"):
			return EnUS
		}
	}
	return defaultLocale
}

// ParseLocale parses a string into a Locale.
//
// Supported inputs:
//   - "" / "auto" → Auto
//   - "zh", "zh-CN", "zh_CN", "zh-cn" → ZhCN
//   - "en", "en-US", "en_US", "en-us" → EnUS
//
// Unrecognized values return Auto (further processed by SetLocale).
func ParseLocale(s string) Locale {
	s = strings.TrimSpace(s)
	if s == "" || strings.EqualFold(s, "auto") {
		return Auto
	}
	low := strings.ToLower(s)
	// Normalize the separator: convert underscores to hyphens.
	low = strings.ReplaceAll(low, "_", "-")
	switch {
	case low == "zh", strings.HasPrefix(low, "zh-"):
		return ZhCN
	case low == "en", strings.HasPrefix(low, "en-"):
		return EnUS
	}
	return Auto
}

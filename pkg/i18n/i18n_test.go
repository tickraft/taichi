package i18n

import (
	"strings"
	"testing"
)

// TestLocaleConstants verifies the Locale constant values.
func TestLocaleConstants(t *testing.T) {
	if ZhCN != "zh-CN" {
		t.Errorf("ZhCN = %q, want %q", ZhCN, "zh-CN")
	}
	if EnUS != "en-US" {
		t.Errorf("EnUS = %q, want %q", EnUS, "en-US")
	}
	if Auto != "auto" {
		t.Errorf("Auto = %q, want %q", Auto, "auto")
	}
}

// TestParseLocale is a table-driven test covering various locale string inputs.
func TestParseLocale(t *testing.T) {
	cases := []struct {
		input string
		want  Locale
	}{
		{"", Auto},
		{"auto", Auto},
		{"AUTO", Auto},
		{"Auto", Auto},
		{"zh", ZhCN},
		{"zh-CN", ZhCN},
		{"zh_CN", ZhCN},
		{"zh-cn", ZhCN},
		{"ZH", ZhCN},
		{"en", EnUS},
		{"en-US", EnUS},
		{"en_US", EnUS},
		{"en-us", EnUS},
		{"EN", EnUS},
		{"fr", Auto},
		{"ja-JP", Auto},
		{"  zh-CN  ", ZhCN},
	}
	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			got := ParseLocale(c.input)
			if got != c.want {
				t.Errorf("ParseLocale(%q) = %q, want %q", c.input, got, c.want)
			}
		})
	}
}

// TestSetLocale verifies locale switching and fallback behavior.
func TestSetLocale(t *testing.T) {
	// Save and restore the original locale.
	original := GetLocale()
	defer SetLocale(original)

	t.Run("set EnUS", func(t *testing.T) {
		SetLocale(EnUS)
		if got := GetLocale(); got != EnUS {
			t.Errorf("GetLocale() = %q, want %q", got, EnUS)
		}
	})

	t.Run("set ZhCN", func(t *testing.T) {
		SetLocale(ZhCN)
		if got := GetLocale(); got != ZhCN {
			t.Errorf("GetLocale() = %q, want %q", got, ZhCN)
		}
	})

	t.Run("set Auto resolves to DetectLocale", func(t *testing.T) {
		SetLocale(Auto)
		got := GetLocale()
		want := DetectLocale()
		if got != want {
			t.Errorf("GetLocale() = %q, want %q (DetectLocale)", got, want)
		}
	})

	t.Run("unregistered locale falls back to default", func(t *testing.T) {
		SetLocale(Locale("fr"))
		if got := GetLocale(); got != defaultLocale {
			t.Errorf("GetLocale() = %q, want %q (defaultLocale)", got, defaultLocale)
		}
	})
}

// TestT verifies translation lookups for known keys in both locales,
// unknown key fallback, and format-arg substitution.
func TestT(t *testing.T) {
	original := GetLocale()
	defer SetLocale(original)

	t.Run("English value for known key", func(t *testing.T) {
		SetLocale(EnUS)
		got := T("cli.root.short")
		want := "taichi test orchestration framework"
		if got != want {
			t.Errorf("T(\"cli.root.short\") = %q, want %q", got, want)
		}
	})

	t.Run("Chinese value for known key", func(t *testing.T) {
		SetLocale(ZhCN)
		got := T("cli.root.short")
		want := "taichi 测试编排框架"
		if got != want {
			t.Errorf("T(\"cli.root.short\") = %q, want %q", got, want)
		}
	})

	t.Run("unknown key returns the key itself", func(t *testing.T) {
		SetLocale(EnUS)
		got := T("nonexistent.key.12345")
		if got != "nonexistent.key.12345" {
			t.Errorf("T(unknown) = %q, want %q", got, "nonexistent.key.12345")
		}
	})

	t.Run("args are formatted into the message", func(t *testing.T) {
		SetLocale(EnUS)
		got := T("cli.run.output.failed_count", 3)
		if !strings.Contains(got, "3") {
			t.Errorf("T(\"cli.run.output.failed_count\", 3) = %q, expected to contain \"3\"", got)
		}
		if !strings.Contains(got, "failed") {
			t.Errorf("T(\"cli.run.output.failed_count\", 3) = %q, expected to contain \"failed\"", got)
		}
	})

	t.Run("Chinese args formatting", func(t *testing.T) {
		SetLocale(ZhCN)
		got := T("cli.run.output.failed_count", 5)
		if !strings.Contains(got, "5") {
			t.Errorf("T(\"cli.run.output.failed_count\", 5) = %q, expected to contain \"5\"", got)
		}
		if !strings.Contains(got, "失败") {
			t.Errorf("T(\"cli.run.output.failed_count\", 5) = %q, expected to contain \"失败\"", got)
		}
	})
}

// TestTMissingKey verifies that unknown keys fall back to the key string,
// with and without format args.
func TestTMissingKey(t *testing.T) {
	original := GetLocale()
	defer SetLocale(original)
	SetLocale(EnUS)

	t.Run("unknown key without args returns key", func(t *testing.T) {
		key := "totally.nonexistent.key"
		got := T(key)
		if got != key {
			t.Errorf("T(%q) = %q, want %q", key, got, key)
		}
	})

	t.Run("unknown key with args but no verbs starts with key", func(t *testing.T) {
		// fmt.Sprintf with extra args and no verbs appends a %!(EXTRA ...) suffix.
		key := "another.missing.key"
		got := T(key, 42, "hello")
		if !strings.HasPrefix(got, key) {
			t.Errorf("T(%q, 42, \"hello\") = %q, expected to start with %q", key, got, key)
		}
		if !strings.Contains(got, "EXTRA") {
			t.Errorf("T(%q, 42, \"hello\") = %q, expected to contain EXTRA marker", key, got)
		}
	})

	t.Run("unknown key with format verb is formatted", func(t *testing.T) {
		key := "missing.%d.count"
		got := T(key, 7)
		want := "missing.7.count"
		if got != want {
			t.Errorf("T(%q, 7) = %q, want %q", key, got, want)
		}
	})
}

// TestRegister verifies that a newly registered locale can be activated and queried.
func TestRegister(t *testing.T) {
	const testLocale Locale = "test-register"
	Register(testLocale, map[string]string{
		"greeting": "Bonjour",
		"farewell": "Au revoir",
	})

	original := GetLocale()
	defer SetLocale(original)
	SetLocale(testLocale)

	if got := GetLocale(); got != testLocale {
		t.Fatalf("GetLocale() = %q, want %q", got, testLocale)
	}

	if got := T("greeting"); got != "Bonjour" {
		t.Errorf("T(\"greeting\") = %q, want %q", got, "Bonjour")
	}
	if got := T("farewell"); got != "Au revoir" {
		t.Errorf("T(\"farewell\") = %q, want %q", got, "Au revoir")
	}
}

// TestRegisterMerge verifies that re-registering the same locale merges entries
// with later values overwriting earlier ones.
func TestRegisterMerge(t *testing.T) {
	const testLocale Locale = "test-merge"

	// First registration.
	Register(testLocale, map[string]string{
		"key1": "value1",
		"key2": "value2",
	})

	// Second registration merges and overwrites key1.
	Register(testLocale, map[string]string{
		"key1": "overwritten",
		"key3": "value3",
	})

	original := GetLocale()
	defer SetLocale(original)
	SetLocale(testLocale)

	// key1 should be overwritten by the second registration.
	if got := T("key1"); got != "overwritten" {
		t.Errorf("T(\"key1\") = %q, want %q (overwritten)", got, "overwritten")
	}
	// key2 from the first registration should still be present.
	if got := T("key2"); got != "value2" {
		t.Errorf("T(\"key2\") = %q, want %q (preserved from first registration)", got, "value2")
	}
	// key3 from the second registration should be present.
	if got := T("key3"); got != "value3" {
		t.Errorf("T(\"key3\") = %q, want %q", got, "value3")
	}
}

// TestDetectLocale verifies locale detection from environment variables.
// Uses t.Setenv for automatic restoration of original values.
func TestDetectLocale(t *testing.T) {
	// Helper to clear all locale-related env vars within a subtest.
	clearAll := func(t *testing.T) {
		t.Setenv("TAICHI_LOCALE", "")
		t.Setenv("LC_ALL", "")
		t.Setenv("LC_MESSAGES", "")
		t.Setenv("LANG", "")
	}

	t.Run("TAICHI_LOCALE=zh_CN returns ZhCN", func(t *testing.T) {
		clearAll(t)
		t.Setenv("TAICHI_LOCALE", "zh_CN")
		if got := DetectLocale(); got != ZhCN {
			t.Errorf("DetectLocale() = %q, want %q", got, ZhCN)
		}
	})

	t.Run("TAICHI_LOCALE=en_US returns EnUS", func(t *testing.T) {
		clearAll(t)
		t.Setenv("TAICHI_LOCALE", "en_US")
		if got := DetectLocale(); got != EnUS {
			t.Errorf("DetectLocale() = %q, want %q", got, EnUS)
		}
	})

	t.Run("LC_ALL=zh_CN.UTF-8 returns ZhCN", func(t *testing.T) {
		clearAll(t)
		t.Setenv("LC_ALL", "zh_CN.UTF-8")
		if got := DetectLocale(); got != ZhCN {
			t.Errorf("DetectLocale() = %q, want %q", got, ZhCN)
		}
	})

	t.Run("LANG=en_US.UTF-8 returns EnUS", func(t *testing.T) {
		clearAll(t)
		t.Setenv("LANG", "en_US.UTF-8")
		if got := DetectLocale(); got != EnUS {
			t.Errorf("DetectLocale() = %q, want %q", got, EnUS)
		}
	})

	t.Run("all env vars cleared returns defaultLocale", func(t *testing.T) {
		clearAll(t)
		if got := DetectLocale(); got != defaultLocale {
			t.Errorf("DetectLocale() = %q, want %q (defaultLocale)", got, defaultLocale)
		}
	})

	t.Run("TAICHI_LOCALE takes priority over LC_ALL", func(t *testing.T) {
		clearAll(t)
		t.Setenv("TAICHI_LOCALE", "en_US")
		t.Setenv("LC_ALL", "zh_CN.UTF-8")
		if got := DetectLocale(); got != EnUS {
			t.Errorf("DetectLocale() = %q, want %q (TAICHI_LOCALE wins)", got, EnUS)
		}
	})

	t.Run("LC_ALL takes priority over LANG", func(t *testing.T) {
		clearAll(t)
		t.Setenv("LC_ALL", "zh_CN.UTF-8")
		t.Setenv("LANG", "en_US.UTF-8")
		if got := DetectLocale(); got != ZhCN {
			t.Errorf("DetectLocale() = %q, want %q (LC_ALL wins)", got, ZhCN)
		}
	})

	t.Run("unrecognized language returns defaultLocale", func(t *testing.T) {
		clearAll(t)
		t.Setenv("LANG", "ja_JP.UTF-8")
		if got := DetectLocale(); got != defaultLocale {
			t.Errorf("DetectLocale() = %q, want %q (defaultLocale for ja)", got, defaultLocale)
		}
	})
}

// TestTranslatorT verifies the T method on a local (non-global) Translator instance.
func TestTranslatorT(t *testing.T) {
	tr := &Translator{
		locale: EnUS,
		packages: map[Locale]map[string]string{
			EnUS: {
				"hello":   "Hello, World!",
				"count":   "%d item(s)",
				"missing": "not really missing",
			},
			ZhCN: {
				"hello": "你好，世界！",
			},
		},
	}

	t.Run("known key in current locale", func(t *testing.T) {
		got := tr.T("hello")
		if got != "Hello, World!" {
			t.Errorf("T(\"hello\") = %q, want %q", got, "Hello, World!")
		}
	})

	t.Run("format args substitution", func(t *testing.T) {
		got := tr.T("count", 5)
		if got != "5 item(s)" {
			t.Errorf("T(\"count\", 5) = %q, want %q", got, "5 item(s)")
		}
	})

	t.Run("unknown key falls back to key itself", func(t *testing.T) {
		got := tr.T("nonexistent")
		if got != "nonexistent" {
			t.Errorf("T(\"nonexistent\") = %q, want %q", got, "nonexistent")
		}
	})

	t.Run("switch locale to ZhCN", func(t *testing.T) {
		tr.mu.Lock()
		tr.locale = ZhCN
		tr.mu.Unlock()
		defer func() {
			tr.mu.Lock()
			tr.locale = EnUS
			tr.mu.Unlock()
		}()

		got := tr.T("hello")
		if got != "你好，世界！" {
			t.Errorf("T(\"hello\") with ZhCN = %q, want %q", got, "你好，世界！")
		}
	})
}

// TestFallbackToDefault verifies that when the active locale is missing a key,
// the default locale's value is returned.
func TestFallbackToDefault(t *testing.T) {
	const partialLocale Locale = "test-fallback"
	Register(partialLocale, map[string]string{
		"only.in.partial": "partial value",
	})

	original := GetLocale()
	defer SetLocale(original)
	SetLocale(partialLocale)

	// Key exists only in default locale (EnUS) — should fall back.
	got := T("cli.root.short")
	want := "taichi test orchestration framework"
	if got != want {
		t.Errorf("T(\"cli.root.short\") fallback = %q, want %q", got, want)
	}

	// Key exists in the partial locale — should return the partial value.
	got = T("only.in.partial")
	if got != "partial value" {
		t.Errorf("T(\"only.in.partial\") = %q, want %q", got, "partial value")
	}

	// Key does not exist in any locale — should return the key itself.
	got = T("nonexistent.everywhere")
	if got != "nonexistent.everywhere" {
		t.Errorf("T(\"nonexistent.everywhere\") = %q, want %q", got, "nonexistent.everywhere")
	}
}

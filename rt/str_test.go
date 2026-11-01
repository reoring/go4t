package rt

import (
	"reflect"
	"testing"
)

func TestStringFunctions(t *testing.T) {
	installTestShapes(t)
	for _, tt := range []struct {
		name string
		call func() any
		want any
	}{
		{"strLength empty", func() any { return StrLength("") }, int64(0)},
		{"strLength runes", func() any { return StrLength("a日本🙂e\u0301") }, int64(6)},
		{"strConcat empty", func() any { return StrConcat(ListFromStrings(nil)) }, ""},
		{"strConcat", func() any { return StrConcat(ListFromStrings([]string{"a", "", "日本", "🙂"})) }, "a日本🙂"},
		{"join empty", func() any { return Join(",", ListFromStrings(nil)) }, ""},
		{"join", func() any { return Join(" / ", ListFromStrings([]string{"a", "", "日本"})) }, "a /  / 日本"},
		{"join singleton", func() any { return Join(",", ListFromStrings([]string{"a"})) }, "a"},
		{"split", func() any { return ListToStrings(Split(",", "a,,日本,")) }, []string{"a", "", "日本", ""}},
		{"split absent", func() any { return ListToStrings(Split(",", "one")) }, []string{"one"}},
		{"split empty", func() any { return ListToStrings(Split(",", "")) }, []string{""}},
		{"split runes", func() any { return ListToStrings(Split("", "a日本🙂e\u0301")) }, []string{"a", "日", "本", "🙂", "e", "\u0301"}},
		{"split empty runes", func() any { return ListToStrings(Split("", "")) }, []string(nil)},
		{"lines empty", func() any { return ListToStrings(Lines("")) }, []string(nil)},
		{"lines no newline", func() any { return ListToStrings(Lines("one")) }, []string{"one"}},
		{"lines final newline", func() any { return ListToStrings(Lines("one\ntwo\n")) }, []string{"one", "two"}},
		{"lines drop one empty", func() any { return ListToStrings(Lines("one\n\n")) }, []string{"one", ""}},
		{"lines blank", func() any { return ListToStrings(Lines("\n")) }, []string{""}},
		{"lines preserve CR", func() any { return ListToStrings(Lines("one\r\ntwo\r\n")) }, []string{"one\r", "two\r"}},
		{"words", func() any { return ListToStrings(Words("  one\t日本\n🙂\u2003two  ")) }, []string{"one", "日本", "🙂", "two"}},
		{"words empty", func() any { return ListToStrings(Words(" \n\t")) }, []string(nil)},
		{"trim", func() any { return Trim("\u2003 \t日本 text\r\n") }, "日本 text"},
		{"trim empty", func() any { return Trim("") }, ""},
		{"toUpper", func() any { return ToUpper("héllo 日本") }, "HÉLLO 日本"},
		{"toLower", func() any { return ToLower("HÉLLO 日本") }, "héllo 日本"},
		{"startsWith true", func() any { return StartsWith("日本語", "日本") }, true},
		{"startsWith false", func() any { return StartsWith("日本語", "語") }, false},
		{"startsWith empty", func() any { return StartsWith("日本語", "") }, true},
		{"endsWith true", func() any { return EndsWith("日本語", "語") }, true},
		{"endsWith false", func() any { return EndsWith("日本語", "日本") }, false},
		{"contains true", func() any { return Contains("日本語", "本") }, true},
		{"contains false", func() any { return Contains("日本語", "x") }, false},
		{"replace all", func() any { return Replace("one one one", "one", "二") }, "二 二 二"},
		{"replace empty", func() any { return Replace("日本", "", "-") }, "-日-本-"},
		{"replace absent", func() any { return Replace("one", "two", "three") }, "one"},
		{"chars", func() any { return ListToStrings(Chars("日🙂e\u0301")) }, []string{"日", "🙂", "e", "\u0301"}},
		{"chars empty", func() any { return ListToStrings(Chars("")) }, []string(nil)},
		{"repeat", func() any { return Repeat("日🙂", int64(3)) }, "日🙂日🙂日🙂"},
		{"repeat zero", func() any { return Repeat("word", int64(0)) }, ""},
		{"repeat negative", func() any { return Repeat("word", int64(-1)) }, ""},
		{"repeat minimum", func() any { return Repeat("word", int64(-1<<63)) }, ""},
		{"repeat empty", func() any { return Repeat("", int64(5)) }, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.call(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("result = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestSubstring(t *testing.T) {
	for _, tt := range []struct {
		name       string
		s          string
		start, end int64
		want       string
	}{
		{"runes", "a日本🙂z", 1, 4, "日本🙂"},
		{"combining rune", "e\u0301x", 1, 2, "\u0301"},
		{"empty string", "", 0, 4, ""},
		{"negative start", "a日本🙂z", -9, 2, "a日"},
		{"negative end", "a日本🙂z", 0, -1, ""},
		{"both negative", "a日本🙂z", -9, -1, ""},
		{"end past length", "a日本🙂z", 2, 100, "本🙂z"},
		{"both past length", "a日本🙂z", 10, 100, ""},
		{"reversed", "a日本🙂z", 4, 1, ""},
		{"equal", "a日本🙂z", 2, 2, ""},
		{"full range", "a日本🙂z", -1 << 63, 1<<63 - 1, "a日本🙂z"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := Substring(tt.s, tt.start, tt.end); got != tt.want {
				t.Errorf("Substring(%q, %d, %d) = %q, want %q", tt.s, tt.start, tt.end, got, tt.want)
			}
		})
	}
}

func TestParseInt(t *testing.T) {
	installTestShapes(t)
	for _, tt := range []struct {
		s    string
		want any
	}{
		{"0", testSome{int64(0)}},
		{"42", testSome{int64(42)}},
		{"+42", testSome{int64(42)}},
		{"-42", testSome{int64(-42)}},
		{"010", testSome{int64(10)}},
		{"9223372036854775807", testSome{int64(1<<63 - 1)}},
		{"-9223372036854775808", testSome{int64(-1 << 63)}},
		{"9223372036854775808", testNone{}},
		{"-9223372036854775809", testNone{}},
		{"", testNone{}},
		{" 42", testNone{}},
		{"42 ", testNone{}},
		{"1.5", testNone{}},
		{"0x10", testNone{}},
		{"1_000", testNone{}},
		{"42x", testNone{}},
	} {
		t.Run(tt.s, func(t *testing.T) {
			if got := ParseInt(tt.s); got != tt.want {
				t.Errorf("ParseInt(%q) = %#v, want %#v", tt.s, got, tt.want)
			}
		})
	}
}

func TestShowPadLeft(t *testing.T) {
	for _, tt := range []struct {
		n, width int64
		want     string
	}{
		{42, 5, "   42"},
		{-42, 5, "  -42"},
		{0, 3, "  0"},
		{42, 2, "42"},
		{42, 1, "42"},
		{42, -1, "42"},
		{42, -1 << 63, "42"},
		{-1 << 63, 22, "  -9223372036854775808"},
	} {
		if got := ShowPadLeft(tt.n, tt.width); got != tt.want {
			t.Errorf("ShowPadLeft(%d, %d) = %q, want %q", tt.n, tt.width, got, tt.want)
		}
	}
}

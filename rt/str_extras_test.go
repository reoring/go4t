package rt

import (
	"reflect"
	"testing"
)

func TestStringExtras(t *testing.T) {
	installTestShapes(t)
	for _, tt := range []struct {
		name string
		call func() any
		want any
	}{
		{"codePoint empty", func() any { return CodePoint("") }, testNone{}},
		{"codePoint first", func() any { return CodePoint("日a") }, testSome{int64('日')}},
		{"codePoint astral", func() any { return CodePoint("🙂") }, testSome{int64('🙂')}},
		{"codePoint malformed", func() any { return CodePoint("\xff") }, testSome{int64('\uFFFD')}},
		{"fromCodePoint zero", func() any { return FromCodePoint(int64(0)) }, "\x00"},
		{"fromCodePoint rune", func() any { return FromCodePoint(int64('🙂')) }, "🙂"},
		{"fromCodePoint negative", func() any { return FromCodePoint(int64(-1)) }, "�"},
		{"fromCodePoint surrogate", func() any { return FromCodePoint(int64(0xD800)) }, "�"},
		{"fromCodePoint last surrogate", func() any { return FromCodePoint(int64(0xDFFF)) }, "�"},
		{"fromCodePoint maximum", func() any { return FromCodePoint(int64(0x10FFFF)) }, "\U0010FFFF"},
		{"fromCodePoint too large", func() any { return FromCodePoint(int64(0x110000)) }, "�"},
		{"fromCodePoint no wrap", func() any { return FromCodePoint(int64(1<<32 + 'A')) }, "�"},
		{"fromCodePoint minimum", func() any { return FromCodePoint(int64(-1 << 63)) }, "�"},
		{"fromCodePoint int maximum", func() any { return FromCodePoint(int64(1<<63 - 1)) }, "�"},
		{"indexOf runes", func() any { return IndexOf("日🙂本🙂", "🙂") }, testSome{int64(1)}},
		{"indexOf substring", func() any { return IndexOf("日🙂本", "🙂本") }, testSome{int64(1)}},
		{"indexOf absent", func() any { return IndexOf("日🙂本", "x") }, testNone{}},
		{"indexOf empty", func() any { return IndexOf("", "x") }, testNone{}},
		{"indexOf empty needle", func() any { return IndexOf("日本", "") }, testSome{int64(0)}},
		{"indexOf both empty", func() any { return IndexOf("", "") }, testSome{int64(0)}},
		{"reverseString empty", func() any { return ReverseString("") }, ""},
		{"reverseString singleton", func() any { return ReverseString("🙂") }, "🙂"},
		{"reverseString runes", func() any { return ReverseString("日🙂e\u0301") }, "\u0301e🙂日"},
		{"splitOnce first", func() any { return SplitOnce("=", "日=a=b") }, testSome{testTuple2{"日", "a=b"}}},
		{"splitOnce leading", func() any { return SplitOnce("=", "=a") }, testSome{testTuple2{"", "a"}}},
		{"splitOnce trailing", func() any { return SplitOnce("=", "a=") }, testSome{testTuple2{"a", ""}}},
		{"splitOnce rune separator", func() any { return SplitOnce("日", "a日b") }, testSome{testTuple2{"a", "b"}}},
		{"splitOnce absent", func() any { return SplitOnce("=", "a") }, testNone{}},
		{"splitOnce empty", func() any { return SplitOnce("=", "") }, testNone{}},
		{"splitOnce empty separator", func() any { return SplitOnce("", "日本") }, testSome{testTuple2{"", "日本"}}},
		{"splitOnce both empty", func() any { return SplitOnce("", "") }, testSome{testTuple2{"", ""}}},
		{"charAt first", func() any { return CharAt("日🙂本", int64(0)) }, testSome{"日"}},
		{"charAt rune", func() any { return CharAt("日🙂本", int64(1)) }, testSome{"🙂"}},
		{"charAt last", func() any { return CharAt("日🙂本", int64(2)) }, testSome{"本"}},
		{"charAt empty", func() any { return CharAt("", int64(0)) }, testNone{}},
		{"charAt end", func() any { return CharAt("日🙂本", int64(3)) }, testNone{}},
		{"charAt negative", func() any { return CharAt("日🙂本", int64(-1)) }, testNone{}},
		{"charAt minimum", func() any { return CharAt("日🙂本", int64(-1<<63)) }, testNone{}},
		{"charAt maximum", func() any { return CharAt("日🙂本", int64(1<<63-1)) }, testNone{}},
		{"showQuoted empty", func() any { return ShowQuoted("") }, `""`},
		{"showQuoted escaped", func() any { return ShowQuoted("\"\n\t\\") }, `"\"\n\t\\"`},
		{"showQuoted unicode", func() any { return ShowQuoted("日本🙂") }, `"日本🙂"`},
		{"showQuoted control", func() any { return ShowQuoted("\x00") }, `"\x00"`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.call(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("result = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestUnicodePredicates(t *testing.T) {
	for _, tt := range []struct {
		name string
		call func(any) any
		yes  string
		no   string
	}{
		{"isDigit", IsDigit, "12٣９", "1a"},
		{"isAlpha", IsAlpha, "é日本Z", "a1"},
		{"isSpace", IsSpace, " \t\n\u2003", " a"},
		{"isUpper", IsUpper, "ÉΑZ", "Aa"},
		{"isLower", IsLower, "éαz", "aA"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, test := range []struct {
				input string
				want  bool
			}{{tt.yes, true}, {tt.no, false}, {"", false}, {"!", false}} {
				if got := tt.call(test.input); got != test.want {
					t.Errorf("%s(%q) = %v, want %v", tt.name, test.input, got, test.want)
				}
			}
		})
	}
}

func TestStringPadding(t *testing.T) {
	for _, tt := range []struct {
		s           string
		width       int64
		left, right string
	}{
		{"日🙂", 4, "  日🙂", "日🙂  "},
		{"日🙂", 2, "日🙂", "日🙂"},
		{"日🙂", 1, "日🙂", "日🙂"},
		{"日🙂", 0, "日🙂", "日🙂"},
		{"日🙂", -1, "日🙂", "日🙂"},
		{"日🙂", -1 << 63, "日🙂", "日🙂"},
		{"", 3, "   ", "   "},
		{"", 0, "", ""},
		{"", -1, "", ""},
		{"e\u0301", 3, " e\u0301", "e\u0301 "},
	} {
		if got := PadLeft(tt.s, tt.width); got != tt.left {
			t.Errorf("PadLeft(%q, %d) = %q, want %q", tt.s, tt.width, got, tt.left)
		}
		if got := PadRight(tt.s, tt.width); got != tt.right {
			t.Errorf("PadRight(%q, %d) = %q, want %q", tt.s, tt.width, got, tt.right)
		}
	}
}

func TestAdditionalExistingBoundaries(t *testing.T) {
	installTestShapes(t)
	for _, tt := range []struct {
		name string
		call func() any
		want any
	}{
		{"parseInt +5", func() any { return ParseInt("+5") }, testSome{int64(5)}},
		{"endsWith empty", func() any { return EndsWith("日本", "") }, true},
		{"contains empty", func() any { return Contains("日本", "") }, true},
		{"toUpper empty", func() any { return ToUpper("") }, ""},
		{"toLower empty", func() any { return ToLower("") }, ""},
		{"replace both empty", func() any { return Replace("", "", "-") }, "-"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.call(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("result = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestModIntByZeroPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("ModInt by zero did not panic")
		}
	}()
	ModInt(int64(1), int64(0))
}

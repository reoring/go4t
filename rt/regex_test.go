package rt

import (
	"reflect"
	"testing"
)

func TestRegexFunctions(t *testing.T) {
	installTestShapes(t)
	captures := func(pattern, text string) any {
		value := RegexCaptures(pattern, text)
		if some, ok := value.(testSome); ok {
			return testSome{ListToStrings(some.value)}
		}
		return value
	}
	for _, tt := range []struct {
		name string
		call func() any
		want any
	}{
		{"match searches", func() any { return RegexMatch("日+", "a日日b") }, true},
		{"match absent", func() any { return RegexMatch("^日+$", "a日日b") }, false},
		{"match empty", func() any { return RegexMatch("", "") }, true},
		{"match invalid", func() any { return RegexMatch("[", "abc") }, false},
		{"find first", func() any { return RegexFind("[0-9]+", "a12b34") }, testSome{"12"}},
		{"find absent", func() any { return RegexFind("[0-9]+", "abc") }, testNone{}},
		{"find empty match", func() any { return RegexFind("a*", "bbb") }, testSome{""}},
		{"find empty input", func() any { return RegexFind("^$", "") }, testSome{""}},
		{"find invalid", func() any { return RegexFind("[", "abc") }, testNone{}},
		{"findAll order", func() any { return ListToStrings(RegexFindAll("[0-9]+", "a12b34")) }, []string{"12", "34"}},
		{"findAll disjoint", func() any { return ListToStrings(RegexFindAll("aa", "aaa")) }, []string{"aa"}},
		{"findAll absent", func() any { return ListToStrings(RegexFindAll("x", "abc")) }, []string(nil)},
		{"findAll empty input", func() any { return ListToStrings(RegexFindAll("x", "")) }, []string(nil)},
		{"findAll empty pattern", func() any { return ListToStrings(RegexFindAll("", "日🙂")) }, []string{"", "", ""}},
		{"findAll both empty", func() any { return ListToStrings(RegexFindAll("", "")) }, []string{""}},
		{"findAll invalid", func() any { return ListToStrings(RegexFindAll("[", "abc")) }, []string(nil)},
		{"replace all", func() any { return RegexReplace("[0-9]+", "a12b34", "#") }, "a#b#"},
		{"replace captures", func() any { return RegexReplace("([a-z]+)=([0-9]+)", "a=1 b=22", "$2:$1") }, "1:a 22:b"},
		{"replace named capture", func() any { return RegexReplace("(?P<key>[a-z]+)", "a", "${key}!$$") }, "a!$"},
		{"replace missing capture", func() any { return RegexReplace("(a)", "a", "$2") }, ""},
		{"replace absent", func() any { return RegexReplace("x", "abc", "!") }, "abc"},
		{"replace empty pattern", func() any { return RegexReplace("", "日🙂", "-") }, "-日-🙂-"},
		{"replace empty input", func() any { return RegexReplace("", "", "-") }, "-"},
		{"replace invalid", func() any { return RegexReplace("[", "abc", "!") }, "abc"},
		{"split fields", func() any { return ListToStrings(RegexSplit("[,;]+", "a,,b;c")) }, []string{"a", "b", "c"}},
		{"split ends", func() any { return ListToStrings(RegexSplit(",", ",a,")) }, []string{"", "a", ""}},
		{"split absent", func() any { return ListToStrings(RegexSplit("x", "abc")) }, []string{"abc"}},
		{"split empty input", func() any { return ListToStrings(RegexSplit(",", "")) }, []string{""}},
		{"split empty pattern", func() any { return ListToStrings(RegexSplit("", "日🙂")) }, []string{"日", "🙂"}},
		{"split both empty", func() any { return ListToStrings(RegexSplit("", "")) }, []string(nil)},
		{"split invalid", func() any { return ListToStrings(RegexSplit("[", "abc")) }, []string(nil)},
		{"captures groups only", func() any { return captures("([A-Z]+) (.*)", "INFO 日🙂") }, testSome{[]string{"INFO", "日🙂"}}},
		{"captures first match", func() any { return captures("([a-z])([0-9])", "a1b2") }, testSome{[]string{"a", "1"}}},
		{"captures nested", func() any { return captures("((a)b)", "ab") }, testSome{[]string{"ab", "a"}}},
		{"captures unmatched optional", func() any { return captures("(a)?(b)", "b") }, testSome{[]string{"", "b"}}},
		{"captures no groups", func() any { return captures("abc", "abc") }, testSome{[]string(nil)}},
		{"captures empty", func() any { return captures("", "") }, testSome{[]string(nil)}},
		{"captures empty group", func() any { return captures("()", "") }, testSome{[]string{""}}},
		{"captures absent", func() any { return captures("(x)", "abc") }, testNone{}},
		{"captures invalid", func() any { return captures("[", "abc") }, testNone{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.call(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("result = %#v, want %#v", got, tt.want)
			}
		})
	}
}

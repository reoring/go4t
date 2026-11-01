package rt

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// StrLength returns the number of Unicode code points, not bytes.
func StrLength(s any) any { return int64(utf8.RuneCountInString(s.(string))) }

// StrConcat concatenates a generated List[String] without a separator.
func StrConcat(xs any) any { return Join("", xs) }

// Join concatenates a generated List[String], separating adjacent elements.
func Join(sep, xs any) any {
	separator := sep.(string)
	uncons := Shapes.Uncons
	if uncons == nil {
		panic("rt.Shapes is not initialized: Uncons is required")
	}
	head, tail, ok := uncons(xs)
	if !ok {
		return ""
	}
	first := head.(string)
	head, tail, ok = uncons(tail)
	if !ok {
		return first
	}
	var out strings.Builder
	out.WriteString(first)
	for ok {
		out.WriteString(separator)
		out.WriteString(head.(string))
		head, tail, ok = uncons(tail)
	}
	return out.String()
}

// Split follows strings.Split; an empty separator splits into one-rune strings.
func Split(sep, s any) any {
	return ListFromStrings(strings.Split(s.(string), sep.(string)))
}

// Lines splits on newline and drops exactly one trailing empty element.
func Lines(s any) any {
	lines := strings.Split(s.(string), "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return ListFromStrings(lines)
}

// Words splits on runs of Unicode whitespace, omitting empty fields.
func Words(s any) any { return ListFromStrings(strings.Fields(s.(string))) }

func Trim(s any) any               { return strings.TrimSpace(s.(string)) }
func ToUpper(s any) any            { return strings.ToUpper(s.(string)) }
func ToLower(s any) any            { return strings.ToLower(s.(string)) }
func StartsWith(s, prefix any) any { return strings.HasPrefix(s.(string), prefix.(string)) }
func EndsWith(s, suffix any) any   { return strings.HasSuffix(s.(string), suffix.(string)) }
func Contains(s, sub any) any      { return strings.Contains(s.(string), sub.(string)) }
func Replace(s, old, new any) any  { return strings.ReplaceAll(s.(string), old.(string), new.(string)) }

// Substring uses a half-open rune range clamped to the string's bounds.
// Reversed ranges are empty. Scan byte offsets to avoid copying into []rune.
func Substring(s, start, end any) any {
	text := s.(string)
	lo, hi := start.(int64), end.(int64)
	if lo < 0 {
		lo = 0
	}
	if hi <= lo {
		return ""
	}
	startByte, endByte := len(text), len(text)
	var runeIndex int64
	for byteIndex := range text {
		if runeIndex == lo {
			startByte = byteIndex
		}
		if runeIndex == hi {
			endByte = byteIndex
			break
		}
		runeIndex++
	}
	return text[startByte:endByte]
}

// Chars returns a generated list of one-rune strings.
func Chars(s any) any { return Split("", s) }

func Repeat(s, n any) any { return strings.Repeat(s.(string), int(n.(int64))) }

// ParseInt parses a signed base-10 Int, returning None for syntax or range errors.
func ParseInt(s any) any {
	value, err := strconv.ParseInt(s.(string), 10, 64)
	if err != nil {
		return NoneOf()
	}
	return SomeOf(value)
}

// ShowPadLeft right-aligns a decimal Int with spaces, without truncating it.
func ShowPadLeft(n, width any) any {
	text := strconv.FormatInt(n.(int64), 10)
	fieldWidth := width.(int64)
	if fieldWidth <= int64(len(text)) {
		return text
	}
	padding := fieldWidth - int64(len(text))
	return strings.Repeat(" ", int(padding)) + text
}

package rt

import (
	"go/token"
	"strconv"
	"unicode"
	"unicode/utf8"
)

// Rune helpers for the self-hosted lexer and code generator. Each takes a
// one-rune string (as produced by chars) except where noted.

// IsLetter reports unicode.IsLetter on the first rune of s.
func IsLetter(s any) any {
	r, _ := utf8.DecodeRuneInString(s.(string))
	return unicode.IsLetter(r)
}

// StartsDigit reports unicode.IsDigit on the first rune of s.
func StartsDigit(s any) any {
	r, _ := utf8.DecodeRuneInString(s.(string))
	return unicode.IsDigit(r)
}

// StartsSpace reports unicode.IsSpace on the first rune of s.
func StartsSpace(s any) any {
	r, _ := utf8.DecodeRuneInString(s.(string))
	return unicode.IsSpace(r)
}

// StartsLower reports unicode.IsLower on the first rune of s.
func StartsLower(s any) any {
	r, _ := utf8.DecodeRuneInString(s.(string))
	return unicode.IsLower(r)
}

// StartsUpper reports unicode.IsUpper on the first rune of s.
func StartsUpper(s any) any {
	r, _ := utf8.DecodeRuneInString(s.(string))
	return unicode.IsUpper(r)
}

// Unquote decodes a Go-syntax double-quoted literal (quotes included);
// returns Option[String] (None when invalid).
func Unquote(s any) any {
	value, err := strconv.Unquote(s.(string))
	if err != nil {
		return NoneOf()
	}
	return SomeOf(value)
}

// Quote renders s as a Go double-quoted literal (strconv.Quote / %q).
func Quote(s any) any { return strconv.Quote(s.(string)) }

// IsGoIdentifier reports token.IsIdentifier(s).
func IsGoIdentifier(s any) any { return token.IsIdentifier(s.(string)) }

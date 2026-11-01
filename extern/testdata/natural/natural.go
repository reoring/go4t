package natural

import (
	"errors"
	"gofx/rt"
)

func Echo(value any) any                                 { return value }
func Int(value int64) int64                              { return value }
func NativeInt(value int) int                            { return value }
func Float(value float64) float64                        { return value }
func String(value string) string                         { return value }
func Bool(value bool) bool                               { return value }
func Unit(value rt.Unit) rt.Unit                         { return value }
func IO(value rt.IO) rt.IO                               { return value }
func Strings(values []string) []string                   { return values }
func Ints(values []int64) []int64                        { return values }
func OptionInt(value int64, ok bool) (int64, bool)       { return value, ok }
func OptionNativeInt(value int, ok bool) (int, bool)     { return value, ok }
func OptionString(value string, ok bool) (string, bool)  { return value, ok }
func OptionBool(value bool, ok bool) (bool, bool)        { return value, ok }
func OptionFloat(value float64, ok bool) (float64, bool) { return value, ok }
func OptionUnit(value rt.Unit, ok bool) (rt.Unit, bool)  { return value, ok }
func Void()                                              {}

var Calls int64

func WithError(value string) (string, error) {
	Calls++
	if value == "bad" {
		return "", errors.New("fixture failure")
	}
	return value, nil
}
func IntsWithError(value []int64) ([]int64, error)      { return value, nil }
func StringsWithError(value []string) ([]string, error) { return value, nil }
func ErasedWithError(value any) (any, error)            { return value, nil }
func Mixed(value any) string                            { return value.(string) }
func Wrong(value int64) bool                            { return value != 0 }
func Variadic(values ...string) string                  { return "" }
func Generic[T any](value T) T                          { return value }

type NamedInt int64
type IntAlias = int64
type UnitAlias = rt.Unit

func Named(value NamedInt) NamedInt       { return value }
func Alias(value IntAlias) IntAlias       { return value }
func AliasUnit(value UnitAlias) UnitAlias { return value }
func Type() int64                         { return 1 }

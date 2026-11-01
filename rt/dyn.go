package rt

import (
	"fmt"
	"reflect"
	"time"
)

// DynToGo copies a std/dyn.Value into the corresponding ordinary Go value.
func DynToGo(value any) any {
	kind, v := Shapes.DynView(value)
	switch kind {
	case "null":
		return nil
	case "bool", "int", "float", "str":
		return v
	case "bytes":
		return []byte(v.(string))
	case "time":
		if v.(int64) == 0 {
			return time.Time{}
		}
		return time.Unix(0, v.(int64))
	case "list":
		return DynListToGo(v)
	case "map":
		m := map[string]any{}
		ForeignMapEach(v, func(k, v any) { m[k.(string)] = DynToGo(v) })
		return m
	default:
		panic("rt.Shapes.DynView: invalid Value")
	}
}

// DynFromGo decodes supported dynamic values. Unknown values are rendered as
// text, never retained as unchecked interface payloads inside the Value ADT.
func DynFromGo(value any) any {
	switch v := value.(type) {
	case nil:
		return Shapes.DynNull()
	case bool:
		return Shapes.DynBool(v)
	case string:
		return Shapes.DynStr(v)
	case []byte:
		return Shapes.DynBytes(string(v))
	case time.Time:
		if v.IsZero() {
			return Shapes.DynTime(int64(0))
		}
		return Shapes.DynTime(v.UnixNano())
	case []any:
		return Shapes.DynList(DynListFromGo(v))
	case map[string]any:
		entries := make([][2]any, 0, len(v))
		for k, x := range v {
			entries = append(entries, [2]any{k, DynFromGo(x)})
		}
		return Shapes.DynMap(ForeignMap(entries))
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return Shapes.DynInt(v.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return Shapes.DynInt(int64(v.Uint()))
	case reflect.Float32, reflect.Float64:
		return Shapes.DynFloat(v.Float())
	}
	return Shapes.DynStr(fmt.Sprint(value))
}
func DynListToGo(value any) []any {
	var result []any
	for {
		v, rest, ok := Shapes.Uncons(value)
		if !ok {
			return result
		}
		result = append(result, DynToGo(v))
		value = rest
	}
}
func DynListFromGo(values []any) any {
	result := Shapes.Nil()
	for i := len(values) - 1; i >= 0; i-- {
		result = Shapes.Cons(DynFromGo(values[i]), result)
	}
	return result
}

package rt

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// ParseJson consumes exactly one JSON value. Token decoding retains object order;
// UseNumber preserves int64 literals before falling back to Float conversion.
func ParseJson(source any) any {
	decoder := json.NewDecoder(strings.NewReader(source.(string)))
	decoder.UseNumber()
	value, err := readJSON(decoder)
	if err == nil {
		if _, trailing := decoder.Token(); trailing != io.EOF {
			if trailing == nil {
				err = fmt.Errorf("multiple values")
			} else {
				err = trailing
			}
		}
	}
	if err != nil {
		return ErrOf("invalid JSON: " + err.Error())
	}
	return OkOf(value)
}

func readJSON(decoder *json.Decoder) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	switch value := token.(type) {
	case nil:
		return Shapes.JNull(), nil
	case bool:
		return Shapes.JBool(value), nil
	case string:
		return Shapes.JStr(value), nil
	case json.Number:
		if !strings.ContainsAny(value.String(), ".eE") {
			if n, err := value.Int64(); err == nil {
				return Shapes.JInt(n), nil
			}
		}
		n, err := value.Float64()
		if err != nil {
			return nil, err
		}
		return Shapes.JNum(n), nil
	case json.Delim:
		switch value {
		case '[':
			var values []any
			for decoder.More() {
				child, err := readJSON(decoder)
				if err != nil {
					return nil, err
				}
				values = append(values, child)
			}
			if _, err := decoder.Token(); err != nil {
				return nil, err
			}
			list := Shapes.Nil()
			for i := len(values) - 1; i >= 0; i-- {
				list = Shapes.Cons(values[i], list)
			}
			return Shapes.JArr(list), nil
		case '{':
			var fields []jsonField
			positions := make(map[string]int)
			for decoder.More() {
				key, err := decoder.Token()
				if err != nil {
					return nil, err
				}
				child, err := readJSON(decoder)
				if err != nil {
					return nil, err
				}
				name := key.(string)
				if i, exists := positions[name]; exists {
					fields[i].value = child
				} else {
					positions[name] = len(fields)
					fields = append(fields, jsonField{name, child})
				}
			}
			if _, err := decoder.Token(); err != nil {
				return nil, err
			}
			return jsonObject(fields), nil
		}
	}
	return nil, fmt.Errorf("unexpected token %v", token)
}

type jsonField struct {
	name  string
	value any
}

// JsonObject is the ordered object constructor used by derived encoders. The
// payload remains an ordinary persistent Map, including its sorted traversals.
func JsonObject(entries any) any {
	var fields []jsonField
	for {
		pair, rest, ok := Shapes.Uncons(entries)
		if !ok {
			break
		}
		name, value := Shapes.Untuple2(pair)
		fields = append(fields, jsonField{name.(string), value})
		entries = rest
	}
	return jsonObject(fields)
}

func jsonObject(fields []jsonField) any {
	order := make([]string, len(fields))
	for i, field := range fields {
		order[i] = field.name
	}
	sort.Slice(fields, func(i, j int) bool { return fields[i].name < fields[j].name })
	var build func([]jsonField) *collNode
	build = func(fields []jsonField) *collNode {
		if len(fields) == 0 {
			return nil
		}
		mid := len(fields) / 2
		return collBranch(fields[mid].name, fields[mid].value, build(fields[:mid]), build(fields[mid+1:]))
	}
	return Shapes.JObj(&mapValue{root: build(fields), jsonOrder: &order})
}

func jsonLookup(root *collNode, key string) *collNode {
	for root != nil {
		switch strings.Compare(key, root.key.(string)) {
		case -1:
			root = root.left
		case 1:
			root = root.right
		default:
			return root
		}
	}
	return nil
}

func ToJson(value any) any {
	var out bytes.Buffer
	writeJSON(&out, value)
	return out.String()
}

func ToJsonPretty(value any) any {
	var compact, pretty bytes.Buffer
	writeJSON(&compact, value)
	if err := json.Indent(&pretty, compact.Bytes(), "", "  "); err != nil {
		panic(err)
	}
	return pretty.String()
}

func writeJSON(out *bytes.Buffer, j any) {
	kind, value := Shapes.JsonView(j)
	switch kind {
	case "null":
		out.WriteString("null")
	case "bool":
		out.WriteString(strconv.FormatBool(value.(bool)))
	case "integer":
		var digits [20]byte
		out.Write(strconv.AppendInt(digits[:0], value.(int64), 10))
	case "number":
		n := value.(float64)
		if math.IsNaN(n) || math.IsInf(n, 0) {
			panic("cannot encode non-finite JSON number")
		}
		// encoding/json chooses the JSON number spelling (including exponent bounds).
		data, _ := json.Marshal(n)
		out.Write(data)
	case "string":
		data, _ := json.Marshal(value.(string))
		out.Write(data)
	case "array":
		out.WriteByte('[')
		first := true
		for {
			item, rest, ok := Shapes.Uncons(value)
			if !ok {
				break
			}
			if !first {
				out.WriteByte(',')
			}
			first = false
			writeJSON(out, item)
			value = rest
		}
		out.WriteByte(']')
	case "object":
		object := value.(*mapValue)
		out.WriteByte('{')
		first := true
		field := func(n *collNode) {
			if !first {
				out.WriteByte(',')
			}
			first = false
			name, _ := json.Marshal(n.key.(string))
			out.Write(name)
			out.WriteByte(':')
			writeJSON(out, n.value)
		}
		if object.jsonOrder == nil {
			// Map's public By functions permit custom comparator order. JSON
			// objects still require lexicographic key order, independently of it.
			nodes := make([]*collNode, 0, collSize(object.root))
			collWalk(object.root, func(n *collNode) { nodes = append(nodes, n) })
			sort.Slice(nodes, func(i, j int) bool { return nodes[i].key.(string) < nodes[j].key.(string) })
			for _, n := range nodes {
				field(n)
			}
		} else {
			for _, name := range *object.jsonOrder {
				field(jsonLookup(object.root, name))
			}
		}
		out.WriteByte('}')
	default:
		panic("rt.Shapes.JsonView: invalid Json kind")
	}
}

// JsonInt accepts integral Floats only within the binary64 exact-integer range.
// Larger integer literals use JInt directly, without a Float conversion.
func JsonInt(value any) any {
	n := value.(float64)
	if math.IsNaN(n) || n < -9007199254740992.0 || n > 9007199254740992.0 || math.Trunc(n) != n {
		return ErrOf("expected Int at $")
	}
	return OkOf(int64(n))
}

func JsonErrorAt(message, segment any) any {
	text := message.(string)
	if i := strings.Index(text, " at $"); i >= 0 {
		return text[:i+5] + segment.(string) + text[i+5:]
	}
	return text + " at $" + segment.(string)
}

func JsonFieldPath(name any) any {
	key := name.(string)
	identifier := key != ""
	for i, r := range key {
		if r != '_' && !unicode.IsLetter(r) && (i == 0 || !unicode.IsDigit(r)) {
			identifier = false
			break
		}
	}
	if identifier {
		return "." + key
	}
	encoded, _ := json.Marshal(key)
	return "[" + string(encoded) + "]"
}

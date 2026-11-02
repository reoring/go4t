package laws

import (
	"sync"

	"go4t/rt"
	"go4t/types"
)

// The runtime's generated-shape ABI is process-global. Serialize the short
// constant-folding call and restore the caller's shape table, including on panic.
// No symbolic argument crosses this ABI.
var stringRuntimeMu sync.Mutex

func (e *evaluator) trustedExtern(f *types.FuncInfo) bool {
	return e.trustedModules[e.p.Owners[f.Name]] && primitive(f.Decl.Extern)
}

func stringPrimitive(name string) bool {
	switch name {
	case "StrLength", "StrConcat", "Join", "Split", "Lines", "Words", "Trim", "ToUpper", "ToLower", "StartsWith", "EndsWith", "Contains", "Replace", "Substring", "Chars", "Repeat", "ParseInt", "ShowPadLeft", "CodePoint", "FromCodePoint", "IsDigit", "IsAlpha", "IsSpace", "IsUpper", "IsLower", "IndexOf", "PadRight", "PadLeft", "ReverseString", "SplitOnce", "CharAt", "ShowQuoted":
		return true
	}
	return false
}

func constantTerm(value *term) bool {
	switch value.kind {
	case "int", "bool", "string", "unit":
		return true
	case "ctor", "tuple":
		for _, arg := range value.args {
			if !constantTerm(arg) { return false }
		}
		return true
	}
	return false
}

func runtimeScalar(value *term) any {
	switch value.kind {
	case "string": return value.name
	case "int": return value.number
	case "bool": return value.flag
	default: return value
	}
}

func normalScalar(value any) *term {
	switch value := value.(type) {
	case string: return text(value)
	case int64: return integer(value)
	case bool: return boolean(value)
	default: return value.(*term)
	}
}

func (e *evaluator) stringExtern(name string, args []*term) *term {
	switch name {
	case "StrConcat", "Join", "Split", "Lines", "Words", "Chars", "ParseInt", "CodePoint", "IndexOf", "SplitOnce", "CharAt":
		stringRuntimeMu.Lock()
		defer stringRuntimeMu.Unlock()
		previous := rt.Shapes
		defer func() { rt.Shapes = previous }()
		rt.Shapes.Cons = func(h, t any) any { return &term{kind: "ctor", ctor: e.ctors["Cons"], args: []*term{normalScalar(h), normalScalar(t)}} }
		rt.Shapes.Nil = func() any { return &term{kind: "ctor", ctor: e.ctors["Nil"]} }
		rt.Shapes.Uncons = func(value any) (any, any, bool) {
			list := value.(*term)
			if list.ctor == e.ctors["Nil"] { return nil, nil, false }
			return runtimeScalar(list.args[0]), list.args[1], true
		}
		rt.Shapes.Some = func(value any) any { return &term{kind: "ctor", ctor: e.ctors["Some"], args: []*term{normalScalar(value)}} }
		rt.Shapes.None = func() any { return &term{kind: "ctor", ctor: e.ctors["None"]} }
		rt.Shapes.Tuple2 = func(a, b any) any { return &term{kind: "tuple", args: []*term{normalScalar(a), normalScalar(b)}} }
	}
	var a, b, c any
	if len(args) > 0 { a = runtimeScalar(args[0]) }
	if len(args) > 1 { b = runtimeScalar(args[1]) }
	if len(args) > 2 { c = runtimeScalar(args[2]) }
	var result any
	switch name {
	case "StrLength": result = rt.StrLength(a)
	case "StrConcat": result = rt.StrConcat(a)
	case "Join": result = rt.Join(a, b)
	case "Split": result = rt.Split(a, b)
	case "Lines": result = rt.Lines(a)
	case "Words": result = rt.Words(a)
	case "Trim": result = rt.Trim(a)
	case "ToUpper": result = rt.ToUpper(a)
	case "ToLower": result = rt.ToLower(a)
	case "StartsWith": result = rt.StartsWith(a, b)
	case "EndsWith": result = rt.EndsWith(a, b)
	case "Contains": result = rt.Contains(a, b)
	case "Replace": result = rt.Replace(a, b, c)
	case "Substring": result = rt.Substring(a, b, c)
	case "Chars": result = rt.Chars(a)
	case "Repeat": result = rt.Repeat(a, b)
	case "ParseInt": result = rt.ParseInt(a)
	case "ShowPadLeft": result = rt.ShowPadLeft(a, b)
	case "CodePoint": result = rt.CodePoint(a)
	case "FromCodePoint": result = rt.FromCodePoint(a)
	case "IsDigit": result = rt.IsDigit(a)
	case "IsAlpha": result = rt.IsAlpha(a)
	case "IsSpace": result = rt.IsSpace(a)
	case "IsUpper": result = rt.IsUpper(a)
	case "IsLower": result = rt.IsLower(a)
	case "IndexOf": result = rt.IndexOf(a, b)
	case "PadRight": result = rt.PadRight(a, b)
	case "PadLeft": result = rt.PadLeft(a, b)
	case "ReverseString": result = rt.ReverseString(a)
	case "SplitOnce": result = rt.SplitOnce(a, b)
	case "CharAt": result = rt.CharAt(a, b)
	case "ShowQuoted": result = rt.ShowQuoted(a)
	}
	return normalScalar(result)
}

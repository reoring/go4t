package extern

import (
	"fmt"
	"go/types"

	gfx "go4t/types"
)

// Kind identifies the representation conversion required at the foreign boundary.
type Kind uint8

const (
	Erased Kind = iota
	Scalar
	Strings
	Ints
	Optional
)

// Value describes one go4t value. An Optional consumes two Go values; Go is the
// scalar Go spelling (including rt.Unit and rt.IO), or the optional payload's.
type Value struct {
	Kind Kind
	Go   string
}

// Adapter is a checked conversion plan. Uniform preserves the legacy any ABI;
// Error delays the Go call inside rt.IO and propagates its error without boxing.
type Adapter struct {
	Uniform bool
	Error   bool
	Params  []Value
	Result  Value
}

// Match selects the ABI for a Go function and a fully resolved go4t declaration.
// A natural signature must match every argument and result; mixed accidental
// any/natural signatures do not silently erase declared primitive types.
func Match(f Function, declared *gfx.TFunc) (Adapter, error) {
	sig := f.Signature
	mismatch := func() (Adapter, error) {
		return Adapter{}, mismatchError{function: f, declared: declared}
	}
	if sig.Variadic() || sig.TypeParams().Len() != 0 {
		return mismatch()
	}
	if sig.Params().Len() == len(declared.Params) && sig.Results().Len() == 1 {
		uniform := true
		for i := range declared.Params {
			uniform = uniform && isAny(sig.Params().At(i).Type())
		}
		result := sig.Results().At(0).Type()
		if uniform && (isAny(result) || (application(declared.Ret, "IO") != nil && named(result, "go4t/rt", "IO"))) {
			return Adapter{Uniform: true}, nil
		}
	}
	adapter := Adapter{Params: make([]Value, len(declared.Params))}
	offset := 0
	for i, param := range declared.Params {
		value, count, ok := matchValue(param, sig.Params(), offset)
		if !ok {
			return mismatch()
		}
		adapter.Params[i] = value
		offset += count
	}
	if offset != sig.Params().Len() {
		return mismatch()
	}
	resultType := declared.Ret
	results := sig.Results()
	if inner := application(resultType, "IO"); inner != nil && results.Len() == 2 && types.Identical(results.At(1).Type(), types.Universe.Lookup("error").Type()) {
		adapter.Error = true
		resultType = inner
		results = types.NewTuple(results.At(0))
	}
	value, count, ok := matchValue(resultType, results, 0)
	if !ok || count != results.Len() {
		return mismatch()
	}
	adapter.Result = value
	return adapter, nil
}

// Keep the declared type available until the CLI selects full-type rendering.
type mismatchError struct {
	function Function
	declared *gfx.TFunc
}

func (e mismatchError) Error() string { return e.Message(false) }

func (e mismatchError) Message(fullTypes bool) string {
	var declared string
	if fullTypes {
		declared = gfx.TypeStringFull(e.declared)
	} else {
		declared = gfx.TypeString(e.declared)
	}
	return fmt.Sprintf("extern %s: Go signature %s does not match declared type %s",
		e.function.Name, SignatureString(e.function.Signature), declared)
}

func application(t gfx.Type, name string) gfx.Type {
	if app, ok := gfx.Unalias(t).(*gfx.TApp); ok && len(app.Args) == 1 {
		if con, ok := gfx.Unalias(app.Fun).(*gfx.TCon); ok && con.Name == name {
			return app.Args[0]
		}
	}
	return nil
}

func matchValue(t gfx.Type, tuple *types.Tuple, offset int) (Value, int, bool) {
	if offset >= tuple.Len() {
		return Value{}, 0, false
	}
	goType := tuple.At(offset).Type()
	if inner := application(t, "Option"); inner != nil && primitive(inner) {
		if offset+1 >= tuple.Len() || !types.Identical(tuple.At(offset+1).Type(), types.Typ[types.Bool]) {
			return Value{}, 0, false
		}
		value, _, ok := matchValue(inner, tuple, offset)
		value.Kind = Optional
		return value, 2, ok
	}
	if inner := application(t, "List"); inner != nil {
		if con, ok := gfx.Unalias(inner).(*gfx.TCon); ok {
			switch con.Name {
			case "String":
				return Value{Kind: Strings}, 1, types.Identical(goType, types.NewSlice(types.Typ[types.String]))
			case "Int":
				return Value{Kind: Ints}, 1, types.Identical(goType, types.NewSlice(types.Typ[types.Int64]))
			}
		}
	}
	if application(t, "IO") != nil {
		return Value{Kind: Scalar, Go: "rt.IO"}, 1, named(goType, "go4t/rt", "IO")
	}
	if con, ok := gfx.Unalias(t).(*gfx.TCon); ok {
		var basic types.BasicKind
		switch con.Name {
		case "Int":
			if types.Identical(goType, types.Typ[types.Int]) {
				return Value{Kind: Scalar, Go: "int"}, 1, true
			}
			basic = types.Int64
		case "Float":
			basic = types.Float64
		case "String":
			basic = types.String
		case "Bool":
			basic = types.Bool
		case "Unit":
			return Value{Kind: Scalar, Go: "rt.Unit"}, 1, named(goType, "go4t/rt", "Unit")
		}
		if basic != types.Invalid {
			return Value{Kind: Scalar, Go: types.Typ[basic].Name()}, 1, types.Identical(goType, types.Typ[basic])
		}
	}
	return Value{Kind: Erased}, 1, isAny(goType)
}

func primitive(t gfx.Type) bool {
	con, ok := gfx.Unalias(t).(*gfx.TCon)
	if !ok {
		return false
	}
	switch con.Name {
	case "Int", "Float", "String", "Bool", "Unit":
		return true
	}
	return false
}

func named(t types.Type, path, name string) bool {
	// Named identities are supplied by the single package loader. Compare their
	// package paths, not their local import aliases.
	n, ok := types.Unalias(t).(*types.Named)
	if !ok {
		return false
	}
	return n.Obj().Pkg() != nil && n.Obj().Pkg().Path() == path && n.Obj().Name() == name
}

func isAny(t types.Type) bool {
	return types.Identical(t, types.Universe.Lookup("any").Type())
}

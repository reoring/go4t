package types

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Unalias removes presentation wrappers at the root. Consumers inspecting a
// type's shape must use this rather than making aliases a new semantic case.
func Unalias(t Type) Type {
	for {
		a, ok := t.(*TAlias)
		if !ok {
			return t
		}
		t = a.Body
	}
}

// TypeString preserves source-written aliases. Types longer than 160 runes
// replace subtrees at depth 3 with an ellipsis; the root has depth 0. Shallow
// types are never clipped in the middle of a name or a parameter list.
func TypeString(t Type) string {
	return printType(t, false, nameContext{})
}

func printType(t Type, full bool, names nameContext) string {
	budget := 160
	if full {
		budget = -1
	}
	p := typePrinter{remaining: budget, names: names}
	p.typ(t, 0)
	if !p.overflow {
		return p.out.String()
	}
	p = typePrinter{remaining: -1, elide: true, names: names}
	p.typ(t, 0)
	return p.out.String()
}

// TypeStringFull uses the same alias spelling without depth elision.
func TypeStringFull(t Type) string {
	return printType(t, true, nameContext{})
}

type typePrinter struct {
	out       strings.Builder
	remaining int // -1 means unbounded
	elide     bool
	overflow  bool
	names     nameContext
}

func (p *typePrinter) text(s string) {
	if p.overflow {
		return
	}
	if p.remaining >= 0 {
		n := utf8.RuneCountInString(s)
		if n > p.remaining {
			p.overflow = true
			return
		}
		p.remaining -= n
	}
	p.out.WriteString(s)
}

func (p *typePrinter) list(ts []Type, depth int) {
	for i, t := range ts {
		if p.overflow {
			return
		}
		if i != 0 {
			p.text(", ")
		}
		p.typ(t, depth)
	}
}

func (p *typePrinter) typ(t Type, depth int) {
	if p.overflow {
		return
	}
	if p.elide && depth >= 3 {
		p.text("…")
		return
	}
	switch t := t.(type) {
	case *TCon:
		p.text(p.names.name(t.Name, t.Display))
	case *TVar:
		p.text(t.Name)
	case *TMeta:
		if t.Ref != nil {
			p.typ(*t.Ref, depth)
		} else {
			p.text("?")
			p.text(strconv.Itoa(t.ID))
		}
	case *TAlias:
		p.text(p.names.name(t.Name, t.Display))
		if len(t.Args) != 0 {
			p.text("[")
			p.list(t.Args, depth+1)
			p.text("]")
		}
	case *TApp:
		p.typ(t.Fun, depth)
		p.text("[")
		p.list(t.Args, depth+1)
		p.text("]")
	case *TFunc:
		p.text("func(")
		p.list(t.Params, depth+1)
		p.text(") ")
		p.typ(t.Ret, depth+1)
	default:
		p.text("?")
	}
}

// Retain structured diagnostic arguments until failure, including nested
// instance explanations. This keeps --full-types per invocation, not global,
// and avoids parsing or truncating arbitrary diagnostic text after rendering.
// Only error paths allocate messages; successful inference does not print types.
type diagnosticMessage struct {
	format string
	args   []any
}

func messagef(format string, args ...any) *diagnosticMessage {
	return &diagnosticMessage{format: format, args: args}
}

func (m *diagnosticMessage) render(full bool, names nameContext) string {
	if m == nil {
		return ""
	}
	args := make([]any, len(m.args))
	for i, arg := range m.args {
		switch arg := arg.(type) {
		case Type:
			args[i] = printType(arg, full, names)
		case diagnosticName:
			args[i] = names.name(string(arg), "")
		case *diagnosticMessage:
			args[i] = arg.render(full, names)
		case *Error:
			if arg.detail != nil {
				args[i] = arg.detail.render(full, names)
			} else {
				args[i] = arg.Msg
			}
		case interface{ Message(bool) string }:
			args[i] = arg.Message(full)
		default:
			args[i] = arg
		}
	}
	return fmt.Sprintf(m.format, args...)
}

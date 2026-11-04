package extern

import (
	"fmt"
	"go/types"
	"sort"
	"strings"
)

func (m *foreignMirror) hasChannel() bool {
	if m == nil {
		return false
	}
	if m.kind == "chan" || m.elem.hasChannel() || m.key.hasChannel() {
		return true
	}
	for _, elem := range m.elems {
		if elem.hasChannel() {
			return true
		}
	}
	if m.callback != nil {
		return m.callback.hasChannel()
	}
	return false
}

func (c *foreignCall) hasChannel() bool {
	for _, param := range c.params {
		if param.hasChannel() {
			return true
		}
	}
	return c.result.hasChannel()
}

func beginShimIO(out *strings.Builder, channelBoundary bool) {
	if channelBoundary {
		out.WriteString("return rt.IO(func(ctx context.Context) (go4tValue any, go4tErr error) {\ndefer rt.RecoverChanFailure(&go4tValue,&go4tErr)\n")
	} else {
		out.WriteString("return rt.IO(func(ctx context.Context) (any,error) {\n")
	}
	out.WriteString("if err := ctx.Err(); err != nil { return nil,rt.ClassifyError(err) }\n")
}

// A channel's element mirror is not invertible (int and time.Time both map to
// Int). Typed constructors retain the exact native element without bridges.
func (g *shimGenerator) channelConstructors(decl *strings.Builder, set *mirrorSet, options BindOptions, candidates *bindingCandidates) {
	keys := make([]string, 0, len(set.channels))
	counts := map[string]int{}
	for key, elem := range set.channels {
		keys = append(keys, key)
		counts[channelElementName(elem.typ)]++
	}
	sort.Strings(keys)
	used := map[string]bool{}
	for _, key := range keys {
		elem := set.channels[key]
		suffix := channelElementName(elem.typ)
		if counts[suffix] > 1 {
			suffix = upcastOwnerSuffix(key, true)
		}
		base := "New" + suffix + "Chan"
		goName := base
		for i := 2; used[goName]; i++ {
			goName = fmt.Sprintf("%s%d", base, i)
		}
		used[goName] = true
		name, reason := bindingName(goName, options.Prefix)
		if reason != "" {
			fmt.Fprintf(decl, "// skipped: %s (%s)\n", goName, reason)
			continue
		}
		candidates.add(name, goName, bindingValue, func() {
			shimName := "Go4t_" + goName
			fmt.Fprintf(decl, "extern func %s(buffer Int) IO[Chan[%s]] = %q.%s\n", name, elem.gt, g.path, shimName)
			conversion := g.conversion(elem)
			fmt.Fprintf(&g.body, "func %s(buffer any) any { return rt.NewGoChan(buffer,%sFrom,%sTo) }\n", shimName, conversion, conversion)
		})
	}
}

func channelElementName(t types.Type) string {
	t = types.Unalias(t)
	switch t := t.(type) {
	case *types.Named:
		return t.Obj().Name()
	case *types.Pointer:
		return "Ptr" + channelElementName(t.Elem())
	case *types.Slice:
		return "Slice" + channelElementName(t.Elem())
	case *types.Map:
		return "Map" + channelElementName(t.Key()) + channelElementName(t.Elem())
	case *types.Chan:
		prefix := "Chan"
		if t.Dir() == types.RecvOnly {
			prefix = "RecvChan"
		} else if t.Dir() == types.SendOnly {
			prefix = "SendChan"
		}
		return prefix + channelElementName(t.Elem())
	case *types.Struct:
		if t.NumFields() == 0 {
			return "Unit"
		}
	}
	return upcastOwnerSuffix(types.TypeString(t, nil), true)
}

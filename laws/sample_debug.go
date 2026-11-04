package laws

import (
	"fmt"
	"strconv"
	"strings"

	"go4t/types"
)

// Observers run only for a failed comparison. They prefer the program's Debug
// dictionary and otherwise inspect the generated value structurally, just like
// the open-term oracle. Depth follows the generator, bounding even non-regular
// recursive type instantiations without constructing a second copy of a value.
func (g *sampleGenerator) observer(typ types.Type, depth int) string {
	if types.SampleHasDebug(g.p, typ) {
		return g.reference("debug")
	}
	if g.observers == nil {
		g.observers = make(map[string]string)
	}
	key := fmt.Sprintf("%s/%d", types.TypeString(typ), depth)
	if name := g.observers[key]; name != "" {
		return name
	}
	name := fmt.Sprintf("%sobserve%d", g.prefix, len(g.observers))
	g.observers[key] = name
	con, args := typeHead(typ)
	var body string
	if _, tuple := types.IsTuple(con); tuple {
		parts := []string{strconv.Quote("(")}
		for i, field := range args {
			if i != 0 {
				parts = append(parts, strconv.Quote(", "))
			}
			parts = append(parts, fmt.Sprintf("%s(value.%d)", g.observer(field, depth), i))
		}
		parts = append(parts, strconv.Quote(")"))
		body = strings.Join(parts, " + ")
	} else if con.Name == "Map" {
		fold := ""
		for function := range g.p.Funcs {
			if sourceName(g.p, function) == "mapFold" && g.p.Owners[function] == "std/map" {
				fold = g.reference(function)
				break
			}
		}
		body = fmt.Sprintf("\"{\" + %s(value, \"\", func(acc, key, item) => acc + (if acc == \"\" { \"\" } else { \", \" }) + %s(key) + \": \" + %s(item)) + \"}\"", fold, g.observer(args[0], depth), g.observer(args[1], depth))
	} else {
		data := g.p.DataTypes[con.Name]
		var arms []string
		omitted := false
		for _, ctor := range data.Ctors {
			fields := sampleFields(data, args, ctor)
			possible := true
			for _, field := range fields {
				possible = possible && sampleInhabited(g.p, field, depth-1)
			}
			if !possible {
				omitted = true
				continue
			}
			names := make([]string, len(fields))
			parts := []string{strconv.Quote(sourceName(g.p, ctor.Name) + "(")}
			for i, field := range fields {
				names[i] = fmt.Sprintf("field%d", i)
				if i != 0 {
					parts = append(parts, strconv.Quote(", "))
				}
				parts = append(parts, g.observer(field, depth-1)+"("+names[i]+")")
			}
			pattern := g.reference(ctor.Name)
			if len(fields) == 0 {
				parts = []string{strconv.Quote(sourceName(g.p, ctor.Name))}
			} else {
				pattern += "(" + strings.Join(names, ", ") + ")"
				parts = append(parts, strconv.Quote(")"))
			}
			arms = append(arms, pattern+" => "+strings.Join(parts, " + "))
		}
		if omitted {
			arms = append(arms, "_ => "+g.prefix+"impossible()")
		}
		body = "match value { " + strings.Join(arms, "; ") + " }"
	}
	fmt.Fprintf(&g.functions, "func %s(value %s) String { %s }\n", name, g.typeSource(typ), body)
	return name
}

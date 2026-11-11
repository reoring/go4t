package laws

import (
	"fmt"
	"strings"

	"github.com/reoring/go4t/ast"
	"github.com/reoring/go4t/types"
)

const sampleDepth = 4

func sampleFields(data *types.DataType, args []types.Type, ctor *types.Ctor) []types.Type {
	sub := make(map[*types.TVar]types.Type, len(args))
	for i, arg := range args {
		sub[data.TParams[i]] = arg
	}
	fields := make([]types.Type, len(ctor.Fields))
	for i, field := range ctor.Fields {
		fields[i] = substituteType(field, sub)
	}
	return fields
}

// Like the open-term property harness, recursive constructors are selectable
// only when all their fields can be inhabited within the remaining depth.
func sampleInhabited(p *types.Program, typ types.Type, depth int) bool {
	con, args := typeHead(typ)
	if con == nil {
		return false
	}
	switch con.Name {
	case "Int", "Float", "Bool", "String", "Unit":
		return true
	case "Map":
		return sampleInhabited(p, args[0], depth) && sampleInhabited(p, args[1], depth)
	}
	if _, tuple := types.IsTuple(con); tuple {
		for _, arg := range args {
			if !sampleInhabited(p, arg, depth) {
				return false
			}
		}
		return true
	}
	if depth < 0 {
		return false
	}
	data := p.DataTypes[con.Name]
	if data == nil {
		return false
	}
	for _, ctor := range data.Ctors {
		possible := true
		for _, field := range sampleFields(data, args, ctor) {
			possible = possible && sampleInhabited(p, field, depth-1)
		}
		if possible {
			return true
		}
	}
	return false
}

func checkSample(p *types.Program, law *types.LawInfo) error {
	if len(law.TParams) != 0 {
		return fmt.Errorf("sample does not support type parameters")
	}
	tactic := law.Proof.Decl.Body.(*ast.TSample)
	if tactic.Count < 1 || tactic.Count > 100000 {
		return fmt.Errorf("sample count must be an Int literal in 1..100000")
	}
	for i, typ := range law.ParamTypes {
		if !sampleInhabited(p, typ, sampleDepth) {
			return fmt.Errorf("sample: parameter %s has no usable generator for %s within depth %d", law.Decl.Params[i].Name, types.TypeString(typ), sampleDepth)
		}
		if err := sampleAccess(p, typ, p.Owners[law.Decl.Name], sampleDepth); err != nil {
			return fmt.Errorf("sample: parameter %s has no usable generator: %w", law.Decl.Params[i].Name, err)
		}
	}
	err := newEvaluator(p).checkEligibility(law, law.Proof, false)
	if err == nil {
		return fmt.Errorf("sample is only for laws with ineligible dependencies; prove it instead")
	}
	if strings.Contains(err.Error(), "step limit") || strings.Contains(err.Error(), "fuel") {
		return err
	}
	return nil
}

func sampleMapFromList(p *types.Program) string {
	for name := range p.Funcs {
		if sourceName(p, name) == "mapFromList" && p.Owners[name] == "std/map" {
			return name
		}
	}
	return ""
}

// Generated source obeys ordinary module privacy, including private field types.
func sampleAccess(p *types.Program, typ types.Type, owner string, depth int) error {
	con, args := typeHead(typ)
	for _, arg := range args {
		if err := sampleAccess(p, arg, owner, depth); err != nil {
			return err
		}
	}
	if con.Name == "Map" && sampleMapFromList(p) == "" {
		return fmt.Errorf("Map generation requires std/map.mapFromList")
	}
	if p.Owners[con.Name] != owner && strings.HasPrefix(sourceName(p, con.Name), "_") {
		return fmt.Errorf("type %s is private to another module", sourceName(p, con.Name))
	}
	data := p.DataTypes[con.Name]
	if data == nil || depth < 0 {
		return nil
	}
	for _, ctor := range data.Ctors {
		fields := sampleFields(data, args, ctor)
		possible := true
		for _, field := range fields {
			possible = possible && sampleInhabited(p, field, depth-1)
		}
		if !possible {
			continue
		}
		if p.Owners[ctor.Name] != owner && strings.HasPrefix(sourceName(p, ctor.Name), "_") {
			return fmt.Errorf("constructor %s is private to another module", sourceName(p, ctor.Name))
		}
		for _, field := range fields {
			if err := sampleAccess(p, field, owner, depth-1); err != nil {
				return err
			}
		}
	}
	return nil
}

// SampleNotices reports accepted sampled laws without claiming proof evidence.
// Each entry is a single positioned line, deliberately without a source excerpt.
func SampleNotices(p *types.Program) []string {
	var notices []string
	for _, file := range p.Files {
		for _, decl := range file.Decls {
			law, ok := decl.(*ast.LawDecl)
			if !ok {
				continue
			}
			info := p.Laws[law.Name]
			if tactic, ok := info.Proof.Decl.Body.(*ast.TSample); ok && checkSample(p, info) == nil {
				notices = append(notices, fmt.Sprintf("%s:%s: law %s: tested by go4t test (sample %d), not proved", file.Name, law.Pos, sourceName(p, law.Name), tactic.Count))
			}
		}
	}
	return notices
}

// HasSamples reports whether test generation needs a preliminary type check.
func HasSamples(modules []*ast.Module) bool {
	for _, module := range modules {
		for _, file := range module.Files {
			for _, decl := range file.Decls {
				if proof, ok := decl.(*ast.ProofDecl); ok {
					if _, ok := proof.Body.(*ast.TSample); ok {
						return true
					}
				}
			}
		}
	}
	return false
}

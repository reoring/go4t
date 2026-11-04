package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go4t/ast"
	"go4t/extern"
)

// Each tuple has its own companion package. Reordering or repeating calls never
// changes package templates or another tuple's immutable companion cache entry.
type genericGoBindings struct {
	binder *goImportBinder
	owner string
	shimPath string
	baseEntry string
}

func (g *genericGoBindings) Instantiate(symbol string, signature *ast.FuncType) (*ast.ExternRef, error) {
	pkg, err := g.binder.loader.Load(g.owner)
	if err != nil { return nil, err }
	instance, err := extern.InstantiateGeneric(&g.binder.loader, pkg, symbol, signature)
	if err != nil { return nil, err }
	// Owner export bytes include dependency fingerprints. Exact argument owners
	// may be unrelated packages, so include their current export bytes too.
	hash := sha256.New()
	fmt.Fprint(hash, instance.Key)
	var fingerprint func(ast.Type) error
	fingerprint = func(t ast.Type) error {
		switch t := t.(type) {
		case *ast.TypeName:
			if owner := foreignTypeOwner(t.Name); owner != "" {
				if _, err := g.binder.loader.Load(owner); err != nil { return err }
				sum, err := g.binder.loader.ExportHash(owner)
				if err != nil { return err }
				fmt.Fprintf(hash, "%s:%x", owner, sum)
			}
			for _, a := range t.Args { if err := fingerprint(a); err != nil { return err } }
		case *ast.FuncType:
			for _, a := range t.Params { if err := fingerprint(a); err != nil { return err } }
			return fingerprint(t.Ret)
		case *ast.TupleType:
			for _, a := range t.Elems { if err := fingerprint(a); err != nil { return err } }
		}
		return nil
	}
	if err := fingerprint(signature); err != nil { return nil, err }
	key := fmt.Sprintf("%x", hash.Sum(nil))
	shimPath := g.shimPath + "/instances/i" + key
	ref := &ast.ExternRef{ImportPath: shimPath, Name: symbol}
	if g.binder.outputDir == "" { return ref, nil }
	cacheFile := ""
	var source []byte
	if g.baseEntry != "" {
		cacheFile = filepath.Join(g.baseEntry, "instances", key+".go")
		source, _ = os.ReadFile(cacheFile)
	}
	if source == nil {
		source, err = instance.Shim(shimPath)
		if err != nil { return nil, err }
		if cacheFile != "" {
			if err := os.MkdirAll(filepath.Dir(cacheFile), 0o755); err == nil {
				if err := writeBindingCacheFile(cacheFile, source); err != nil { traceBindingCache(g.owner, "instance write failed", err) }
			}
		}
	}
	dir := filepath.Join(g.binder.outputDir, "go4tbind", filepath.FromSlash(g.owner), "instances", "i"+key)
	if err := os.MkdirAll(dir, 0o755); err != nil { return nil, err }
	if err := os.WriteFile(filepath.Join(dir, "shim.go"), source, 0o644); err != nil { return nil, err }
	return ref, nil
}

func foreignTypeOwner(name string) string {
	if rest, ok := strings.CutPrefix(name, "go:"); ok {
		if dot := strings.LastIndexByte(rest, '.'); dot > 0 { return rest[:dot] }
	}
	return ""
}

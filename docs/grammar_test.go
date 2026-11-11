package docs_test

import (
	"fmt"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unicode"

	"github.com/reoring/go4t/lexer"
	"github.com/reoring/go4t/parser"
)

// This reader deliberately handles the notation in SPEC rather than treating
// grammar-looking lines as opaque strings. Lexical special sequences (? ... ?)
// keep identifier/literal token classes separate from quoted fixed terminals.
type grammarNode struct {
	kind     byte // t terminal, l lexical class, r reference, s sequence, a alternative, ? optional, * repetition
	text     string
	children []*grammarNode
}

type grammarProduction struct {
	name string
	line int
	body *grammarNode
}

type specGrammar struct {
	productions []grammarProduction
	byName      map[string]*grammarNode
}

type grammarToken struct {
	kind byte
	text string
	line int
}

var grammarDefinition = regexp.MustCompile(`(?m)^[ \t]*[A-Z][A-Za-z0-9_]*[ \t]*:=`)

func grammarBlocks(source string) ([]docBlock, error) {
	var blocks []docBlock
	var body []string
	language, start := "", 0
	for i, line := range strings.Split(source, "\n") {
		trimmed := strings.TrimSpace(line)
		if start == 0 && strings.HasPrefix(trimmed, "```") {
			language, start = strings.TrimSpace(strings.TrimPrefix(trimmed, "```")), i+1
			body = nil
			continue
		}
		if start != 0 && trimmed == "```" {
			text := strings.Join(body, "\n")
			if language == "ebnf" {
				blocks = append(blocks, docBlock{line: start, text: text})
			} else if grammarDefinition.MatchString(text) {
				return nil, fmt.Errorf("line %d: grammar fence must be labelled ebnf, found %q", start, language)
			}
			start = 0
			continue
		}
		if start != 0 {
			body = append(body, line)
		}
	}
	if start != 0 {
		return nil, fmt.Errorf("line %d: unclosed %s fence", start, language)
	}
	if len(blocks) == 0 {
		return nil, fmt.Errorf("no ebnf fences")
	}
	return blocks, nil
}

func grammarTokens(source string, line int) ([]grammarToken, error) {
	var tokens []grammarToken
	for len(source) > 0 {
		c := source[0]
		if c == '\n' {
			line++
			source = source[1:]
			continue
		}
		if c == ' ' || c == '\t' || c == '\r' {
			source = source[1:]
			continue
		}
		if strings.HasPrefix(source, "//") {
			if end := strings.IndexByte(source, '\n'); end >= 0 {
				source = source[end:]
			} else {
				source = ""
			}
			continue
		}
		token := grammarToken{line: line}
		switch {
		case c == '\'' || c == '"':
			token.kind = 't'
			source = source[1:]
			for len(source) > 0 && source[0] != c {
				r, _, rest, err := strconv.UnquoteChar(source, c)
				if err != nil || source[0] == '\n' {
					return nil, fmt.Errorf("line %d: invalid quoted terminal", line)
				}
				token.text += string(r)
				source = rest
			}
			if len(source) == 0 || token.text == "" {
				return nil, fmt.Errorf("line %d: empty or unclosed quoted terminal", line)
			}
			source = source[1:]
		case c == '?':
			end := strings.IndexByte(source[1:], '?')
			if end < 0 {
				return nil, fmt.Errorf("line %d: unclosed lexical class", line)
			}
			token.kind, token.text = 'l', strings.TrimSpace(source[1:end+1])
			if token.text == "" || strings.Contains(token.text, "\n") {
				return nil, fmt.Errorf("line %d: invalid lexical class", line)
			}
			source = source[end+2:]
		case c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z':
			end := 1
			for end < len(source) && (unicode.IsLetter(rune(source[end])) || unicode.IsDigit(rune(source[end])) || source[end] == '_') {
				end++
			}
			token.kind, token.text = 'r', source[:end]
			source = source[end:]
		case strings.HasPrefix(source, ":="):
			token.kind, token.text = '=', ":="
			source = source[2:]
		case strings.ContainsRune("|{}[]()", rune(c)):
			token.kind, token.text = c, string(c)
			source = source[1:]
		default:
			return nil, fmt.Errorf("line %d: unexpected EBNF character %q", line, c)
		}
		tokens = append(tokens, token)
	}
	return append(tokens, grammarToken{line: line}), nil
}

type grammarReader struct {
	tokens []grammarToken
	i      int
}

func (r *grammarReader) definitionAhead() bool {
	return r.tokens[r.i].kind == 'r' && r.i+1 < len(r.tokens) && r.tokens[r.i+1].kind == '='
}

func (r *grammarReader) expression() (*grammarNode, error) {
	var choices []*grammarNode
	for {
		var sequence []*grammarNode
		for !r.definitionAhead() {
			token := r.tokens[r.i]
			if token.kind == 0 || strings.ContainsRune("|)}]", rune(token.kind)) {
				break
			}
			r.i++
			node := &grammarNode{kind: token.kind, text: token.text}
			switch token.kind {
			case 't', 'l', 'r':
			case '(', '[', '{':
				body, err := r.expression()
				if err != nil {
					return nil, err
				}
				close := map[byte]byte{'(': ')', '[': ']', '{': '}'}[token.kind]
				if r.tokens[r.i].kind != close {
					return nil, fmt.Errorf("line %d: expected %q", r.tokens[r.i].line, close)
				}
				r.i++
				node = body
				if token.kind == '[' || token.kind == '{' {
					kind := byte('?')
					if token.kind == '{' {
						kind = '*'
					}
					node = &grammarNode{kind: kind, children: []*grammarNode{body}}
				}
			default:
				return nil, fmt.Errorf("line %d: expected EBNF term, found %q", token.line, token.text)
			}
			sequence = append(sequence, node)
		}
		if len(sequence) == 0 {
			return nil, fmt.Errorf("line %d: empty EBNF alternative", r.tokens[r.i].line)
		}
		node := sequence[0]
		if len(sequence) > 1 {
			node = &grammarNode{kind: 's', children: sequence}
		}
		choices = append(choices, node)
		if r.tokens[r.i].kind != '|' {
			break
		}
		r.i++
	}
	if len(choices) == 1 {
		return choices[0], nil
	}
	return &grammarNode{kind: 'a', children: choices}, nil
}

func readGrammar(source string) (*specGrammar, error) {
	blocks, err := grammarBlocks(source)
	if err != nil {
		return nil, err
	}
	g := &specGrammar{byName: make(map[string]*grammarNode)}
	for _, block := range blocks {
		tokens, err := grammarTokens(block.text, block.line+1)
		if err != nil {
			return nil, err
		}
		r := grammarReader{tokens: tokens}
		before := len(g.productions)
		for r.tokens[r.i].kind != 0 {
			if !r.definitionAhead() {
				return nil, fmt.Errorf("line %d: expected production name and :=", r.tokens[r.i].line)
			}
			name := r.tokens[r.i]
			r.i += 2
			body, err := r.expression()
			if err != nil {
				return nil, err
			}
			if _, exists := g.byName[name.text]; exists {
				return nil, fmt.Errorf("line %d: duplicate production %s", name.line, name.text)
			}
			g.byName[name.text] = body
			g.productions = append(g.productions, grammarProduction{name.text, name.line, body})
		}
		if len(g.productions) == before {
			return nil, fmt.Errorf("line %d: empty ebnf fence", block.line)
		}
	}
	return g, nil
}

func walkGrammar(n *grammarNode, visit func(*grammarNode)) {
	visit(n)
	for _, child := range n.children {
		walkGrammar(child, visit)
	}
}

// Only File is exempt from being referenced, as documented in SPEC's opening.
var grammarRoots = map[string]string{"File": "complete source file"}

var grammarLexemes = map[string]struct {
	text string
	kind lexer.Kind
}{
	"identifier":                  {"v", lexer.Ident},
	"lower-case identifier":       {"v", lexer.Ident},
	"upper-case identifier":       {"C", lexer.Ident},
	"sample identifier":           {"sample", lexer.Ident},
	"induction identifier":        {"induction", lexer.Ident},
	"requires identifier":         {"requires", lexer.Ident},
	"integer literal":             {"1", lexer.Int},
	"float literal":               {"1.5", lexer.Float},
	"string literal":              {`"text"`, lexer.String},
	"interpolated string literal": {`"${1}"`, lexer.InterpTok},
}

func (g *specGrammar) wellFormed(roots map[string]string) error {
	for _, p := range g.productions {
		var err error
		walkGrammar(p.body, func(n *grammarNode) {
			if n.kind == 'r' && g.byName[n.text] == nil {
				err = fmt.Errorf("line %d: %s references undefined production %s", p.line, p.name, n.text)
			}
			if n.kind == 'l' {
				if _, ok := grammarLexemes[n.text]; !ok {
					err = fmt.Errorf("line %d: unknown lexical class %q", p.line, n.text)
				}
			}
		})
		if err != nil {
			return err
		}
	}
	seen := make(map[string]bool)
	var reach func(string)
	reach = func(name string) {
		if seen[name] {
			return
		}
		seen[name] = true
		walkGrammar(g.byName[name], func(n *grammarNode) {
			if n.kind == 'r' {
				reach(n.text)
			}
		})
	}
	for name, reason := range roots {
		if g.byName[name] == nil || reason == "" {
			return fmt.Errorf("invalid documented root %s", name)
		}
		reach(name)
	}
	for _, p := range g.productions {
		if !seen[p.name] {
			return fmt.Errorf("line %d: unused production %s (unreachable from documented roots)", p.line, p.name)
		}
	}
	return nil
}

func (g *specGrammar) terminalsMatch(fixed map[string]lexer.Kind) error {
	terminals := make(map[string]bool)
	for _, p := range g.productions {
		walkGrammar(p.body, func(n *grammarNode) {
			if n.kind == 't' {
				terminals[n.text] = true
			}
		})
	}
	var missing, extra []string
	for text := range terminals {
		if _, ok := fixed[text]; !ok {
			extra = append(extra, text)
		}
	}
	for text := range fixed {
		if !terminals[text] {
			missing = append(missing, text)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	if len(missing)+len(extra) > 0 {
		return fmt.Errorf("terminal mismatch: lexer tokens absent from grammar %q; grammar terminals absent from lexer %q", missing, extra)
	}
	return nil
}

func loadSpecGrammar(t *testing.T) *specGrammar {
	t.Helper()
	source, err := os.ReadFile("../SPEC.md")
	if err != nil {
		t.Fatal(err)
	}
	g, err := readGrammar(string(source))
	if err != nil {
		t.Fatal(err)
	}
	if err := g.wellFormed(grammarRoots); err != nil {
		t.Fatal(err)
	}
	return g
}

func TestSpecGrammar(t *testing.T) {
	g := loadSpecGrammar(t)
	fixed := make(map[string]lexer.Kind)
	lexer.VisitFixedTokens(func(kind lexer.Kind, text string) {
		if _, duplicate := fixed[text]; duplicate {
			t.Errorf("duplicate lexer token %q", text)
		}
		fixed[text] = kind
		tokens, err := lexer.New(text).All()
		if err != nil || len(tokens) == 0 || tokens[0].Kind != kind || tokens[0].Text != text {
			t.Errorf("fixed token %q is not produced as advertised: %v, %v", text, tokens, err)
		}
	})
	if err := g.terminalsMatch(fixed); err != nil {
		t.Fatal(err)
	}
	for name, leaf := range grammarLexemes {
		tokens, err := lexer.New(leaf.text).All()
		if err != nil || len(tokens) == 0 || tokens[0].Kind != leaf.kind {
			t.Errorf("lexical leaf %s: %v, %v", name, tokens, err)
		}
	}
	t.Logf("grammar: %d productions, %d fixed terminals", len(g.productions), len(fixed))
}

// A sentence keeps lexical atoms until rendering, so repeated identifier leaves
// get fresh names (record fields and instance-head parameters must be unique).
type grammarAtom struct {
	text    string
	lexical bool
}

type grammarSentence struct {
	atoms []grammarAtom
	path  string
}

func joinSentences(a, b grammarSentence) grammarSentence {
	atoms := make([]grammarAtom, 0, len(a.atoms)+len(b.atoms))
	atoms = append(atoms, a.atoms...)
	atoms = append(atoms, b.atoms...)
	return grammarSentence{atoms, strings.TrimSpace(a.path + " " + b.path)}
}

func sentenceProduct(left, right []grammarSentence) []grammarSentence {
	out := make([]grammarSentence, 0, len(left)*len(right))
	for _, a := range left {
		for _, b := range right {
			out = append(out, joinSentences(a, b))
		}
	}
	return out
}

// Minimal finite witnesses close the depth frontier, without hard-coding an
// alternative grammar. A fixed point also rejects unproductive reference cycles.
func (g *specGrammar) witnesses() (map[*grammarNode]grammarSentence, error) {
	witnesses := make(map[*grammarNode]grammarSentence)
	changed := true
	for changed {
		changed = false
		var solve func(*grammarNode) (grammarSentence, bool)
		solve = func(n *grammarNode) (grammarSentence, bool) {
			var candidate grammarSentence
			ok := true
			switch n.kind {
			case 't', 'l':
				candidate.atoms = []grammarAtom{{n.text, n.kind == 'l'}}
			case 'r':
				candidate, ok = witnesses[g.byName[n.text]]
				candidate.path = n.text + " " + candidate.path
			case '?', '*':
				// Empty is the shortest witness, but solve the body as well.
				solve(n.children[0])
			case 'a':
				ok = false
				for i, child := range n.children {
					s, finite := solve(child)
					if finite && (!ok || len(s.atoms) < len(candidate.atoms)) {
						candidate, ok = s, true
						candidate.path = fmt.Sprintf("alt%d %s", i+1, s.path)
					}
				}
			case 's':
				for _, child := range n.children {
					s, finite := solve(child)
					ok = ok && finite
					candidate = joinSentences(candidate, s)
				}
			}
			if old, exists := witnesses[n]; ok && (!exists || len(candidate.atoms) < len(old.atoms)) {
				witnesses[n] = candidate
				changed = true
			}
			result, finite := witnesses[n]
			return result, finite
		}
		for _, p := range g.productions {
			solve(p.body)
		}
	}
	for _, p := range g.productions {
		if _, ok := witnesses[p.body]; !ok {
			return nil, fmt.Errorf("production %s has no finite sentence", p.name)
		}
	}
	return witnesses, nil
}

// Depth counts EBNF expansion nodes: references, alternatives, concatenations,
// optionals and repetitions each consume one level. At zero, a node is completed
// with its shortest finite witness (ultimately identifiers/literals/terminals).
// Repetitions enumerate 0..depth copies; every Cartesian combination is retained,
// with no random sampling, sentence cap, or parser-based rejection/filtering.
// Testing EVERY production as a root also exercises deeply factored helper rules.
func (g *specGrammar) expand(n *grammarNode, depth int, witnesses map[*grammarNode]grammarSentence) []grammarSentence {
	if depth == 0 || n.kind == 't' || n.kind == 'l' {
		s := witnesses[n]
		s.path = "leaf(" + strings.TrimSpace(s.path) + ")"
		return []grammarSentence{s}
	}
	switch n.kind {
	case 'r':
		out := g.expand(g.byName[n.text], depth-1, witnesses)
		for i := range out {
			out[i].path = n.text + " -> " + out[i].path
		}
		return out
	case 'a':
		var out []grammarSentence
		for i, child := range n.children {
			for _, s := range g.expand(child, depth-1, witnesses) {
				s.path = fmt.Sprintf("alt%d -> %s", i+1, s.path)
				out = append(out, s)
			}
		}
		return out
	case 's':
		out := []grammarSentence{{}}
		for i, child := range n.children {
			part := g.expand(child, depth-1, witnesses)
			for j := range part {
				part[j].path = fmt.Sprintf("term%d(%s)", i+1, part[j].path)
			}
			out = sentenceProduct(out, part)
		}
		return out
	case '?', '*':
		max := depth
		if n.kind == '?' {
			max = 1
		}
		out := []grammarSentence{{path: "empty"}}
		part := g.expand(n.children[0], depth-1, witnesses)
		repeated := []grammarSentence{{}}
		for count := 1; count <= max; count++ {
			repeated = sentenceProduct(repeated, part)
			for _, s := range repeated {
				s.path = fmt.Sprintf("repeat%d(%s)", count, s.path)
				out = append(out, s)
			}
		}
		return out
	}
	panic("unknown EBNF node")
}

type grammarContext struct {
	node           *grammarNode
	prefix, suffix grammarSentence
	path           string
}

// Expand nested EBNF subexpressions too, in the smallest enclosing production
// context. Otherwise harmless factoring (e.g. an optional list inside a choice)
// could hide every branch behind the depth frontier. References are not followed
// here: each referenced production already has its own generation root.
func grammarContexts(n *grammarNode, witnesses map[*grammarNode]grammarSentence) []grammarContext {
	out := []grammarContext{{node: n}}
	for i, child := range n.children {
		var prefix, suffix grammarSentence
		if n.kind == 's' {
			for _, before := range n.children[:i] {
				prefix = joinSentences(prefix, witnesses[before])
			}
			for _, after := range n.children[i+1:] {
				suffix = joinSentences(suffix, witnesses[after])
			}
		}
		for _, context := range grammarContexts(child, witnesses) {
			context.prefix = joinSentences(prefix, context.prefix)
			context.suffix = joinSentences(context.suffix, suffix)
			context.path = fmt.Sprintf("%c%d/%s", n.kind, i+1, context.path)
			out = append(out, context)
		}
	}
	return out
}

func renderSentence(s grammarSentence) string {
	words := make([]string, 0, len(s.atoms))
	for i, atom := range s.atoms {
		text := atom.text
		if atom.lexical {
			leaf := grammarLexemes[text]
			text = leaf.text
			if leaf.kind == lexer.Ident && text != "sample" && text != "induction" {
				text += strconv.Itoa(i)
			}
		}
		words = append(words, text)
	}
	// Spaces prevent accidental token fusion (notably '.' + integer => float).
	// All braces and match-arm bars stay on one line; separators are explicit.
	return strings.Join(words, " ")
}

// These are declaration roots, not statement/expression/pattern/type roots.
// Their cross-name predicates cannot be represented by independent lexical
// leaves. All their context-free constituents are generated separately below.
var grammarGenerationAllowlist = map[string]string{
	"Class":    "superclass argument must equal the class parameter, not a fresh identifier",
	"Instance": "given arguments must reuse parameters declared in this instance head",
	"LawDecl":  "equations and Boolean/pipeline predicates reject unparenthesized equality operands at the predicate's level",
	"LawBody":  "arbitrary Expr combinations may form an ambiguous Boolean/pipeline predicate; expression constituents are generated separately",
	"Decl":     "includes the name-dependent Class and Instance declarations",
	"File":     "includes Decl; imports and all context-free declaration constituents have their own roots",
}

func grammarProgram(root, sentence string) (string, bool) {
	expression := ""
	switch root {
	case "Expr", "InferExpr", "IfExpr", "Binary", "Unary", "Postfix", "Primary", "Block", "Section", "Ident", "Int", "Float", "String", "Interp":
		expression = sentence
	case "CallArg", "Projection":
		expression = "f(" + sentence + ")"
	case "RecordFields":
		expression = "C" + sentence
	case "BinOp":
		expression = "1 " + sentence + " 1"
	case "LParam":
		expression = "func(" + sentence + ") => 1"
	case "Stmt", "LetStmt", "LocalFunc":
		expression = "{ " + sentence + "; 1 }"
	case "DoStmt":
		expression = "do { " + sentence + "; ioPure(unit) }"
	case "Arms", "Arm":
		expression = "match value { " + sentence + " }"
	case "ArmSep":
		expression = "match value { _ => 1 " + sentence + " _ => 2 }"
	case "Pattern", "ConstructorName", "LowerIdent":
		expression = "match value { " + sentence + " => 1 }"
	case "RecordPatternFields":
		expression = "match value { C" + sentence + " => 1 }"
	case "ListRest":
		expression = "match value { [_, " + sentence + "] => 1 }"
	case "Type", "Name", "UpperIdent":
		return "package main; alias T = " + sentence, true
	case "TypeDecl", "AliasDecl", "Func", "Extern", "Import", "ProofDecl":
		return "package main; " + sentence, true
	case "Tactic":
		return "package main; proof lawName() { " + sentence + " }", true
	case "TacticBlock":
		return "package main; proof lawName() " + sentence, true
	case "TacticArm":
		return "package main; proof lawName(x) { match x { " + sentence + " } }", true
	case "TacticPattern":
		return "package main; proof lawName(x) { match x { " + sentence + " => refl } }", true
	case "TacticRecordFields":
		return "package main; proof lawName(x) { match x { C" + sentence + " => refl } }", true
	case "Separator":
		return sentence + " package main", true
	case "Ctor":
		return "package main; type T = " + sentence, true
	case "DataParams":
		return "package main; alias T" + sentence + " = Int", true
	case "TParams":
		return "package main; func f" + sentence + "() Int { 1 }", true
	case "TParam":
		return "package main; class C[" + sentence + "] {}", true
	case "Param":
		return "package main; func f(" + sentence + ") Int { 1 }", true
	case "MethodSig":
		return "package main; class C[A] { " + sentence + " }", true
	case "InstMethod":
		return "package main; instance C[T] { " + sentence + " }", true
	case "Head":
		return "package main; instance C[" + sentence + "] {}", true
	case "HeadArg":
		return "package main; instance C[T[" + sentence + "]] {}", true
	case "Constraint":
		// The wrapper declares exactly the name referenced by this constraint.
		parameter := sentence[strings.LastIndex(sentence, "[")+1 : strings.LastIndex(sentence, "]")]
		return "package main; instance C[T[" + parameter + "]] given " + sentence + " {}", true
	default:
		return "", false
	}
	return "package main; func main() IO[Unit] { let _x = " + expression + "; ioPure(unit) }", true
}

func TestSpecGrammarGenerated(t *testing.T) {
	g := loadSpecGrammar(t)
	witnesses, err := g.witnesses()
	if err != nil {
		t.Fatal(err)
	}
	for name, reason := range grammarGenerationAllowlist {
		if g.byName[name] == nil || reason == "" {
			t.Fatalf("stale or unexplained generation allowlist entry %s", name)
		}
	}
	generated, roots := 0, 0
	for _, p := range g.productions {
		if reason, exempt := grammarGenerationAllowlist[p.name]; exempt {
			t.Logf("generation allowlist: %s: %s", p.name, reason)
			continue
		}
		roots++
		t.Run(p.name, func(t *testing.T) {
			seen := make(map[string]bool)
			for _, context := range grammarContexts(p.body, witnesses) {
				for depth := 0; depth <= 3; depth++ {
					for _, s := range g.expand(context.node, depth, witnesses) {
						s = joinSentences(joinSentences(context.prefix, s), context.suffix)
						sentence := renderSentence(s)
						if seen[sentence] {
							continue
						}
						seen[sentence] = true
						program, ok := grammarProgram(p.name, sentence)
						if !ok {
							t.Fatalf("line %d: production %s needs a parser context or a reasoned generation allowlist entry", p.line, p.name)
						}
						if _, err := parser.Parse(program); err != nil {
							t.Fatalf("SPEC.md:%d: %s depth %d\nproduction path: %s/%s -> %s\nsentence: %s\nprogram: %s\nparser: %v", p.line, p.name, depth, p.name, context.path, s.path, sentence, program, err)
						}
					}
				}
			}
			if len(seen) == 0 {
				t.Fatal("no generated sentences")
			}
			generated += len(seen)
			t.Logf("%d distinct sentences", len(seen))
		})
	}
	t.Logf("grammar generation: %d productions, %d generated roots, %d allowlisted declaration roots, %d distinct root/sentence pairs (depth <= 3)", len(g.productions), roots, len(grammarGenerationAllowlist), generated)
}

func TestGrammarReader(t *testing.T) {
	valid := "```ebnf\nRoot := 'let' [Name] {',' Name} ('=' | '+') // comment\nName := ? identifier ?\n```"
	g, err := readGrammar(valid)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.wellFormed(map[string]string{"Root": "test entry"}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ source, want string }{
		{"```\nRoot := 'let'\n```", "grammar fence must be labelled ebnf"},
		{"```text\nRoot := 'let'\n```", "grammar fence must be labelled ebnf"},
		{"```ebnf\nRoot := 'let'", "unclosed ebnf fence"},
		{"```ebnf\n```", "empty ebnf fence"},
		{"```ebnf\nRoot := 'let'\n```\n```ebnf\nRoot := 'do'\n```", "duplicate production Root"},
		{"```ebnf\nRoot := ['let'\n```", "expected ']'"},
		{"```ebnf\nRoot := 'let' |\n```", "empty EBNF alternative"},
		{"```ebnf\nRoot :=\nOther := 'let'\n```", "empty EBNF alternative"},
		{"```ebnf\nRoot := 'let\n```", "empty or unclosed quoted terminal"},
		{"```ebnf\nRoot = 'let'\n```", "unexpected EBNF character"},
	} {
		_, err := readGrammar(tc.source)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("readGrammar(%q) = %v; want %s", tc.source, err, tc.want)
		}
	}
	for _, tc := range []struct{ body, want string }{
		{"Root := Missing", "undefined production Missing"},
		{"Root := 'let'\nUnused := 'do'", "unused production Unused"},
		{"Root := 'let'\nCycle := Cycle", "unused production Cycle"},
		{"Root := ? unknown ?", "unknown lexical class"},
	} {
		g, err := readGrammar("```ebnf\n" + tc.body + "\n```")
		if err == nil {
			err = g.wellFormed(map[string]string{"Root": "test entry"})
		}
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("wellFormed(%q) = %v; want %s", tc.body, err, tc.want)
		}
	}
	if err := g.terminalsMatch(map[string]lexer.Kind{"let": lexer.Keyword, "missing": lexer.Keyword}); err == nil || !strings.Contains(err.Error(), "missing") || !strings.Contains(err.Error(), ",") {
		t.Fatalf("terminal drift not detected in both directions: %v", err)
	}
}

func TestGrammarExpansion(t *testing.T) {
	g, err := readGrammar("```ebnf\nRoot := ('true' | 'false') ['!']\nRepeat := {'+' | '-'}\nRecursive := '!' Recursive | 'true'\n```")
	if err != nil {
		t.Fatal(err)
	}
	witnesses, err := g.witnesses()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		depth int
		want  []string
	}{
		{"Root", 0, []string{"true"}},
		{"Root", 3, []string{"false", "false !", "true", "true !"}},
		{"Repeat", 2, []string{"", "+", "+ +", "+ -", "-", "- +", "- -"}},
		{"Recursive", 3, []string{"! true", "true"}},
	} {
		var got []string
		for _, s := range g.expand(g.byName[tc.name], tc.depth, witnesses) {
			got = append(got, renderSentence(s))
		}
		sort.Strings(got)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("expand(%s, %d) = %q, want %q", tc.name, tc.depth, got, tc.want)
		}
	}
	cycle, err := readGrammar("```ebnf\nRoot := Root\n```")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cycle.witnesses(); err == nil || !strings.Contains(err.Error(), "no finite sentence") {
		t.Fatalf("unproductive cycle accepted: %v", err)
	}
}

func TestGrammarNestedContexts(t *testing.T) {
	g, err := readGrammar("```ebnf\nRoot := '(' ['[' ['true' | 'false'] ']'] ')'\n```")
	if err != nil {
		t.Fatal(err)
	}
	witnesses, err := g.witnesses()
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]bool)
	for _, context := range grammarContexts(g.byName["Root"], witnesses) {
		for _, s := range g.expand(context.node, 3, witnesses) {
			s = joinSentences(joinSentences(context.prefix, s), context.suffix)
			seen[renderSentence(s)] = true
		}
	}
	want := map[string]bool{"( )": true, "( [ ] )": true, "( [ true ] )": true, "( [ false ] )": true}
	if !reflect.DeepEqual(seen, want) {
		t.Fatalf("nested choices hidden by depth frontier: got %v, want %v", seen, want)
	}
}

// These old grammar shapes really do describe invalid parser input. Keep the
// detector honest: it must not silently filter generated failures, or substitute
// a generic identifier for a malformed expression/pattern.
func TestGrammarGeneratedRejections(t *testing.T) {
	for _, tc := range []struct{ root, body, want string }{
		{
			"Pattern",
			"Pattern := ConstructorName '{' [Ident ':' Pattern] [',' '..'] '}'\nConstructorName := ? upper-case identifier ?\nIdent := ? identifier ?",
			"record pattern needs fields or ..",
		},
		{
			"Section",
			"Section := '(' '_' '+' Expr ')'\nExpr := 'func' '(' ')' '=>' Int\nInt := ? integer literal ?",
			`expected expression, found "func"`,
		},
	} {
		g, err := readGrammar("```ebnf\n" + tc.body + "\n```")
		if err != nil {
			t.Fatal(err)
		}
		witnesses, err := g.witnesses()
		if err != nil {
			t.Fatal(err)
		}
		rejected := false
		for _, s := range g.expand(g.byName[tc.root], 3, witnesses) {
			program, ok := grammarProgram(tc.root, renderSentence(s))
			if !ok {
				t.Fatalf("missing context for %s", tc.root)
			}
			if _, err := parser.Parse(program); err != nil && strings.Contains(err.Error(), tc.want) {
				rejected = true
			}
		}
		if !rejected {
			t.Errorf("%s: invalid grammar failed to expose parser error %q", tc.root, tc.want)
		}
	}
}

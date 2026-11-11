package laws

import (
	"fmt"

	"github.com/reoring/go4t/ast"
	"github.com/reoring/go4t/types"
)

// Each node is a function body with its resolved dictionary environment. This
// includes instance methods, local functions and lambdas, not just Funcs.
// Reference edges keep even unused callbacks in the dependency closure. A
// reference-only edge in a recursive SCC cannot establish structural decrease.
type terminationNode struct {
	decl         ast.Node
	name         string
	global       string
	method       *ast.InstMethod
	dictionaries []*dictionary
	params       []ast.Param
	block        *ast.Block
	expr         ast.Expr
	dicts        dictionaryEnv
	captured     terminationEnv
	edges        []terminationEdge
	index, low   int
	onStack      bool
}

type terminationEdge struct {
	to       *terminationNode
	position int // first strict decrease; -1 means no certified decrease
}

// For a fixed global, dictionaries in scheme-constraint order identify the
// same instantiation as the termination node's bound dictionary environment.
type decreasingPosition struct {
	dicts    []*dictionary
	position int
}

// root is one-based (zero means no parameter provenance). Only pattern descent
// increases depth; arithmetic, projections and reconstructed data do not.
type terminationValue struct {
	root, depth int
	fields      []terminationValue
	function    *terminationNode
	generalized *terminationBinding
}

type terminationBinding struct {
	expr   ast.Expr
	scheme *types.Scheme
	env    terminationEnv
	dicts  dictionaryEnv
}

type terminationEnv map[string]terminationValue

type terminationChecker struct {
	e                 *evaluator
	nodes             []*terminationNode
	decreasing        map[string][]decreasingPosition
	decreasingMethods map[*ast.InstMethod][]decreasingPosition
	err               error
}

func (e *evaluator) checkDependencies(law *types.LawInfo, proof *types.ProofInfo) error {
	return e.checkEligibility(law, proof, true)
}

// Sampling uses the same taint scan but never treats failed termination as an
// excuse to sample an otherwise eligible law.
func (e *evaluator) checkEligibility(law *types.LawInfo, proof *types.ProofInfo, termination bool) error {
	c := &terminationChecker{e: e}
	root := &terminationNode{name: sourceName(e.p, law.Decl.Name)}
	if law.Decl.Requires != nil {
		c.expression(root, law.Decl.Requires, nil, nil, false)
	}
	c.expression(root, law.Decl.LHS, nil, nil, false)
	c.expression(root, law.Decl.RHS, nil, nil, false)
	// Conditions and rewrite arguments must be total too: a divergent or
	// effectful assumption cannot manufacture a proof of an arbitrary goal.
	ast.Walk(proof.Decl, func(node ast.Node) {
		switch tactic := node.(type) {
		case *ast.TMatch:
			if tactic.Scrut != nil {
				c.expression(root, tactic.Scrut, nil, nil, false)
			}
		case *ast.TIf:
			c.expression(root, tactic.Cond, nil, nil, false)
		case *ast.TRewrite:
			for _, arg := range tactic.Args {
				c.expression(root, arg, nil, nil, false)
			}
			// Rewriting normalizes the referenced equation too. Its
			// recursive globals need the same certificates as the goal.
			if tactic.Law.Name != law.Decl.Name {
				referenced := e.p.Laws[tactic.Law.Name]
				c.expression(root, referenced.Decl.LHS, nil, nil, false)
				c.expression(root, referenced.Decl.RHS, nil, nil, false)
				if referenced.Decl.Requires != nil {
					c.expression(root, referenced.Decl.Requires, nil, nil, false)
				}
			}
		}
	})
	// Scanning a body appends newly discovered callees to this worklist.
	for i := 0; i < len(c.nodes) && c.err == nil; i++ {
		n := c.nodes[i]
		env := cloneTerminationEnv(n.captured)
		// Captured variables are not parameters of the nested function.
		for name, value := range env {
			env[name] = capturedTerminationValue(value)
		}
		for i, param := range n.params {
			value := terminationValue{root: i + 1}
			if param.Pat == nil {
				env[param.Name] = value
			} else {
				c.pattern(param.Pat, value, env)
			}
		}
		if n.block != nil {
			c.block(n, n.block, env, n.dicts)
		} else {
			c.expression(n, n.expr, env, n.dicts, false)
		}
	}
	if c.err != nil {
		return c.err
	}
	if !termination {
		return nil
	}
	if bad := c.nonterminating(); bad != nil {
		return fmt.Errorf("depends on %s which may not terminate", bad.name)
	}
	e.decreasing = c.decreasing
	e.decreasingMethods = c.decreasingMethods
	return nil
}

func cloneTerminationEnv(env terminationEnv) terminationEnv {
	result := make(terminationEnv, len(env))
	for name, value := range env {
		result[name] = value
	}
	return result
}

func capturedTerminationValue(value terminationValue) terminationValue {
	value.root, value.depth = 0, 0
	if len(value.fields) != 0 {
		fields := make([]terminationValue, len(value.fields))
		for i, field := range value.fields {
			fields[i] = capturedTerminationValue(field)
		}
		value.fields = fields
	}
	return value
}

func sameDictionaryEnv(a, b dictionaryEnv) bool {
	if len(a) != len(b) {
		return false
	}
	for key, value := range a {
		other := b[key]
		if other == nil || value.instance != other.instance || !sameDictionaries(value.args, other.args) {
			return false
		}
	}
	return true
}

func (c *terminationChecker) node(decl ast.Node, name string, params []ast.Param, block *ast.Block, expr ast.Expr, env terminationEnv, dicts dictionaryEnv) *terminationNode {
	for _, n := range c.nodes {
		if n.decl == decl && sameDictionaryEnv(n.dicts, dicts) {
			return n
		}
	}
	n := &terminationNode{decl: decl, name: name, params: params, block: block, expr: expr, captured: env, dicts: dicts}
	c.nodes = append(c.nodes, n)
	return n
}

func (c *terminationChecker) function(name string, dicts []*dictionary) *terminationNode {
	f := c.e.p.Funcs[name]
	if f == nil {
		c.err = fmt.Errorf("missing checked function %s", name)
		return nil
	}
	if reason := c.e.unsupportedType(f.Scheme.Type, false, make(map[string]bool)); reason != "" {
		c.err = fmt.Errorf("%s", reason)
		return nil
	}
	if f.Decl.Extern != nil {
		if !c.e.trustedExtern(f) {
			c.err = externError(f.Decl.Extern)
		}
		return nil
	}
	bound, err := bindDictionaries(nil, f.Scheme.Constraints, dicts)
	if err != nil {
		c.err = err
		return nil
	}
	n := c.node(f.Decl, sourceName(c.e.p, name), f.Decl.Params, f.Decl.Body, nil, nil, bound)
	n.global, n.dictionaries = name, dicts
	// Structural models can introduce calls absent from the runtime body.
	// Certify those callees too; otherwise a neutral append/fmap would unfold
	// without a decreasing-position certificate.
	switch c.e.listModel(name) {
	case "reverse", "flatten", "intercalate":
		c.function(c.e.listFunctions["append"], nil)
	case "indexed":
		c.method("fmap", []*dictionary{c.e.listFunctor})
	}
	return n
}

func (c *terminationChecker) method(name string, dicts []*dictionary) *terminationNode {
	decl, bound, err := c.e.methodBody(name, dicts)
	if err != nil {
		c.err = err
		return nil
	}
	params := make([]ast.Param, len(decl.Params))
	for i, name := range decl.Params {
		params[i].Name = name
	}
	inst := dicts[0].instance
	label := sourceName(c.e.p, inst.Class.Name) + "[" + sourceName(c.e.p, inst.TypeCon.Name) + "]." + name
	n := c.node(decl, label, params, decl.Body, nil, nil, bound)
	n.method, n.dictionaries = decl, dicts
	// Modeled map comparison also calls the lawful Ord's Eq superclass.
	// Include that implicit dependency so recursive Eq gets the same
	// certificates as an explicit == in a law or tactic.
	if name == "compare" && c.e.eligibleMapDictionary(dicts[0], make(map[*types.Instance]bool)) {
		eq, err := c.e.mapEqDictionary(dicts[0])
		if err != nil {
			c.err = err
			return nil
		}
		c.method("eq", []*dictionary{eq})
	}
	return n
}

func (c *terminationChecker) edge(from, to *terminationNode, args []terminationValue) {
	if to == nil {
		return
	}
	position := -1
	for i, arg := range args {
		if arg.root != i+1 {
			break
		}
		if arg.depth > 0 {
			position = i
			break
		}
	}
	from.edges = append(from.edges, terminationEdge{to: to, position: position})
}

func (c *terminationChecker) resolve(refs []types.DictRef, env dictionaryEnv) []*dictionary {
	dicts, err := c.e.resolveAll(refs, env)
	if err != nil {
		c.err = err
	}
	return dicts
}

func (c *terminationChecker) expression(from *terminationNode, expr ast.Expr, env terminationEnv, dicts dictionaryEnv, callee bool) terminationValue {
	if expr == nil || c.err != nil {
		return terminationValue{}
	}
	if err := c.e.step(); err != nil {
		c.err = err
		return terminationValue{}
	}
	ann := types.AnnOf(expr)
	if reason := c.e.unsupportedType(ann.Type, false, make(map[string]bool)); reason != "" {
		c.err = fmt.Errorf("%s", reason)
		return terminationValue{}
	}
	var result terminationValue
	switch x := expr.(type) {
	case *ast.Ident:
		switch ann.Ref {
		case types.RefLocal:
			result = env[x.Name]
			if binding := result.generalized; binding != nil {
				bound, err := bindDictionaries(binding.dicts, binding.scheme.Constraints, c.resolve(ann.Dicts, dicts))
				if err != nil {
					c.err = err
					return terminationValue{}
				}
				return c.expression(from, binding.expr, binding.env, bound, callee)
			}
		case types.RefFunc:
			result.function = c.function(x.Name, c.resolve(ann.Dicts, dicts))
			if f := c.e.p.Funcs[x.Name]; f != nil && f.Constant != nil {
				c.edge(from, result.function, []terminationValue{})
				return terminationValue{}
			}
		case types.RefMethod:
			if ds := c.resolve(ann.Dicts, dicts); c.err == nil {
				result.function = c.method(x.Name, ds)
			}
		}
	case *ast.Call:
		fn := c.expression(from, x.Fn, env, dicts, true)
		args := make([]terminationValue, len(x.Args))
		for i, arg := range x.Args {
			args[i] = c.expression(from, arg, env, dicts, false)
		}
		c.edge(from, fn.function, args)
	case *ast.Lambda:
		result.function = c.node(x, from.name+".<lambda>", x.Params, nil, x.Body, cloneTerminationEnv(env), dicts)
	case *ast.BlockExpr:
		return c.block(from, x.Block, cloneTerminationEnv(env), dicts)
	case *ast.TupleLit:
		result.fields = make([]terminationValue, len(x.Elems))
		for i, item := range x.Elems {
			result.fields[i] = c.expression(from, item, env, dicts, false)
		}
	case *ast.ListLit:
		for _, item := range x.Elems {
			c.expression(from, item, env, dicts, false)
		}
	case *ast.RecordLit:
		for _, item := range x.Values {
			c.expression(from, item, env, dicts, false)
		}
	case *ast.RecordUpdate:
		c.expression(from, x.X, env, dicts, false)
		for _, item := range x.Values {
			c.expression(from, item, env, dicts, false)
		}
	case *ast.Field:
		c.expression(from, x.X, env, dicts, false)
	case *ast.Match:
		value := c.expression(from, x.Scrut, env, dicts, false)
		for _, arm := range x.Arms {
			branch := cloneTerminationEnv(env)
			c.pattern(arm.Pat, value, branch)
			c.expression(from, arm.Guard, branch, dicts, false)
			c.expression(from, arm.Body, branch, dicts, false)
		}
	case *ast.If:
		c.expression(from, x.Cond, env, dicts, false)
		c.expression(from, x.Then, env, dicts, false)
		c.expression(from, x.Else, env, dicts, false)
	case *ast.Unary:
		c.expression(from, x.X, env, dicts, false)
	case *ast.Binary:
		a := c.expression(from, x.L, env, dicts, false)
		b := c.expression(from, x.R, env, dicts, false)
		if x.Op == "/" || x.Op == "%" {
			if !nonzeroLiteral(x.R) {
				c.err = fmt.Errorf("law depends on division by a nonconstant divisor")
				if n, ok := x.R.(*ast.IntLit); ok && n.Value == 0 {
					c.err = fmt.Errorf("law depends on division by zero")
				}
			}
		}
		method := ""
		switch x.Op {
		case "==", "!=":
			method = "eq"
		case "<", "<=", ">", ">=":
			method = "compare"
		}
		if method != "" {
			if ds := c.resolve(ann.Dicts, dicts); c.err == nil {
				c.edge(from, c.method(method, ds), []terminationValue{a, b})
			}
		}
	case *ast.Interp:
		for i, item := range x.Exprs {
			arg := c.expression(from, item, env, dicts, false)
			if ds := c.resolve([]types.DictRef{ann.ShowDicts[i]}, dicts); c.err == nil {
				c.edge(from, c.method("show", ds), []terminationValue{arg})
			}
		}
	case *ast.IntLit, *ast.StringLit, *ast.BoolLit:
	default:
		c.err = fmt.Errorf("unsupported law expression %T", expr)
	}
	if !callee {
		c.edge(from, result.function, nil)
	}
	return result
}

func (c *terminationChecker) block(from *terminationNode, block *ast.Block, env terminationEnv, dicts dictionaryEnv) terminationValue {
	for i := 0; i < len(block.Stmts); i++ {
		switch stmt := block.Stmts[i].(type) {
		case *ast.LetStmt:
			if scheme := types.AnnOf(stmt.Value).LetScheme; scheme != nil {
				bound := &terminationBinding{expr: stmt.Value, scheme: scheme, env: cloneTerminationEnv(env), dicts: dicts}
				env[stmt.Name] = terminationValue{generalized: bound}
				bound.env[stmt.Name] = env[stmt.Name]
				continue
			}
			value := c.expression(from, stmt.Value, env, dicts, false)
			if types.AnnOf(stmt.Value).Recursive && value.function != nil {
				value.function.captured[stmt.Name] = value
			}
			if stmt.Pat == nil {
				env[stmt.Name] = value
			} else {
				c.pattern(stmt.Pat, value, env)
			}
		case *ast.ExprStmt:
			c.expression(from, stmt.X, env, dicts, false)
		case *ast.FuncDecl:
			// Allocate the whole adjacent recursive group before scanning bodies.
			group := cloneTerminationEnv(env)
			for ; i < len(block.Stmts); i++ {
				decl, ok := block.Stmts[i].(*ast.FuncDecl)
				if !ok {
					break
				}
				n := c.node(decl, decl.Name, decl.Params, decl.Body, nil, group, dicts)
				group[decl.Name] = terminationValue{function: n}
				c.edge(from, n, nil)
			}
			i--
			env = cloneTerminationEnv(group)
		}
	}
	return c.expression(from, block.Result, env, dicts, false)
}

func (c *terminationChecker) pattern(pattern ast.Pattern, value terminationValue, env terminationEnv) {
	types.BindStructuralPattern(pattern, value, c.e.p.RecordPats, func(value terminationValue, index int) terminationValue {
		if index < len(value.fields) {
			return value.fields[index]
		}
		return terminationValue{root: value.root, depth: value.depth + 1}
	}, func(name string, value terminationValue) {
		env[name] = value
	})
}

func nonzeroLiteral(expr ast.Expr) bool {
	if value, ok := expr.(*ast.IntLit); ok {
		return value.Value != 0
	}
	if value, ok := expr.(*ast.Unary); ok && value.Op == "-" {
		if n, ok := value.X.(*ast.IntLit); ok {
			return n.Value != 0
		}
	}
	return false
}

// Tarjan's SCC algorithm separates genuine recursion from acyclic calls. Every
// intra-SCC edge must decrease at the SAME lexicographic position. Requiring a
// common position prevents alternating resets of earlier parameters in mutual
// recursion; later parameters remain unrestricted.
func (c *terminationChecker) nonterminating() *terminationNode {
	index := 0
	var stack []*terminationNode
	var bad *terminationNode
	var visit func(*terminationNode)
	visit = func(n *terminationNode) {
		index++
		n.index, n.low, n.onStack = index, index, true
		stack = append(stack, n)
		for _, edge := range n.edges {
			to := edge.to
			if to.index == 0 {
				visit(to)
				if to.low < n.low {
					n.low = to.low
				}
			} else if to.onStack && to.index < n.low {
				n.low = to.index
			}
		}
		if n.low != n.index {
			return
		}
		members := make(map[*terminationNode]bool)
		for {
			last := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			last.onStack = false
			members[last] = true
			if last == n {
				break
			}
		}
		position := -2
		for _, member := range c.nodes {
			if !members[member] {
				continue
			}
			for _, edge := range member.edges {
				if !members[edge.to] {
					continue
				}
				if edge.position < 0 || position != -2 && edge.position != position {
					if bad == nil {
						bad = member
					}
				}
				position = edge.position
			}
		}
		// No intra-SCC edge means this is an acyclic function, not a
		// certified recursive one. Publish nothing unless all SCCs pass.
		if bad == nil && position >= 0 {
			for member := range members {
				certificate := decreasingPosition{dicts: member.dictionaries, position: position}
				if member.global != "" {
					if c.decreasing == nil {
						c.decreasing = make(map[string][]decreasingPosition)
					}
					c.decreasing[member.global] = append(c.decreasing[member.global], certificate)
				} else if member.method != nil {
					if c.decreasingMethods == nil {
						c.decreasingMethods = make(map[*ast.InstMethod][]decreasingPosition)
					}
					c.decreasingMethods[member.method] = append(c.decreasingMethods[member.method], certificate)
				}
			}
		}
	}
	for _, n := range c.nodes {
		if n.index == 0 {
			visit(n)
		}
	}
	return bad
}

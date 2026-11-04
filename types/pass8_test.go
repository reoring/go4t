package types_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/reoring/go4t/ast"
	"github.com/reoring/go4t/types"
)

func TestRecordPatternAnnotations(t *testing.T) {
	for _, tt := range []struct {
		name, source string
		names        []string
		indexes      []int
		result       string
	}{
		{"full reordered", `func value(p Entry[Int]) (Int, String) { match p { Entry{value: v, name: n} => (v, n) } }`, []string{"value", "name"}, []int{1, 0}, "Tuple2[Int, String]"},
		{"partial", `func value(p Entry[Int]) Int { match p { Entry{value: v, ..} => v } }`, []string{"value"}, []int{1}, "Int"},
		{"rest only", `func value(p Entry[Int]) Int { match p { Entry{..} => 1 } }`, nil, []int{}, "Int"},
		{"full with rest", `func value(p Entry[Int]) String { match p { Entry{value: _, name: n, ..} => n } }`, []string{"value", "name"}, []int{1, 0}, "String"},
		{"generic", `func value[A](p Entry[A]) A { match p { Entry{value: v, ..} => v } }`, []string{"value"}, []int{1}, "A"},
		{"nested tuple", `func value(p (Int, Entry[String])) (String, Int) { match p { (n, Entry{value: v, ..}) => (v, n) } }`, []string{"value"}, []int{1}, "Tuple2[String, Int]"},
		{"as binding", `func value(p Entry[Int]) Entry[Int] { match p { whole @ Entry{..} => whole } }`, nil, []int{}, "Entry[Int]"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			files := parseFiles(t, "type Entry[A] = Entry{name: String, value: A}\n"+tt.source+pass5Main)
			for range 2 {
				p, err := checkFixture(t, []*ast.Module{{Files: files}})
				if err != nil {
					t.Fatal(err)
				}
				if len(p.RecordPats) != 1 {
					t.Fatalf("record patterns = %d, want 1", len(p.RecordPats))
				}
				for pat, resolved := range p.RecordPats {
					if resolved.Ctor != p.DataTypes["Entry"].Ctors[0] || !reflect.DeepEqual(resolved.Indexes, tt.indexes) {
						t.Fatalf("record pattern resolution = %#v, want Entry %v", resolved, tt.indexes)
					}
					if len(pat.Names) != len(tt.names) || (len(tt.names) != 0 && !reflect.DeepEqual(pat.Names, tt.names)) || len(pat.Pats) != len(tt.names) {
						t.Fatalf("record pattern rewritten: %#v", pat)
					}
				}
				body := p.Funcs["value"].Decl.Body.Result.(*ast.Match)
				if got := types.TypeString(types.AnnOf(body.Arms[0].Body).Type); got != tt.result {
					t.Fatalf("arm body type = %s, want %s", got, tt.result)
				}
				if len(p.Warnings) != 0 {
					t.Fatalf("unexpected warnings: %v", p.Warnings)
				}
				walkExprs(reflect.ValueOf(files), func(e ast.Expr) { assertZonked(t, e) })
			}
		})
	}
}

func TestRecordPatternExhaustiveness(t *testing.T) {
	for _, tt := range []struct {
		name, source string
		complete     bool
	}{
		{"literal with rest catchall", `func value(p Point) Int { match p { Point{x: 0, ..} => 0; Point{..} => 1 } }`, true},
		{"literal alone", `func value(p Point) Int { match p { Point{x: 0, ..} => 0 } }`, false},
		{"positional catchall", `func value(p Point) Int { match p { Point{x: 0, ..} => 0; Point(_, _) => 1 } }`, true},
		{"nested literal complete", `func value(p (Int, Point)) Int { match p { (_, Point{x: 0, ..}) => 0; (_, Point{..}) => 1 } }`, true},
		{"nested literal incomplete", `func value(p (Int, Point)) Int { match p { (_, Point{x: 0, ..}) => 0 } }`, false},
		{"variant complete", `type Shape = Dot{x: Int} | Empty; func value(p Shape) Int { match p { Dot{x: x} => x; Empty => 0 } }`, true},
		{"variant missing", `type Shape = Dot{x: Int} | Empty; func value(p Shape) Int { match p { Dot{..} => 0 } }`, false},
		{"nested record complete", `type Box = Box{point: Point}; func value(p Box) Int { match p { Box{point: Point{x: 0, ..}} => 0; Box{point: Point{..}} => 1 } }`, true},
		{"nested record incomplete", `type Box = Box{point: Point}; func value(p Box) Int { match p { Box{point: Point{x: 0, ..}} => 0 } }`, false},
		{"reordered product complete", `type Pair = Pair{left: Option[Int], right: Option[Int]}; func value(p Pair) Int { match p { Pair{right: None, ..} => 0; Pair{left: None, right: Some(_)} => 1; Pair{right: Some(_), left: Some(_)} => 2 } }`, true},
		{"reordered product incomplete", `type Pair = Pair{left: Option[Int], right: Option[Int]}; func value(p Pair) Int { match p { Pair{right: None, ..} => 0; Pair{left: None, right: Some(_)} => 1 } }`, false},
		{"recursive ignored field", `type Chain = End | Link{next: Chain, value: Option[Int]}; func value(p Chain) Int { match p { End => 0; Link{value: Some(x), ..} => x } }`, false},
		{"recursive ignored field complete", `type Chain = End | Link{next: Chain, value: Option[Int]}; func value(p Chain) Int { match p { End => 0; Link{value: Some(x), ..} => x; Link{value: None, ..} => 0 } }`, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p, err := checkFixture(t, []*ast.Module{{Files: parseFiles(t, "type Point = Point{x: Int, y: Int}\n"+tt.source+pass5Main)}})
			if tt.complete {
				if err != nil {
					t.Fatal(err)
				}
				walkExprs(reflect.ValueOf(p.Files), func(e ast.Expr) { assertZonked(t, e) })
				return
			}
			var typed *types.Error
			if p != nil || !errors.As(err, &typed) || !strings.HasPrefix(typed.Msg, "non-exhaustive match; missing: ") {
				t.Fatalf("Check = (%v, %v), want non-exhaustive match", p, err)
			}
		})
	}
}

func TestRecordPatternErrors(t *testing.T) {
	for _, tt := range []struct{ source, want string }{
		{`func value(p Point) Int { match p { Missing{..} => 0 } }`, "unknown constructor Missing"},
		{`func value(p Option[Int]) Int { match p { Some{..} => 0; None => 1 } }`, "Some is not a record constructor"},
		{`func value(p Option[Int]) Int { match p { None{..} => 0; Some(_) => 1 } }`, "None is not a record constructor"},
		{`func value(p Point) Int { match p { Point{z: _, ..} => 0 } }`, "unknown field z in constructor Point"},
		{`func value(p Point) Int { match p { Point{x: _} => 0 } }`, "missing field y in constructor Point"},
		{`func value(p Point) Int { match p { Point{x: true, ..} => 0; _ => 1 } }`, "type mismatch: expected Int, got Bool"},
		{`func value(p Point) Int { match p { Point{x: v, y: v} => v } }`, "duplicate pattern variable v"},
		{`func value(p Point) Int { match p { v @ Point{x: v, ..} => v } }`, "duplicate pattern variable v"},
		{`func value(p Int) Int { match p { Point{..} => 0 } }`, "type mismatch: expected Int, got Point"},
		{`type Entry[A] = Entry{value: A}; func value(p Entry[String]) Int { match p { Entry{value: 1} => 0; _ => 1 } }`, "type mismatch: expected String, got Int"},
	} {
		t.Run(tt.source, func(t *testing.T) {
			p, err := checkFixture(t, []*ast.Module{{Files: parseFiles(t, "type Point = Point{x: Int, y: Int}\n"+tt.source+pass5Main)}})
			var typed *types.Error
			if p != nil || !errors.As(err, &typed) || typed.Msg != tt.want || typed.Pos.Line == 0 || typed.Pos.Col == 0 {
				t.Fatalf("Check = (%v, %v), want positioned %q", p, err, tt.want)
			}
		})
	}
}

func TestRecordPatternDuplicateFieldAST(t *testing.T) {
	files := parseFiles(t, `
type Point = Point{x: Int, y: Int}
func value(p Point) Int { match p { Point{x: _, y: _, ..} => 0 } }
`+pass5Main)
	pat := files[1].Decls[1].(*ast.FuncDecl).Body.Result.(*ast.Match).Arms[0].Pat.(*ast.PRecord)
	// The parser also rejects duplicates; check this invariant independently.
	pat.Names[1] = pat.Names[0]
	p, err := checkFixture(t, []*ast.Module{{Files: files}})
	var typed *types.Error
	if p != nil || !errors.As(err, &typed) || typed.Msg != "duplicate field x in constructor Point" || typed.Pos != pat.Pos {
		t.Fatalf("Check = (%v, %v), want duplicate field error at %s", p, err, pat.Pos)
	}
}

func TestTupleFieldAnnotations(t *testing.T) {
	files := parseFiles(t, `
func first(p (Int, String)) Int { p.0 }
func second(p (Int, String)) String { p.1 }
func third(p (Int, String, Bool)) Bool { p.2 }
func chained(p (Int, (String, Bool))) String { p.1.0 }
func generic[A, B](p (A, B)) B { p.1 }
func literal() String { (1, "value").1 }
func identity[A](a A) A { a }
func inferred() Int { let p = identity((1, "value")); p.0 }
`+pass5Main)
	for range 2 {
		p, err := checkFixture(t, []*ast.Module{{Files: files}})
		if err != nil {
			t.Fatal(err)
		}
		if p.RecordPats == nil || len(p.RecordPats) != 0 {
			t.Fatalf("record pattern map = %v, want initialized empty map", p.RecordPats)
		}
		for _, tt := range []struct {
			name, typ string
			index     int
		}{
			{"first", "Int", 0},
			{"second", "String", 1},
			{"third", "Bool", 2},
			{"chained", "String", 0},
			{"generic", "B", 1},
			{"literal", "String", 1},
			{"inferred", "Int", 0},
		} {
			field := p.Funcs[tt.name].Decl.Body.Result.(*ast.Field)
			ann := types.AnnOf(field)
			if ann.Ctor != nil || ann.FieldIndex != tt.index || types.TypeString(ann.Type) != tt.typ {
				t.Fatalf("%s annotation = %#v, want tuple field %d: %s", tt.name, ann, tt.index, tt.typ)
			}
		}
		inner := p.Funcs["chained"].Decl.Body.Result.(*ast.Field).X.(*ast.Field)
		if ann := types.AnnOf(inner); ann.Ctor != nil || ann.FieldIndex != 1 || types.TypeString(ann.Type) != "Tuple2[String, Bool]" {
			t.Fatalf("inner field annotation = %#v", ann)
		}
		if len(p.Warnings) != 0 {
			t.Fatalf("unexpected warnings: %v", p.Warnings)
		}
		walkExprs(reflect.ValueOf(files), func(e ast.Expr) { assertZonked(t, e) })
	}
}

func TestTupleFieldLambdaArgumentContext(t *testing.T) {
	files := parseFiles(t, `
instance Functor[List] {
 fmap(xs, f) { match xs { Nil => Nil; Cons(x, rest) => Cons(f(x), fmap(rest, f)) } }
}
func firsts(ps List[(Int, String)]) List[Int] { fmap(ps, func(p) => p.0) }
`+pass5Main)
	p, err := checkFixture(t, []*ast.Module{{Files: files}})
	if err != nil {
		t.Fatal(err)
	}
	call := p.Funcs["firsts"].Decl.Body.Result.(*ast.Call)
	lambda := call.Args[1].(*ast.Lambda)
	ft := types.AnnOf(lambda).Type.(*types.TFunc)
	if len(ft.Params) != 1 || types.TypeString(ft.Params[0]) != "Tuple2[Int, String]" || ft.Ret != types.Int {
		t.Fatalf("lambda type = %s", types.TypeString(ft))
	}
	field := lambda.Body.(*ast.Field)
	if ann := types.AnnOf(field); ann.Ctor != nil || ann.FieldIndex != 0 || ann.Type != types.Int {
		t.Fatalf("lambda field annotation = %#v", ann)
	}
	walkExprs(reflect.ValueOf(files), func(e ast.Expr) { assertZonked(t, e) })
}

func TestTupleFieldErrors(t *testing.T) {
	for _, tt := range []struct{ source, want string }{
		{`func value(p (Int, String)) Int { p.2 }`, "tuple index 2 out of range for (Int, String)"},
		{`func value[A, B](p (A, B)) A { p.2 }`, "tuple index 2 out of range for (A, B)"},
		{`func value(p (Int, (String, Bool))) Int { p.1.2 }`, "tuple index 2 out of range for (String, Bool)"},
		{`func value(p Int) Int { p.0 }`, "tuple index requires a tuple type, got Int"},
		{`type Point = Point{x: Int}; func value(p Point) Int { p.0 }`, "tuple index requires a tuple type, got Point"},
		{`func value(p List[Int]) Int { p.0 }`, "tuple index requires a tuple type, got List[Int]"},
		{`func value[A](p A) Int { p.0 }`, "tuple index requires a tuple type, got A"},
		{`func value(p (Int, String)) Int { p.0.0 }`, "tuple index requires a tuple type, got Int"},
		{`func value() Unit { let get = func(x) => x.0; unit() }`, "cannot infer tuple type of x.0; annotate"},
		{`func value() Unit { let get = func(x) => { match x { (_, _) => unit() }; x.1.0 }; unit() }`, "cannot infer tuple type of x.1.0; annotate"},
	} {
		t.Run(tt.source, func(t *testing.T) {
			p, err := checkFixture(t, []*ast.Module{{Files: parseFiles(t, tt.source+pass5Main)}})
			var typed *types.Error
			if p != nil || !errors.As(err, &typed) || typed.Msg != tt.want || typed.Pos.Line == 0 || typed.Pos.Col == 0 {
				t.Fatalf("Check = (%v, %v), want positioned %q", p, err, tt.want)
			}
		})
	}
}

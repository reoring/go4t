package types_test

import (
	"testing"

	"gofx/ast"
	"gofx/types"
)

func TestDictionaryRigidIdentityAnnotations(t *testing.T) {
	files := parseFiles(t, `
type Pair[A] = Pair(A, A)
class Render[X] { render[A Show + Ord](pair X, other A) String }
instance Render[Pair[A]] given Show[A], Ord[A] {
  render(pair, other) {
    match pair {
      Pair(x, y) => if x == y {
        show(x) + show(other)
      } else {
        if other == other { show(x) + show(other) } else { "" }
      }
    }
  }
}
`+pass5Main)
	for range 2 {
		p, err := types.Check([]*ast.Module{{Files: files}})
		if err != nil {
			t.Fatal(err)
		}
		instance := p.Instances[len(p.Instances)-1]
		contextParam := instance.Params[0]
		methodParam := instance.Class.Methods[0].Scheme.TParams[0]
		if contextParam == methodParam || contextParam.Name != methodParam.Name {
			t.Fatal("test requires distinct rigid parameters sharing a source name")
		}
		uses := make(map[*types.TVar]int)
		ast.Walk(instance.Decl, func(node ast.Node) {
			var dict types.DictRef
			var argument ast.Expr
			switch node := node.(type) {
			case *ast.Call:
				dict = types.AnnOf(node.Fn).Dicts[0]
				argument = node.Args[0]
			case *ast.Binary:
				if node.Op != "==" {
					return
				}
				dict = types.AnnOf(node).Dicts[0]
				argument = node.L
			default:
				return
			}
			for dict.Kind == types.DictSuper {
				dict = *dict.Super
			}
			want := contextParam
			if argument.(*ast.Ident).Name == "other" {
				want = methodParam
			}
			if dict.Kind != types.DictParam || dict.Param != want {
				t.Fatalf("dictionary lost rigid identity: %#v, want %p", dict, want)
			}
			uses[want]++
		})
		if uses[contextParam] != 3 || uses[methodParam] != 3 {
			t.Fatalf("dictionary use counts = %v, want three per rigid parameter", uses)
		}
	}
}

package format_test

import (
	"testing"

	"github.com/reoring/go4t/format"
)

func TestProjectionGolden(t *testing.T) {
	for _, tc := range []struct{ source, want string }{
		{"(_ . qty)", "(_.qty)"},
		{"(_ . 0)", "(_.0)"},
		{"(_ . a . b)", "(_.a.b)"},
		{"map(xs,_ . qty)", "map(xs, _.qty)"},
		{"map(xs,_ . 0)", "map(xs, _.0)"},
		{"map(xs,_ . a . 0 . b)", "map(xs, _.a.0.b)"},
		{"map(xs,_ . 0 . 1)", "map(xs, _.0.1)"},
		{"xs|>map(_.qty)", "xs |> map(_.qty)"},
		{"f(_,_.a.b,g(_.0),(_.qty))", "f(_, _.a.b, g(_.0), (_.qty))"},
		{"_ . qty", "_.qty"},
		{"if true{_.qty}else{_.other}", "if true {\n    _.qty\n  } else {\n    _.other\n  }"},
	} {
		t.Run(tc.source, func(t *testing.T) {
			source := "package main\nfunc f() T {" + tc.source + "}"
			want := "package main\n\nfunc f() T {\n  " + tc.want + "\n}\n"
			got, err := format.Source([]byte(source))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, want)
			}
			assertProperties(t, source)
		})
	}
}

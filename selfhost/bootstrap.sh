#!/usr/bin/env bash
# Bootstrap the self-hosted gofx compiler and check the fixed point.
#   stage1: selfhost/*.gfx compiled by the Go compiler (cmd/gofx)
#   stage2: selfhost/*.gfx compiled by stage1
#   stage3: selfhost/*.gfx compiled by stage2
# Pass criteria: every example prints the same stdout under stage1 as under
# cmd/gofx, and gofmt(stage2 source) == gofmt(stage3 source).
set -euo pipefail
cd "$(dirname "$0")/.."

SRC=(selfhost/base.gfx selfhost/ast.gfx selfhost/ty.gfx selfhost/core.gfx selfhost/check.gfx
     selfhost/lexer.gfx selfhost/parser.gfx selfhost/types.gfx selfhost/decls.gfx selfhost/infer.gfx
     selfhost/cg_expr.gfx selfhost/cg_decls.gfx selfhost/main.gfx)
OUT=.tmp/bootstrap
rm -rf "$OUT"
mkdir -p "$OUT/stage1" "$OUT/stage2" "$OUT/stage3" "$OUT/examples"

echo "== stage1 (Go compiler -> selfhost)"
go run ./cmd/gofx build "${SRC[@]}"
cp selfhost/base_gen/main.go "$OUT/stage1/main.go"
go build -o "$OUT/stage1/selfhost" ./selfhost/base_gen

echo "== examples via stage1"
status=0
for dir in examples/*/; do
  name=$(basename "$dir")
  src="${dir}main.gfx"
  [ -f "$src" ] || continue
  gen="$OUT/examples/${name}_gen"
  mkdir -p "$gen"
  if ! "$OUT/stage1/selfhost" "$src" -o "$gen/main.go" 2>"$OUT/examples/$name.err"; then
    echo "FAIL compile $name: $(head -1 "$OUT/examples/$name.err")"; status=1; continue
  fi
  if ! go build -o "$gen/prog" "./$gen" 2>"$OUT/examples/$name.goerr"; then
    echo "FAIL go build $name: $(head -3 "$OUT/examples/$name.goerr")"; status=1; continue
  fi
  expected=$(go run ./cmd/gofx run "$src" </dev/null 2>/dev/null || true)
  actual=$("$gen/prog" </dev/null 2>/dev/null || true)
  if [ "$expected" == "$actual" ]; then echo "ok   $name"; else
    echo "FAIL output $name"; { diff <(echo "$expected") <(echo "$actual") || true; } | head -10; status=1
  fi
done

echo "== stage2 (stage1 -> selfhost)"
"$OUT/stage1/selfhost" "${SRC[@]}" -o "$OUT/stage2/main.go"
go build -o "$OUT/stage2/selfhost" "./$OUT/stage2"

echo "== stage3 (stage2 -> selfhost)"
"$OUT/stage2/selfhost" "${SRC[@]}" -o "$OUT/stage3/main.go"

if diff <(gofmt "$OUT/stage2/main.go") <(gofmt "$OUT/stage3/main.go") >"$OUT/fixpoint.diff"; then
  echo "FIXED POINT: stage2 == stage3"
else
  echo "FAIL fixed point; see $OUT/fixpoint.diff"; status=1
fi
exit $status

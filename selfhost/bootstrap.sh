#!/usr/bin/env bash
# Bootstrap the self-hosted compiler; stdout and successful exits are both contracts.
set -euo pipefail
cd "$(dirname "$0")/.."

SRC=(selfhost/base.gt selfhost/ast.gt selfhost/ty.gt selfhost/core.gt selfhost/check.gt
     selfhost/lexer.gt selfhost/parser.gt selfhost/modules.gt selfhost/types.gt selfhost/decls.gt selfhost/derive_json.gt selfhost/derive_higher.gt selfhost/infer.gt
     selfhost/cg_expr.gt selfhost/cg_decls.gt selfhost/main.gt)
OUT=.tmp/bootstrap
mkdir -p "$OUT/stage1" "$OUT/stage2" "$OUT/stage3" "$OUT/examples"

# Explicit exclusions: these are not silently inferred from compilation failures.
declare -A SKIP=(
  [deps]='requires Go module dependency resolution; entry is app/main.gt'
  [go-import]='imports go:io, go:net/http, and go:strings'
  [node-drain-typed]='imports Kubernetes Go packages and requires a live cluster'
  [sql-service]='imports go:database/sql and service glue; starts a network server'
  [typed-extern]='methods.gt imports go:fmt and requires typed Go adapters'
  [http]='starts a TCP listener and makes HTTP requests'
  [http-service]='starts two TCP listeners and makes HTTP requests'
  [sales-service]='starts a TCP listener and makes HTTP requests'
  [go-chan]='imports Go channel fixtures through import-time binding'
  [go-generics]='imports generic Go functions through import-time binding'
)

# Classification is explicit; coincident samples cannot establish determinism.
declare -A NONDETERMINISTIC=(
  [concurrency]=1 # Prints elapsed milliseconds.
  [sales-report]=1 # randomInt controls EUR failure; also prints elapsed milliseconds.
  [sales-service]=1 # Prints "(rates Nms)"; currently excluded by the network skip.
)

# Heavy Go invocations are serialized on development VMs; CI can omit the runner.
RUNNER=/home/vagrant/.codex/skills/serialize-vm-tests/scripts/serialize-vm-tests
run_go() {
  if [[ -x "$RUNNER" ]]; then "$RUNNER" --class short -- "$@"; else "$@"; fi
}
report_error() {
  local line count=0
  while IFS= read -r line && (( count < 3 )); do
    printf '  %s\n' "$line"
    count=$((count + 1))
  done < "$1"
}

echo '== stage1 (Go compiler -> selfhost)'
run_go go build -o "$OUT/go4t" ./cmd/go4t
run_go "$OUT/go4t" build --emit-go selfhost/base_gen "${SRC[@]}"
run_go go build -o "$OUT/stage1/selfhost" ./selfhost/base_gen

echo '== examples via stage1'
status=0
for dir in examples/*/; do
  name=${dir%/}; name=${name##*/}
  if [[ -n "${SKIP[$name]:-}" ]]; then
    printf 'skip %s: %s\n' "$name" "${SKIP[$name]}"
    continue
  fi
  [[ -f "${dir}main.gt" ]] || continue
  sources=("${dir}main.gt")
  [[ ! -f "${dir}gostd.gt" ]] || sources+=("${dir}gostd.gt")
  # The CI rule counts files defining main, not all .gt files. This excludes
  # diagnostic and panic entrypoints while retaining directory helper files.
  main_files=0
  for file in "$dir"*.gt; do
    while IFS= read -r line; do
      if [[ "$line" == 'func main'* ]]; then main_files=$((main_files + 1)); break; fi
    done < "$file"
  done
  if (( main_files == 1 )); then sources=("${dir%/}"); fi
  args=()
  if [[ -f "${dir}args.txt" ]]; then read -r -d '' -a args < "${dir}args.txt" || true; fi
  input=/dev/null
  [[ ! -f "${dir}input.txt" ]] || input="${dir}input.txt"
  gen="$OUT/examples/${name}_gen"
  mkdir -p "$gen"
  if ! timeout 120 "$OUT/stage1/selfhost" "${sources[@]}" -o "$gen/main.go" 2>"$OUT/examples/$name.err"; then
    echo "FAIL compile $name"; report_error "$OUT/examples/$name.err"; status=1; continue
  fi
  if ! run_go go build -o "$gen/prog" "./$gen" 2>"$OUT/examples/$name.goerr"; then
    echo "FAIL go build $name"; report_error "$OUT/examples/$name.goerr"; status=1; continue
  fi
  if ! run_go bash -c 'input=$1; shift; exec "$@" < "$input"' bash "$input" timeout 120 "$OUT/go4t" run "${sources[@]}" -- "${args[@]}" >"$OUT/examples/$name.expected" 2>"$OUT/examples/$name.reference.err"; then
    echo "FAIL reference $name"; report_error "$OUT/examples/$name.reference.err"; status=1; continue
  fi
  if ! timeout 120 "$gen/prog" "${args[@]}" < "$input" >"$OUT/examples/$name.actual" 2>"$OUT/examples/$name.run.err"; then
    echo "FAIL run $name"; report_error "$OUT/examples/$name.run.err"; status=1; continue
  fi
  mode=deterministic
  [[ -z "${NONDETERMINISTIC[$name]:-}" ]] || mode=nondeterministic
  if ! python3 selfhost/dev/compare_stdout.py "$name" "$mode" "$OUT/examples/$name.expected" "$OUT/examples/$name.actual"; then status=1; fi
done

echo '== stage2 (stage1 -> selfhost)'
if ! timeout 180 "$OUT/stage1/selfhost" "${SRC[@]}" -o "$OUT/stage2/main.go"; then
  echo 'FAIL compile stage2'; exit 1
fi
if ! run_go go build -o "$OUT/stage2/selfhost" "./$OUT/stage2"; then
  echo 'FAIL go build stage2'; exit 1
fi

echo '== stage3 (stage2 -> selfhost)'
if ! timeout 180 "$OUT/stage2/selfhost" "${SRC[@]}" -o "$OUT/stage3/main.go"; then
  echo 'FAIL compile stage3'; exit 1
fi
if diff <(gofmt "$OUT/stage2/main.go") <(gofmt "$OUT/stage3/main.go") >"$OUT/fixpoint.diff"; then
  echo 'FIXED POINT: stage2 == stage3'
else
  echo "FAIL fixed point; see $OUT/fixpoint.diff"; status=1
fi
exit "$status"

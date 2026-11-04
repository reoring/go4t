#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)
cd "$ROOT"
OUT=/tmp/go4t-soak
KUBECONFIG=/tmp/go4t-soak.kubeconfig
export KUBECONFIG
HOURS=${SOAK_HOURS:-3}
SERIALIZER=/home/vagrant/.codex/skills/serialize-vm-tests/scripts/serialize-vm-tests
mkdir -p "$OUT"
exec 9>"$OUT/driver.lock"
flock -n 9 || { echo 'Another soak driver is running' >&2; exit 1; }
python3 -c 'import sys; h=float(sys.argv[1]); assert 0 < h < float("inf"), "SOAK_HOURS must be positive and finite"' "$HOURS"
for cmd in kind kubectl docker go python3 /usr/bin/time "$SERIALIZER"; do
  command -v "$cmd" >/dev/null
done
printf '%s\n' "Soak startup: $(date -Is), PID $$"
printf '%s\n' "$$" > "$OUT/driver.pid"
kind get clusters > "$OUT/clusters.before.txt"
cat > "$OUT/kind.yaml" <<'YAML'
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
- role: worker
YAML
found=false
while IFS= read -r cluster; do
  if [[ "$cluster" == go4t-soak ]]; then found=true; fi
done < "$OUT/clusters.before.txt"
if [[ "$found" == false ]]; then
  kind create cluster --name go4t-soak --config "$OUT/kind.yaml" --kubeconfig "$KUBECONFIG" --wait 180s
fi
kind get kubeconfig --name go4t-soak > "$KUBECONFIG"
chmod 600 "$KUBECONFIG"
K=(kubectl --kubeconfig /tmp/go4t-soak.kubeconfig)
"${K[@]}" get nodes -o wide > "$OUT/nodes.before.txt"
"${K[@]}" get node go4t-soak-control-plane go4t-soak-worker
"${K[@]}" wait --for=condition=Ready node/go4t-soak-control-plane node/go4t-soak-worker --timeout=180s
cat > "$OUT/workload.yaml" <<'YAML'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: go4t-soak-nginx
  namespace: default
spec:
  replicas: 2
  selector:
    matchLabels:
      app: go4t-soak-nginx
  template:
    metadata:
      labels:
        app: go4t-soak-nginx
    spec:
      terminationGracePeriodSeconds: 5
      tolerations:
      - key: node-role.kubernetes.io/control-plane
        operator: Exists
        effect: NoSchedule
      affinity:
        nodeAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            preference:
              matchExpressions:
              - key: kubernetes.io/hostname
                operator: In
                values: [go4t-soak-worker]
      containers:
      - name: nginx
        image: nginx:1.27-alpine
        readinessProbe:
          httpGet:
            path: /
            port: 80
          periodSeconds: 1
---
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: go4t-soak-nginx
  namespace: default
spec:
  minAvailable: 1
  selector:
    matchLabels:
      app: go4t-soak-nginx
YAML
"${K[@]}" apply -f "$OUT/workload.yaml"
"${K[@]}" rollout status deployment/go4t-soak-nginx --timeout=180s
kind get clusters > "$OUT/clusters.after.txt"
"${K[@]}" get nodes,pdb,pods -o wide > "$OUT/setup.after.txt"
printf '%s\n' 'Build started'
SERIALIZE_VM_TESTS_TARGET=local "$SERIALIZER" --class short -- go run ./cmd/go4t build -o /tmp/go4t-drain examples/node-drain-typed
printf '%s\n' "Build complete: $(date -Is)" | tee "$OUT/build.complete"
# A new invocation owns a fresh result set; refuse to overwrite an earlier run.
[[ ! -e "$OUT/log.tsv" ]] || { echo "Archive $OUT/log.tsv before restarting" >&2; exit 1; }
printf 'cycle\titerations_to_drained\twall_sum_s\tmax_rss_kib\texit_codes\tdrained\trecovery_exit\n' > "$OUT/log.tsv"
: > "$OUT/errors.txt"
: > "$OUT/stderr.jsonl"

summarize() {
  python3 - "$OUT" <<'PY'
import collections, csv, json, pathlib, re, statistics, sys
p = pathlib.Path(sys.argv[1])
rows = list(csv.DictReader((p / 'log.tsv').open(), delimiter='\t'))
counts = collections.Counter()
kinds = collections.Counter()
for line in (p / 'stderr.jsonl').open():
    text = json.loads(line)
    matches = re.findall(r'\b(NotFound|PermissionDenied|Timeout|Cancelled|Network|Failure|Panic)\b', text)
    labels = list(dict.fromkeys(matches))
    if text.startswith('blocked:'):
        labels.append('Blocked')
    if 'TooManyRequests:' in text:
        labels.append('TooManyRequests:')
    kind = ','.join(labels) or 'Unclassified'
    counts[(kind, text)] += 1
    kinds.update(labels or ['Unclassified'])
with (p / 'errors.txt.tmp').open('w') as f:
    f.write('count\tkind\tstderr_line\n')
    for (kind, text), count in sorted(counts.items()):
        f.write(f'{count}\t{kind}\t{json.dumps(text, ensure_ascii=False)}\n')
(p / 'errors.txt.tmp').replace(p / 'errors.txt')
print('cycles\tmean_iterations\tmax_iterations\tmean_rss_kib\tmax_rss_kib')
if rows:
    iterations = [int(r['iterations_to_drained']) for r in rows]
    rss = [int(r['max_rss_kib']) for r in rows]
    print(f'{len(rows)}\t{statistics.mean(iterations):.2f}\t{max(iterations)}\t{statistics.mean(rss):.2f}\t{max(rss)}')
print('error_kind\tcount')
for kind, count in sorted(kinds.items()):
    print(f'{kind}\t{count}')
PY
}
trap summarize EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
SECONDS=0
DURATION=$(python3 -c 'import sys; print(int(float(sys.argv[1])*3600))' "$HOURS")
cycle=0
while (( SECONDS < DURATION )); do
  cycle=$((cycle + 1))
  dir="$OUT/cycle-$cycle"
  mkdir -p "$dir"
  drained=false
  codes=()
  for ((iteration=1; iteration<=60; iteration++)); do
    rc=0
    /usr/bin/time -f '%e %M' -o "$dir/$iteration.time" /tmp/go4t-drain go4t-soak-worker /tmp/go4t-soak.kubeconfig > "$dir/$iteration.stdout" 2> "$dir/$iteration.stderr" || rc=$?
    codes+=("$rc")
    if [[ $(< "$dir/$iteration.stdout") == *MarkDrained* ]]; then
      drained=true
      break
    fi
    if (( iteration < 60 )); then sleep 2; fi
  done
  iterations=$iteration
  (( iterations <= 60 )) || iterations=60
  rc=0
  /usr/bin/time -f '%e %M' -o "$dir/release.time" /tmp/go4t-drain go4t-soak-worker /tmp/go4t-soak.kubeconfig --release > "$dir/release.stdout" 2> "$dir/release.stderr" || rc=$?
  codes+=("release:$rc")
  # Recreate pods after uncordon: otherwise they stay on the control-plane and
  # subsequent drains would exercise only Cordon/MarkDrained, never eviction.
  recovery=0
  "${K[@]}" rollout restart deployment/go4t-soak-nginx > "$dir/recovery.stdout" 2> "$dir/recovery.stderr" &&
    "${K[@]}" rollout status deployment/go4t-soak-nginx --timeout=180s >> "$dir/recovery.stdout" 2>> "$dir/recovery.stderr" &&
    hash=$("${K[@]}" get replicasets -l app=go4t-soak-nginx --sort-by=.metadata.creationTimestamp -o jsonpath='{.items[-1].metadata.labels.pod-template-hash}' 2>> "$dir/recovery.stderr") &&
    [[ -n "$hash" ]] &&
    "${K[@]}" wait --for=jsonpath='{.status.phase}'=Running pod -l "app=go4t-soak-nginx,pod-template-hash=$hash" --timeout=180s >> "$dir/recovery.stdout" 2>> "$dir/recovery.stderr" &&
    "${K[@]}" wait --for=condition=Ready pod -l "app=go4t-soak-nginx,pod-template-hash=$hash" --timeout=180s >> "$dir/recovery.stdout" 2>> "$dir/recovery.stderr" || recovery=$?
  python3 - "$OUT" "$cycle" "$iterations" "$drained" "$recovery" "$(IFS=,; echo "${codes[*]}")" <<'PY'
import json, pathlib, sys
out, cycle, iterations, drained, recovery, codes = sys.argv[1:]
p = pathlib.Path(out)
d = p / f'cycle-{cycle}'
elapsed = 0.0
rss = 0
for f in d.glob('*.time'):
    # GNU time may prepend a diagnostic when its child exits nonzero.
    seconds, peak = f.read_text().splitlines()[-1].split()
    elapsed += float(seconds)
    rss = max(rss, int(peak))
with (p / 'stderr.jsonl').open('a') as dest:
    for f in sorted(d.glob('*.stderr')):
        for line in f.read_text(errors='replace').splitlines():
            dest.write(json.dumps(line) + '\n')
with (p / 'log.tsv').open('a') as dest:
    dest.write(f'{cycle}\t{iterations}\t{elapsed:.2f}\t{rss}\t{codes}\t{drained}\t{recovery}\n')
PY
  summarize
  printf 'Cycle %s complete: drained=%s recovery_exit=%s\n' "$cycle" "$drained" "$recovery"
  # Recovery failure cannot safely start another cycle; preserve its evidence.
  (( recovery == 0 )) || exit "$recovery"
  sleep 2
done

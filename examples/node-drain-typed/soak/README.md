# Typed node-drain soak
Run from the repository root with kind, Docker, kubectl, Go, Python 3, GNU time, and the VM serializer installed.
Start: `mkdir -p /tmp/go4t-soak; SOAK_HOURS=3 nohup bash examples/node-drain-typed/soak/soak.sh > /tmp/go4t-soak/driver.log 2>&1 &`.
Only kind `go4t-soak` is targeted; all Kubernetes calls explicitly use `/tmp/go4t-soak.kubeconfig`, never the default production config.
The cluster has one control-plane and one worker; two nginx replicas prefer the worker, tolerate the control-plane, and share a minAvailable=1 PDB.
The existing example module replacement is required; the controller builds once through the serializer before the timed soak begins (default three hours).
Each cycle reconciles at two-second intervals up to MarkDrained or 60 attempts, releases once, restarts nginx onto the worker, then waits for Running/Ready on the new ReplicaSet only.
`cat /tmp/go4t-soak/log.tsv` shows attempts (60 when exhausted), summed drain/release wall seconds, peak KiB RSS, exit codes, drained flag, and recovery status.
`cat /tmp/go4t-soak/errors.txt` shows counted distinct stderr lines with IoError constructor names, `TooManyRequests:`, and separate `Blocked` labels for caught `blocked:` eviction messages.
Raw cycle stdout/stderr/timing, snapshots, PID, and summary remain under `/tmp/go4t-soak`; stop via `kill -TERM $(cat /tmp/go4t-soak/driver.pid)`, retain the cluster for inspection, and archive results before rerunning.

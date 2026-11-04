#!/usr/bin/env python3
"""Build paired kernels, measure three executions, compare outputs, profile worst ratio."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import shlex
import shutil
import subprocess
import tempfile


ROOT = Path(__file__).resolve().parents[2]
PERF = ROOT / "bench/perf"
KERNELS = ("wordfreq", "sortints", "nbody", "mapheavy", "treefold")
RUNNER = Path(os.environ.get(
    "SERIALIZE_VM_TESTS_RUNNER",
    "/home/vagrant/.codex/skills/serialize-vm-tests/scripts/serialize-vm-tests",
))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("kernels", nargs="*", choices=KERNELS)
    parser.add_argument("--compiler", type=Path, help="use an already-built compiler snapshot")
    parser.add_argument("--work-dir", type=Path, help="new artifact directory beneath bench/perf/.work")
    args = parser.parse_args()
    kernels = args.kernels or list(KERNELS)
    if not RUNNER.is_file() or not os.access(RUNNER, os.X_OK):
        parser.error(f"missing executable serializer: {RUNNER}")
    for program in ("go", "/usr/bin/time", "timeout"):
        if shutil.which(program) is None:
            parser.error(f"missing executable: {program}")
    scratch = PERF / ".work"
    scratch.mkdir(exist_ok=True)
    if args.work_dir:
        work = args.work_dir.resolve()
        if not work.is_relative_to(scratch):
            parser.error(f"--work-dir must be beneath {scratch}")
        work.mkdir(parents=True, exist_ok=False)
    else:
        work = Path(tempfile.mkdtemp(prefix="run-", dir=scratch))
    print(f"ARTIFACTS {work}", flush=True)
    env = dict(os.environ, GOMAXPROCS="1", GOGC="100",
               SERIALIZE_VM_TESTS_QUEUE_TIMEOUT_SECONDS="180",
               SERIALIZE_VM_TESTS_TIMEOUT_SECONDS="180")
    commands = work / "commands.log"

    def command(argv, *, serialized=True, stdin=None, stdout=None, stderr=None, extra_env=None):
        argv = [str(arg) for arg in argv]
        if stdin is not None:
            # The serializer backgrounds its child, which otherwise inherits /dev/null.
            # Open the workload input inside the reservation, before GNU time starts.
            argv = ["bash", "-c", 'input=$1; shift; exec "$@" < "$input"',
                    "go4t-perf-input", str(stdin.name), *argv]
        if serialized:
            argv = [str(RUNNER), "--class", "short", "--", *argv]
        rendered = shlex.join(argv)
        if stdout is not None:
            rendered += " > " + shlex.quote(str(stdout.name))
        if stderr is not None:
            rendered += " 2> " + shlex.quote(str(stderr.name))
        with commands.open("a") as log:
            log.write(rendered + "\n")
        print("+ " + rendered, flush=True)
        result = subprocess.run(argv, cwd=ROOT, env=env | (extra_env or {}),
                                stdin=subprocess.DEVNULL, stdout=stdout, stderr=stderr, check=False)
        print(f"EXIT {result.returncode}", flush=True)
        return result.returncode

    compiler = args.compiler.resolve() if args.compiler else work / "go4t"
    if not args.compiler:
        if command(["go", "build", "-o", compiler, "./cmd/go4t"]):
            raise SystemExit("compiler build failed")
    if not compiler.is_file():
        raise SystemExit(f"compiler does not exist: {compiler}")
    metadata = {
        "platform": platform.platform(), "cpu_count": os.cpu_count(),
        "load_start": os.getloadavg(), "GOMAXPROCS": "1", "GOGC": "100",
        "go_version": subprocess.check_output(["go", "version"], text=True).strip(),
        "compiler_sha256": hashlib.sha256(compiler.read_bytes()).hexdigest(),
        "source_sha256": {}, "input_sha256": {},
    }
    for source in PERF.rglob("*"):
        if source.is_file() and ".work" not in source.parts:
            metadata["source_sha256"][str(source.relative_to(ROOT))] = hashlib.sha256(source.read_bytes()).hexdigest()
    inputs = work / "input"
    if command(["go", "run", "./bench/perf/gen", inputs]):
        raise SystemExit("input generation failed")
    for source in inputs.iterdir():
        metadata["input_sha256"][source.name] = hashlib.sha256(source.read_bytes()).hexdigest()
    (work / "metadata.json").write_text(json.dumps(metadata, indent=2) + "\n")
    results = []
    failed = False
    for kernel in kernels:
        print(f"BUILD {kernel}", flush=True)
        directory = work / kernel
        directory.mkdir()
        generated = directory / "generated"
        # Explicit source emission keeps profiling independent of build -o FILE.
        if command([compiler, "build", "--emit-go", generated, "--package", "main", PERF / kernel / "main.gt"]):
            raise SystemExit(f"{kernel}: go4t emission failed")
        binaries = {"go4t": directory / "go4t-bin", "Go": directory / "go-bin"}
        if command(["go", "build", "-o", binaries["go4t"], generated]):
            raise SystemExit(f"{kernel}: generated Go build failed")
        if command(["go", "build", "-o", binaries["Go"], PERF / kernel / "go"]):
            raise SystemExit(f"{kernel}: Go build failed")
        row = {"kernel": kernel, "runs": {"go4t": [], "Go": []},
               "binary_bytes": {name: binary.stat().st_size for name, binary in binaries.items()}}
        outputs = []
        for attempt in range(1, 4):
            for language, binary in binaries.items():
                stem = directory / f"{language}-{attempt}"
                time_path = stem.with_suffix(".time")
                with (inputs / f"{kernel}.txt").open("rb") as source, stem.with_suffix(".stdout").open("wb") as out, stem.with_suffix(".stderr").open("wb") as err:
                    print(f"RUN {kernel} {language} {attempt}/3", flush=True)
                    status = command(["/usr/bin/time", "-f", "%e %M", "-o", time_path,
                                      "timeout", "--kill-after=5s", "175s", binary],
                                     stdin=source, stdout=out, stderr=err)
                sample = {"exit": status, "stdout": str(stem.with_suffix(".stdout").relative_to(work)),
                          "stderr": str(stem.with_suffix(".stderr").relative_to(work))}
                if time_path.exists():
                    timing = time_path.read_text().splitlines()[-1].split()
                    sample.update(seconds=float(timing[0]), rss_kb=int(timing[1]))
                row["runs"][language].append(sample)
                outputs.append(stem.with_suffix(".stdout").read_bytes())
                print(f"RESULT {kernel} {language} {sample}", flush=True)
        row["checksum_equal"] = bool(outputs[0]) and all(output == outputs[0] for output in outputs)
        row["success"] = all(sample["exit"] == 0 for samples in row["runs"].values() for sample in samples)
        row["checksum_equal"] = row["checksum_equal"] and row["success"]
        row["checksum"] = outputs[0].decode("utf-8", errors="replace")
        row["checksum_sha256"] = hashlib.sha256(outputs[0]).hexdigest()
        for language, samples in row["runs"].items():
            successful = [sample for sample in samples if sample["exit"] == 0]
            row[language] = {
                "seconds": min((sample["seconds"] for sample in successful), default=None),
                "rss_kb": min((sample["rss_kb"] for sample in successful), default=None),
            }
        go_seconds = row["Go"]["seconds"]
        row["ratio"] = row["go4t"]["seconds"] / go_seconds if row["success"] and go_seconds else None
        results.append(row)
        failed = failed or not row["checksum_equal"]
        (work / "results.json").write_text(json.dumps(results, indent=2) + "\n")

    table = ["| Kernel | go4t wall s | Go wall s | Ratio | go4t RSS KB | Go RSS KB | Checksum equal | go4t binary bytes | Go binary bytes |",
             "| --- | ---: | ---: | ---: | ---: | ---: | :---: | ---: | ---: |"]
    for row in results:
        seconds = lambda value: f"{value:.2f}" if value is not None else "failed"
        ratio = f"{row['ratio']:.2f}x" if row["ratio"] is not None else "n/a"
        table.append(f"| {row['kernel']} | {seconds(row['go4t']['seconds'])} | {seconds(row['Go']['seconds'])} | {ratio} | {row['go4t']['rss_kb']} | {row['Go']['rss_kb']} | {'yes' if row['checksum_equal'] else 'NO'} | {row['binary_bytes']['go4t']} | {row['binary_bytes']['Go']} |")
    rendered_table = "\n".join(table) + "\n"
    print(rendered_table, flush=True)
    (work / "table.md").write_text(rendered_table)

    candidates = [row for row in results if row["ratio"] is not None and row["checksum_equal"]]
    if candidates:
        worst = max(candidates, key=lambda row: row["ratio"])
        kernel = worst["kernel"]
        print(f"PROFILE {kernel} ({worst['ratio']:.2f}x)", flush=True)
        directory = work / kernel
        profiled = directory / "profile-generated"
        shutil.copytree(directory / "generated", profiled)
        mains = [source for source in profiled.glob("*.go") if "func main() {" in source.read_text()]
        if len(mains) != 1:
            raise SystemExit(f"expected one generated main, got {len(mains)}")
        main_source = mains[0].read_text()
        if main_source.count("func main() {") != 1:
            raise SystemExit("ambiguous generated main for profiling")
        mains[0].write_text(main_source.replace("func main() {", "func main() {\n\tdefer perfProfile()()\n", 1))
        shutil.copyfile(PERF / "profile.go.txt", profiled / "perf_profile.go")
        binary = directory / "profile-bin"
        profile = directory / "cpu.pprof"
        if command(["go", "build", "-o", binary, profiled]):
            raise SystemExit("profile build failed")
        with (inputs / f"{kernel}.txt").open("rb") as source, (directory / "profile.stdout").open("wb") as out:
            status = command(["timeout", "--kill-after=5s", "175s", binary], stdin=source, stdout=out,
                             extra_env={"GO4T_PERF_CPU_PROFILE": str(profile)})
        if status or (directory / "profile.stdout").read_text() != worst["checksum"]:
            raise SystemExit("profile run failed or changed checksum")
        with (work / "pprof-top.txt").open("w") as out:
            if command(["go", "tool", "pprof", "-top", "-nodecount=10", binary, profile], stdout=out):
                raise SystemExit("pprof failed")
        print((work / "pprof-top.txt").read_text(), flush=True)
        metadata["profile_kernel"] = kernel
    metadata["load_end"] = os.getloadavg()
    (work / "metadata.json").write_text(json.dumps(metadata, indent=2) + "\n")
    print(f"ARTIFACTS {work}", flush=True)
    if failed:
        raise SystemExit("kernel failed or checksums differed; see preserved stdout/stderr")


if __name__ == "__main__":
    main()

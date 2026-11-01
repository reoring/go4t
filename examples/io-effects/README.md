# IO effects: configuration, validation, reports

Run from the repository root. The first argument is a configuration file containing
exactly two positive integers. The optional second argument is an output prefix;
the program writes `<prefix>.report.txt` and appends to `<prefix>.log`.
Without arguments the configuration path is `examples/io-effects/missing.conf`
and the output prefix is `io-effects`.

A failed `tryIO(readFile(path))` becomes `Err(e)` with an `io.IoError` (a missing
file is `io.NotFound(message)`), so the program falls back to `3 4`.
`io.ioErrorMessage(e)` or `show(e)` retrieves the message when needed.
A successful read uses the file as-is. The stdout/report formats below are unchanged.
Pure validation uses `do` over
`Result[String]`: parsing and positivity checks stop at the first `Err`. The
`bad also-bad` demonstration reports the first parse failure, never the second.
Each report is overwritten; each log entry uses `formatTime(nowMillis(), ...)`
via an IO bind, with a UTC timestamp.

## Missing file

```sh
out=$(mktemp -d /tmp/go4t-io-effects.XXXXXX)
go run ./cmd/go4t run examples/io-effects/main.gt -- examples/io-effects/missing.conf "$out/missing"
```

Observed stdout (exit 0):

```text
config: defaults (read failed)
validation: Ok(7)
early error: Err(first: expected an integer)
wrote report and appended timestamped log
```

Observed `missing.report.txt`:

```text
sum: Ok(7)
```

Observed `missing.log` (the timestamp changes on each run):

```text
2026-11-01T06:27:17.668Z sum: Ok(7)
```

## Present file

The checked-in `config.txt` contains `7 11`.

```sh
go run ./cmd/go4t run examples/io-effects/main.gt -- examples/io-effects/config.txt "$out/present"
```

Observed stdout (exit 0):

```text
config: file
validation: Ok(18)
early error: Err(first: expected an integer)
wrote report and appended timestamped log
```

Observed `present.report.txt`:

```text
sum: Ok(18)
```

Observed `present.log`:

```text
2026-11-01T06:27:20.817Z sum: Ok(18)
```

The recorded runs used `/tmp/go4t-io-effects.i6winO` for `$out`, under the
VM-wide serialized test runner. Remove the temporary output directory when done:

```sh
rm -r "$out"
```

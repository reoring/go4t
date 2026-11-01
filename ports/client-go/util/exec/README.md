# Native exec errors

The pinned package exposes ExitError and CodeExitError (it does not expose command execution or an error-chain classifier). `CodeExitError {err: support.Error, code: Int}` retains the original canonical error handle and exact signed exit code. `error` and `string` forward to that error, `exited` is always true, and `exitStatus` returns code without normalization. `asExitError` adapts to an explicit callback capability. No production client-go imports; the host helper only invokes the external error interface. No checks run in this worker.

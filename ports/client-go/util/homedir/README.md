# Native home directory selection

`homeDir()` uses OS/environment/stat/path-join primitives. `homeDirWith(Host)` exposes the same platform policy for deterministic tests. Every non-Windows OS returns HOME directly, with no user-database fallback.

Windows searches HOME, HOMEDRIVE+HOMEPATH, USERPROFILE for a stat-able `.kube/config` path (a directory at that path also qualifies upstream). Both HOMEDRIVE and HOMEPATH must be nonempty. If no config exists, search HOME, USERPROFILE, drive+path for a directory whose permission bits include owner write (0200); this is the upstream bit test, not an access/ACL check. Then use first existing path, then first set path, else empty. Stat failures are skipped.

Prepared behavioral program `util/homedir/regression` covers all precedence tiers, Unix behavior, and the distinction between existing files and writable directories. It is not yet run; primary should check/execute it and exercise the real Windows host primitive when available.

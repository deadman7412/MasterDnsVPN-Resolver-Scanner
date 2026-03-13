# Logging

Session logging is off by default. Enable it with `--log`.

---

## Enabling Logs

**Auto-named file** — generates a timestamped filename in the current directory:

```
scanner --mode quick --log auto
```

Creates: `scanner-2026-03-13T15-04-05.log`

**Explicit path:**

```
scanner --mode quick --log /tmp/scan.log
scanner --mode full  --log ./results/run1.log
```

---

## What Gets Logged

Everything printed to stdout is also written to the log file.
The log file adds a millisecond-precision timestamp to every line.

**Stdout (no timestamps):**
```
[info] mode: quick  estimated pool: 7842 IPs
[ok]   8.8.8.8 -- stage 1 pass, RTT: 12ms
[fail] 1.2.3.4 -- stage 2 timeout
[warn] some resolvers skipped due to early exit
```

**Log file (with timestamps):**
```
------------------------------------------------------------
MasterDnsVPN Resolver Scanner
session started: 2026-03-13 15:04:05
  mode:      quick
  min:       20
  domain:    -
  key:       -
  method:    0
  inputs:    0 extra
------------------------------------------------------------
2026-03-13 15:04:05.001 [info   ] logging to scanner-2026-03-13T15-04-05.log
2026-03-13 15:04:05.002 [info   ] mode: quick  estimated pool: 7842 IPs
2026-03-13 15:04:05.430 [ok     ] 8.8.8.8 -- stage 1 pass, RTT: 12ms
2026-03-13 15:04:05.431 [fail   ] 1.2.3.4 -- stage 2 timeout
2026-03-13 15:04:06.190 [warn   ] some resolvers skipped due to early exit
------------------------------------------------------------
session ended: 2026-03-13 15:04:06
------------------------------------------------------------
```

The session header records the flags used so you always know what produced a log.

---

## Log Tags

| Tag | Meaning |
|---|---|
| `[info]` | General status update |
| `[ok]` | Per-item pass (resolver passed a stage) |
| `[success]` | Top-level success (scan complete, resolver fully validated) |
| `[warn]` | Non-fatal warning |
| `[error]` | Error (scan continues) |
| `[fail]` | Per-item failure (resolver failed a stage) |
| `[skip]` | Item not tested (e.g. early exit) |

---

## Tips

**Run multiple scans, keep all logs:**
```
scanner --mode range --input 185.51.0.0/16 --log auto
scanner --mode range --input 2.144.x.x     --log auto
```
Each gets its own timestamped file.

**Pipe stdout while also saving to a file:**
```
scanner --mode quick --log scan.log | grep "\[ok\]"
```
Stdout flows normally; the full log is in `scan.log`.

**Check what a log file contains:**
```
head -10 scanner-2026-03-13T15-04-05.log   # session header
grep "\[fail\]" scan.log                   # all failures
grep "\[ok\]"   scan.log                   # all passes
```

**The log file is append-safe** — if you point two runs at the same file each
session gets its own header/footer block, so nothing is overwritten.

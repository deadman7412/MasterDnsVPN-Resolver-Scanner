# CLI Usage — MasterDnsVPN Resolver Scanner

Finds working DNS resolvers for MasterDnsVPN in censored environments.
Tests each IP through a 7-stage pipeline and scores survivors by MTU and latency.

---

## Quick Start

```
scanner --mode quick
```

Scans the bundled curated resolver list (~7,800 IPs). Stops as soon as 20
valid resolvers are found. No configuration required.

---

## Build

Quick build for the current machine:

```bash
cd cli/
go build -o scanner .
```

Release build for all platforms (cleans assets first):

```bash
./scripts/build.sh
```

See `docs/build.md` for full build instructions, cross-compilation targets,
asset cleaning, and how to update the bundled resolver data.

---

## Flags

### --mode `quick | range | full`

Controls which IP sources are used and when to stop.

| Mode | Source | Stops when |
|---|---|---|
| `quick` | Bundled curated list (~7,800 pre-verified IPs) | `--min` reached |
| `range` | Bundled CIDR ranges (~10.8M IPs) + any `--input` | `--min` reached |
| `full` | All sources combined | Every IP exhausted |

Default: `quick`

Full scan asks for confirmation before starting due to the large IP count.

---

### --input `value`

Add IPs to scan. Works with any mode. Accepts:

| Format | Example |
|---|---|
| Single IP | `8.8.8.8` |
| CIDR range | `185.51.0.0/16` |
| Wildcard | `2.144.x.x` — normalized to `2.144.0.0/16` |
| Wildcard + prefix | `2.144.x.x/24` — normalized to `2.144.0.0/24` |
| File path | `resolvers.txt` — one entry per line, comments with `#` |

Repeatable. Also accepts comma-separated values in a single flag:

```
--input 8.8.8.8 --input 1.1.1.0/24
--input "8.8.8.8,1.1.1.0/24,2.144.x.x"
```

In `range` mode, `--input` entries are scanned in addition to the bundled CIDR ranges.
In `quick` mode, `--input` entries are ignored (only the curated list is used).
In `full` mode, `--input` entries are added on top of everything else.

---

### --min `N`

Stop after finding N valid resolvers. Applies to `quick` and `range` modes.
Has no effect in `full` mode (full always exhausts every IP).

Default: `20`

```
scanner --mode range --min 50
```

---

### --domain `string`

The MasterDnsVPN domain name. Required for stages 5 and above.

```
scanner --mode quick --domain vpn.example.com
```

---

### --key `string`

The encryption key. Required for stages 6 and 7.

```
scanner --mode quick --domain vpn.example.com --key mysecretkey
```

---

### --method `0-5`

Encryption method to use when testing stages 6-7.

| Value | Cipher | Overhead |
|---|---|---|
| `0` | None (plaintext) | 0 bytes |
| `1` | XOR | 0 bytes |
| `2` | ChaCha20-Poly1305 | 16 bytes |
| `3` | AES-128-GCM | 28 bytes |
| `4` | AES-192-GCM | 28 bytes |
| `5` | AES-256-GCM | 28 bytes |

Default: `0`

Must match the encryption method configured on the MasterDnsVPN server.

---

### --log `path`

Save the session log to a file. Disabled by default.

| Value | Behavior |
|---|---|
| not set | no file, stdout only |
| `auto` | creates `scanner-YYYY-MM-DDTHH-MM-SS.log` in current directory |
| any path | logs to that file |

Stdout output is unchanged — no timestamps added, same format as always.
The log file gets a millisecond-precision timestamp on every line plus a
session header and footer recording the flags used.

```
scanner --mode quick --log auto
scanner --mode full  --log /tmp/scan.log
```

See `docs/logging.md` for log format details, tag meanings, and debug tips.

---

## Pipeline Stages

Resolvers are tested in order. Failure at any stage drops the resolver immediately.
Stages run in parallel up to the concurrency limit for each stage.

| Stage | Test | Requires | Concurrency |
|---|---|---|---|
| 1 | UDP port 53 reachability | nothing | 750 |
| 2 | Valid DNS response | nothing | 750 |
| 3 | Poisoning / hijack detection | nothing | 350 |
| 4 | TXT record support | nothing | 350 |
| 5 | VPN domain NS resolution | `--domain` | 150 |
| 6 | MTU binary search | `--domain` + `--key` | 35 |
| 7 | Full E2E session handshake | `--domain` + `--key` | 7 |

Stages 1-4 work with no configuration and produce a filtered list of clean,
unpoisoned DNS resolvers. This is immediately useful without a VPN server.

Stages 5-7 require a live MasterDnsVPN server.

---

## CIDR Scanning Strategy

CIDR ranges are not scanned linearly. A shuffle-within-step-window strategy
distributes probes across the range before going deep into any subnet:

```
Range: 185.51.0.0/16 -> 65,536 IPs

Window: 1,000 IPs
Round 1: shuffle IPs 0-999    -> scan -> 12 valid found
Round 2: shuffle IPs 1000-1999 -> scan -> 8 valid found
...
quick/range: stop when --min reached
full: continue until all 65,536 IPs exhausted
```

This means a /16 range does not always take hours — if resolvers are dense,
`quick`/`range` mode finds them fast and stops. `full` uses the same shuffle
but never stops early.

---

## Output

Each valid resolver produces a result line:

```
[ok] 185.x.x.x -- upload MTU: 210, download MTU: 180, RTT: 42ms, score: 94/100
```

Final output is sorted by score and ready to paste into `RESOLVER_DNS_SERVERS`
in the MasterDnsVPN client config.

Scoring weights:

| Metric | Weight |
|---|---|
| Upload MTU | 40% |
| Download MTU | 40% |
| Average RTT | 20% |

---

## Examples

Quickest possible scan — just find some working resolvers:

```
scanner --mode quick
```

Scan a specific subnet:

```
scanner --mode range --input 185.51.0.0/16
```

Scan with wildcard notation, stop at 50 valid resolvers:

```
scanner --mode range --input 2.144.x.x --min 50
```

Scan your own list file:

```
scanner --mode range --input /path/to/mylist.txt
```

Full pipeline including VPN stages (requires live server):

```
scanner --mode quick --domain vpn.example.com --key mysecretkey --method 2
```

Full scan — exhausts everything (will ask for confirmation):

```
scanner --mode full --domain vpn.example.com --key mysecretkey --method 2
```

---

## Bundled Data

The binary embeds two data files from `cli/assets/`:

| File | Content | Used by |
|---|---|---|
| `resolvers.txt` | Pre-verified resolver IPs | `quick` and `full` modes |
| `ranges.txt` | CIDR ranges to scan | `range` and `full` modes |

To update the bundled data, replace these files and rebuild the binary.
Sources: `ir-resolvers.txt` and `ir-cidrs.txt` from the findns project.

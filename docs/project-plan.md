# MasterDnsVPN Resolver Scanner — Project Plan

## Goal

A tool that takes a pool of IP addresses, filters out unsuitable DNS resolvers, and tests the
remaining ones end-to-end against a MasterDnsVPN server. Designed for use in extreme censorship
environments where most DNS resolvers are blocked, poisoned, or otherwise unusable.

---

## Development Phases

### Phase 1 — Go CLI

Build the full scanning pipeline as a CLI tool in Go. Cross-compile and release binaries for
Linux, macOS, Windows, and Android (Termux). Validate every stage in real censored environments.

The Go implementation becomes the **reference implementation** — the source of truth for correct
behavior at every stage of the pipeline.

### Phase 2 — Flutter App

Build a Flutter UI shell targeting Android, macOS, Linux, and Windows. The scanning logic gets
reimplemented in Dart using the Go code as the spec. No bridging, no FFI, no inter-process
communication — clean separation from the start.

The Flutter phase is deferred until the logic is proven correct in Phase 1. This avoids debugging
protocol issues and UI issues at the same time.

---

## Why CLI First

- Bugs in the scanning logic surface immediately without UI noise
- Termux on Android gives real validation on the actual target platform before the Flutter app exists
- Go binaries are immediately useful and distributable — users can start using the tool during Phase 1
- When Flutter work begins, the hard problem (correct scanning logic) is already solved

---

## Architecture Principle

The Go codebase must maintain clean separation between:

- **Core logic** — scanning stages, DNS protocol, VPN protocol, result data structures
- **Presentation** — CLI output, progress reporting, formatting

This separation makes the Dart port straightforward. The Flutter app mirrors the Go core logic;
Flutter handles presentation. Nothing needs to be untangled later.

---

## Technology

| Phase | Technology | Reason |
|---|---|---|
| Phase 1 CLI | Go | Single static binary, built-in cross-compilation, goroutines for concurrency, `miekg/dns` for DNS |
| Phase 2 App | Flutter (Dart) | One codebase for Android + macOS + Linux + Windows, AOT-compiled native, no bridging complexity |

### Go Cross-Compilation Targets

```bash
GOOS=linux   GOARCH=amd64  CGO_ENABLED=0 go build -o scanner-linux-amd64
GOOS=linux   GOARCH=arm64  CGO_ENABLED=0 go build -o scanner-linux-arm64
GOOS=windows GOARCH=amd64  CGO_ENABLED=0 go build -o scanner-windows.exe
GOOS=darwin  GOARCH=arm64  CGO_ENABLED=0 go build -o scanner-macos-arm64
GOOS=darwin  GOARCH=amd64  CGO_ENABLED=0 go build -o scanner-macos-amd64
GOOS=android GOARCH=arm64  CGO_ENABLED=0 go build -o scanner-android-arm64
```

`CGO_ENABLED=0` produces a fully static binary with no runtime dependencies.

---

## Project Structure (Phase 1)

```
masterDnsScanner/
├── main.go
├── config/
│   └── config.go          # domain, encryption key, timeouts, concurrency limits
├── input/
│   └── input.go           # IP list file, CIDR range, single IP parsing
├── stages/
│   ├── stage1_reach.go    # UDP port 53 reachability
│   ├── stage2_valid.go    # Valid DNS response check
│   ├── stage3_poison.go   # Poisoning / hijack detection
│   ├── stage4_txt.go      # TXT record support
│   ├── stage5_ns.go       # VPN domain NS resolution
│   ├── stage6_mtu.go      # MasterDnsVPN MTU binary search
│   └── stage7_e2e.go      # Full E2E session handshake
├── protocol/
│   └── vpn.go             # MasterDnsVPN packet building and parsing (reimplemented from dns_utils)
├── output/
│   └── output.go          # Result ranking, scoring, formatting, export
└── docs/
```

---

## Scanning Pipeline

Resolvers are processed through stages in order. A resolver that fails any stage is dropped
immediately — no further stages are run against it.

### Stage 1 — UDP Port 53 Reachability
**Cost:** 1 packet · **Timeout:** 0.5s · **Concurrency:** 500–1000

Send the smallest valid DNS query to each IP. No response means the resolver is dead or blocked
at the network level. No domain required. Eliminates the majority of the pool immediately.

### Stage 2 — Valid DNS Response
**Cost:** 1 packet · **Concurrency:** 500–1000

Send a standard A query for a known domain (e.g. `google.com`). Verify the response is a valid
DNS packet with RCODE `NOERROR` and at least one answer. Catches misconfigured forwarders and
honeypot resolvers that accept port 53 traffic but return garbage.

### Stage 3 — Poisoning / Hijack Detection
**Cost:** 2–3 packets · **Concurrency:** 200–500

Two sub-checks:

- **Known-good baseline:** Query a domain with a well-known stable IP (e.g. `one.one.one.one`
  must return `1.1.1.1`). A different answer means the resolver is hijacking responses.
- **NXDOMAIN injection:** Query a random nonexistent domain. If it returns an A record instead
  of NXDOMAIN, the resolver redirects all queries — a standard censorship technique.

### Stage 4 — TXT Record Support
**Cost:** 1–2 packets · **Concurrency:** 200–500

MasterDnsVPN uses TXT queries exclusively. Query TXT records for a known domain that has them
(e.g. `_dmarc.google.com`). Verify the TXT data comes back intact and non-empty. Some resolvers
in censored environments strip or block TXT records entirely.

### Stage 5 — VPN Domain NS Resolution
**Cost:** 1–2 packets · **Concurrency:** 100–200

Query the actual VPN domain from config. Verify NS records return the VPN server's nameserver
and A/AAAA records return the VPN server's IP. Catches domain-specific blocking — a resolver
may work fine for `google.com` but poison the VPN domain specifically.

### Stage 6 — MasterDnsVPN MTU Binary Search
**Cost:** 20–40 packets · **Concurrency:** 20–50

Replicates the exact MTU test the VPN client runs internally. Requires the VPN server to be
running and the domain and encryption key to be configured.

**Upload MTU binary search** (`MTU_UP_REQ` → `MTU_UP_RES`):
Binary search from 30 to max (default 512 bytes). Finds the largest TXT query payload the
resolver will forward to the VPN server and return a valid response for.
Records: `upload_mtu_bytes`, `upload_mtu_chars`.

**Download MTU binary search** (`MTU_DOWN_REQ` → `MTU_DOWN_RES`):
Asks the VPN server to send back N bytes and binary searches for the maximum N that arrives
intact. Records: `download_mtu_bytes`.

Both encoding modes are tested:
- Raw bytes in TXT response (`BASE_ENCODE=false`)
- Base64-encoded TXT response (`BASE_ENCODE=true`)

The mode that yields the higher MTU is recorded. Resolvers that don't support EDNS0 are not
eliminated — they simply converge to a lower download MTU (≤450 bytes) and score lower.

### Stage 7 — Full E2E Session Handshake
**Cost:** 5–10 packets · **Concurrency:** 5–10

Runs the actual `SESSION_INIT` → session ID exchange with the VPN server. Confirms the resolver
correctly forwards the encrypted VPN packet, the server decrypts it, assigns a session ID, and
responds. Optionally follows up with `SET_MTU_REQ` and `PING` to verify the session is live.

---

## Scoring

Resolvers that pass all stages are ranked by score:

| Metric | Weight |
|---|---|
| Upload MTU | 40% |
| Download MTU | 40% |
| Average RTT | 20% |

### Output Record Per Resolver

| Field | Example |
|---|---|
| IP | `185.x.x.x` |
| Avg RTT | `42ms` |
| Upload MTU | `210 bytes` |
| Download MTU | `180 bytes` |
| Base encode required | `false` |
| E2E session verified | `true` |
| Score | `94/100` |

Final output is sorted by score and formatted for direct use in `RESOLVER_DNS_SERVERS` in the
MasterDnsVPN client config.

---

## Note on EDNS0

MasterDnsVPN always sends EDNS0 in outgoing queries (4096-byte buffer declared). However EDNS0
support is not a hard requirement and is not tested as a separate stage:

- **Upload direction:** VPN data lives in DNS labels (the subdomain portion of the query). A
  resolver that strips EDNS0 from forwarded queries still delivers the full payload to the VPN
  server.
- **Download direction:** The VPN server only adds EDNS0 to its response if the incoming query
  included it (`ar_count > 0` check in `DnsPacketParser`). If the resolver strips EDNS0, the
  server stays within the 512-byte UDP limit. The MTU binary search in Stage 6 converges to the
  correct limit automatically — resolvers without EDNS0 simply score lower.

---

## Protocol Reimplementation (Go)

The `dns_utils` Python package from MasterDnsVPN source must be reimplemented in Go for Stages
6 and 7. Components and estimated complexity:

| Component | Source reference | Complexity |
|---|---|---|
| Label encoding / chunking | `DnsPacketParser.generate_labels` | Medium |
| Upload MTU capacity calc | `DnsPacketParser.calculate_upload_mtu` | Low |
| VPN header creation | `DnsPacketParser.create_vpn_header` | Medium |
| VPN header parsing | `DnsPacketParser.parse_vpn_header_bytes` | Medium |
| DNS response extraction | `DnsPacketParser.extract_vpn_response` | Low |
| XOR cipher (method 1) | `dns_utils/utils.py` | Trivial |
| ChaCha20 (method 2) | `dns_utils/utils.py` | Trivial — `golang.org/x/crypto/chacha20poly1305` |
| AES-128/192/256-GCM (methods 3–5) | `dns_utils/utils.py` | Trivial — `crypto/aes` + `crypto/cipher` |
| Key derivation per cipher | `DnsPacketParser._derive_key` | Low |
| MTU binary search | `client.py _binary_search_mtu` | Low |
| DNS packet send/receive | `client.py _send_and_receive_dns` | Trivial — `miekg/dns` |
| Upload MTU test | `client.py send_upload_mtu_test` | Low — note: `encode_data=False`, no encryption |
| Download MTU test | `client.py send_download_mtu_test` | Low — note: `encode_data=True`, encrypted |
| SESSION_INIT packet | `client.py _init_session` | Low — includes compression nibble byte |
| SET_MTU_REQ sync | `client.py _sync_mtu_with_server` | Low |

**Key implementation notes:**

- Upload MTU test sends data as raw hex (no encryption, `encode_data=False`). Download MTU
  test encrypts the payload (`encode_data=True`). These are not symmetric.
- SESSION_INIT payload is 18 bytes: 16-char hex token + flag byte + compression byte.
  Compression byte = `(upload_compression << 4) | download_compression`. For scanner use,
  set both to 0 (OFF) since we are testing connectivity, not sending real traffic.
- VPN header has variable length depending on packet type. Stream-related packets include
  stream_id (2 bytes) and sequence_num (2 bytes). Fragment-related packets add fragment_id
  (1 byte), total_fragments (1 byte), total_data_length (2 bytes). PING is 2 bytes only.
- Crypto overhead to subtract from usable MTU: 0 (XOR/None), 16 (ChaCha20), 28 (AES-GCM).

Total estimated effort: 3–5 days to implement and validate against a live MasterDnsVPN server.

---

## Phase 1 Timeline — Go CLI

### M1 — Foundation `3 days` — DONE

Everything needed before a single DNS packet can be sent.

- [x] Go module init, folder structure (`cli/`)
- [x] Config: flags + optional config file (domain, key, timeouts, concurrency per stage)
- [x] Input: single IP, CIDR notation, wildcard notation (`2.144.x.x/24`), IP list file
- [x] Pool: bundled curated list (~7,800 IPs from findns `ir-resolvers.txt`) embedded in binary
- [x] Pool: bundled default CIDR ranges (~10.8M IPs from findns `ir-cidrs.txt`)
- [x] Scan mode flag: `--mode quick|range|full`
- [x] Shuffle-within-step-window logic for CIDR scanning
- [x] Early-exit logic (min_valid_resolvers) for quick/range modes
- [x] Full scan confirmation prompt with IP count estimate
- [x] Progress output using bracketed tags (`[ok]` `[fail]` `[info]` etc.)
- [x] Logger module (`cli/logger/`) — stdout clean, file has millisecond timestamps, `--log` flag
- [x] Asset cleaner (`cli/tools/cleanassets/`) — validates and deduplicates bundled data pre-build
- [x] Build script (`scripts/build.sh`) — cleans assets then cross-compiles all 6 targets
- [x] CLI docs (`docs/cli-usage.md`, `docs/logging.md`, `docs/build.md`)

---

### M2 — Generic DNS Stages 1–5 `4 days` — NEXT

No VPN server required. Pure DNS validation.

- [ ] Concurrency control: semaphore per stage with configurable limits
- [ ] Stage 1: UDP port 53 reachability (750 concurrent, 0.5s timeout)
- [ ] Stage 2: Valid DNS response — A query, RCODE NOERROR, at least one answer
- [ ] Stage 3: Poisoning detection — known-good baseline + NXDOMAIN injection check
- [ ] Stage 4: TXT record support — query `_dmarc.google.com`, verify data intact
- [ ] Stage 5: VPN domain NS resolution — NS + A/AAAA for configured domain
- [ ] Per-stage result structs (RTT, pass/fail, reason)
- [ ] Live stats output: IPs scanned, passed, failed, dropped per stage
- [ ] Wire stages into pool stream in main.go

Note: `miekg/dns` and `golang.org/x/crypto` dependencies already installed (done in M1).

**Milestone exit:** tool can ingest a pool and produce a filtered list of clean DNS resolvers
without needing a VPN server. Immediately useful as a standalone pre-filter.

---

### M3 — Protocol Layer `4 days`

Go reimplementation of `dns_utils`. The hardest milestone — must match Python behavior exactly.

- [ ] Study `DnsPacketParser.py` in full before writing any Go
- [ ] Label encoding / chunking (`generate_labels`)
- [ ] VPN header creation (`create_vpn_header`)
- [ ] XOR cipher
- [ ] ChaCha20 cipher (`golang.org/x/crypto/chacha20poly1305`)
- [ ] AES-128/192/256-GCM (`crypto/aes` + `crypto/cipher`)
- [ ] Cipher dispatch (select by encryption method ID, mirrors Python `codec_transform`)
- [ ] DNS packet send/receive (UDP, configurable timeout, buffer size)
- [ ] Packet parsing: extract header, packet type, returned data
- [ ] Validate output against live MasterDnsVPN server — send raw packets, check responses

**Milestone exit:** Go code can exchange valid packets with a real MasterDnsVPN server.

---

### M4 — VPN Stages 6–7 `4 days`

Requires a live VPN server and M3 complete.

- [ ] Stage 6: upload MTU binary search (`MTU_UP_REQ` / `MTU_UP_RES`)
- [ ] Stage 6: download MTU binary search (`MTU_DOWN_REQ` / `MTU_DOWN_RES`)
- [ ] Stage 6: test both encoding modes (raw + base64), record best
- [ ] Stage 6: configurable MTU limits and retry count (mirrors client config)
- [ ] Stage 7: `SESSION_INIT` handshake, capture session ID
- [ ] Stage 7: optional `SET_MTU_REQ` + `PING` verification
- [ ] Concurrency limits respected (stage 6: 20–50, stage 7: 5–10)

**Milestone exit:** tool produces fully scored resolver results end-to-end.

---

### M5 — Output and Scoring `2 days`

- [ ] Scoring formula: upload MTU 40% + download MTU 40% + RTT 20%
- [ ] Ranked results table (IP, RTT, upload MTU, download MTU, base encode, E2E, score)
- [ ] Export: plain IP list ready to paste into `RESOLVER_DNS_SERVERS`
- [ ] Export: full JSON result for later processing
- [ ] Summary on completion: total scanned, passed each stage, final valid count

---

### M6 — Build Pipeline `2 days` — PARTIAL

- [x] `scripts/build.sh` — cleans assets then cross-compiles all 6 targets into `cli/bin/`
- [x] `cli/tools/cleanassets/` — validates and deduplicates bundled data before embedding
- [x] All 6 targets build and produce static binaries
- [ ] `.github/workflows/release.yml` — builds and attaches binaries on git tag push
- [ ] `cli/README.md` — usage, flags, config reference, example output

---

### Summary

| Milestone | Focus | Status |
|---|---|---|
| M1 | Foundation — config, input, pool, logger, build tooling | DONE |
| M2 | Stages 1–5 — generic DNS validation | NEXT |
| M3 | Protocol layer — Go reimplementation of dns_utils | pending |
| M4 | Stages 6–7 — VPN MTU test + E2E handshake | pending |
| M5 | Output, scoring, export | pending |
| M6 | Build pipeline, CI/CD, binaries | PARTIAL (build script done, CI/CD pending) |

M2 can start immediately — no blockers.
M3 must complete before M4 can start.
M5 can be built incrementally alongside M4.

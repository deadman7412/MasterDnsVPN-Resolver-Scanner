# MasterDnsVPN Resolver Scanner — Tool Design

## What We're Actually Testing

In MasterDnsVPN, the connection path is:

```
Your App → Client → [DNS Resolver IP] → (forwards) → VPN Server (authoritative NS)
                                                              ↓
Your App ← Client ← [DNS Resolver IP] ← (returns)  ← VPN Server
```

The **resolver IP** is the middleman. The goal is to find resolvers that:
1. Are reachable in the censored environment
2. Forward DNS to the VPN server without poisoning or stripping payloads
3. Handle TXT records carrying VPN protocol packets
4. Pass the actual MasterDnsVPN MTU protocol handshake

---

## IP Pool

### Sources (used in order)

**Tier 1 — Bundled curated list (default)**
A compact list of ~5,000–10,000 known DNS resolver IPs embedded in the binary. Pre-filtered
through Stages 1–4 and updated with each release. Sources: PYDNS-Scanner list (filtered),
public-dns.info, openresolvers.org. This is what runs when the user hits scan with no
configuration — fast, no setup required.

**Tier 2 — Bundled default CIDR ranges**
A set of CIDR ranges known to contain many open resolvers in censored environments, shipped
as an editable config file. Users can add, remove, or replace ranges.

**Tier 3 — User-defined input**
Provided at runtime. Supports:

| Format | Example |
|---|---|
| Single IP | `8.8.8.8` |
| Standard CIDR | `185.51.0.0/16` |
| Wildcard notation | `2.144.x.x/24` → normalized to `2.144.0.0/24` |
| IP list file | `resolvers.txt` (one per line, mixed formats) |

---

## Scan Modes

Three modes control how the pool is consumed. All three run the same pipeline — they differ
only in scope and stopping behavior.

### Quick Scan
- Source: Tier 1 curated list only
- Stops as soon as `min_valid_resolvers` threshold is reached (default: 20)
- Typical runtime: seconds to a few minutes
- Best for: everyday use, quick refresh of resolver list

### Range Scan
- Source: Tier 2 default CIDR ranges + any user-defined input
- Uses shuffle-within-step-window strategy (see below)
- Stops as soon as `min_valid_resolvers` threshold is reached
- Typical runtime: minutes to tens of minutes
- Best for: finding resolvers in specific ISP or regional ranges

### Full Scan
- Source: all tiers — curated list + all CIDR ranges + user input
- No early exit — scans every IP in the pool regardless of how many valid resolvers found
- Shuffle still applied within step windows for even distribution
- Typical runtime: hours (depends on pool size and machine)
- Best for: thorough discovery on a strong machine or when time is not a constraint

The scan mode is set via a flag:
```
scanner --mode quick
scanner --mode range
scanner --mode full
```

---

## CIDR Scanning Strategy

For any range-based input, IPs are not scanned linearly. A shuffle-within-step approach is
used to distribute probes across the range before going deep into any one subnet:

```
Range: 185.51.0.0/16  →  65,536 IPs

Step window: 1,000 IPs
Round 1: shuffle first 1,000 → scan → 12 valid found
Round 2: shuffle next 1,000  → scan → 8 valid found
...
Quick/Range mode: stop when min_valid_resolvers reached
Full mode: continue until all IPs exhausted
```

This means a /16 range does not always take hours — if resolvers are dense, Quick/Range mode
finds them fast and stops. Full mode uses the same shuffle but never stops early.

Step window and min_valid_resolvers are configurable.

---

## Filter Pipeline

Ordered from cheapest to most expensive. A resolver that fails any stage is **dropped immediately**.

---

### Stage 1 — UDP Port 53 Reachability

**Cost:** ~1 packet, 0.5s timeout
**Concurrency:** 500–1000 parallel

Send the smallest valid DNS query (A record for `.`) to each IP. No response = dead or blocked at network level.

This alone eliminates most of the pool in a censored environment. No domain needed — pure reachability probe.

---

### Stage 2 — Valid DNS Response Check

**Cost:** ~1 packet
**Concurrency:** 500–1000 parallel

Send a standard A query for a known domain (e.g. `google.com`). Verify:
- Response is a valid DNS packet (not garbage, not truncated without reason)
- RCODE is `NOERROR`
- Has at least one answer

Catches: resolvers that accept UDP on port 53 but return malformed or empty responses (honeypot resolvers, misconfigured forwarders).

---

### Stage 3 — DNS Poisoning / Hijacking Detection

**Cost:** ~2–3 packets
**Concurrency:** 200–500 parallel

Critical for censored environments. Many ISP resolvers intercept all DNS traffic and return fake answers.

Two sub-checks:

- **Known-good baseline:** Query a domain with a well-known stable IP (e.g. `one.one.one.one` → must return `1.1.1.1`). Wrong answer = hijacking.
- **NXDOMAIN injection check:** Query a random garbage domain (`xrandomtest12345abc.com`). If it returns an A record instead of NXDOMAIN, the resolver redirects all queries — a common censorship technique.

---

### Stage 4 — TXT Record Support

**Cost:** ~1–2 packets
**Concurrency:** 200–500 parallel

MasterDnsVPN uses TXT queries exclusively. Some resolvers in censored environments strip TXT records, return empty answers, or block them entirely.

- Query TXT for a known domain that has TXT records (e.g. `_dmarc.google.com`)
- Verify TXT data comes back intact and non-empty

---

### Stage 5 — VPN Domain NS Resolution

**Cost:** ~1–2 packets
**Concurrency:** 100–200 parallel

Query the actual VPN domain (the one in `DOMAINS` config). Checks:
- NS records for the domain → should return the VPN server's nameserver
- A/AAAA for the domain → should return the VPN server's IP

If this fails or returns wrong data, the resolver cannot reach the VPN server regardless of anything else. Also catches **domain-specific blocking** — a resolver may work fine for `google.com` but poison the VPN domain specifically.

---

### Stage 6 — MasterDnsVPN Protocol Test (MTU Binary Search)

**Cost:** ~20–40 packets
**Concurrency:** 20–50 parallel

Replicates the exact MTU test the VPN client runs internally. Requires the VPN server to be running.

**Upload MTU binary search** (`MTU_UP_REQ` → `MTU_UP_RES`):
- Binary search from 30 → max (default 512 bytes)
- Finds the largest TXT query payload the resolver will forward to the VPN server and return a valid response
- Records: `upload_mtu_bytes`, `upload_mtu_chars`

**Download MTU binary search** (`MTU_DOWN_REQ` → `MTU_DOWN_RES`):
- Asks the server to send back N bytes, binary searches for max N
- Validates the full return path payload size
- Records: `download_mtu_bytes`

Also tests both encoding modes:
- `BASE_ENCODE = false` (raw bytes in TXT response)
- `BASE_ENCODE = true` (base64-encoded TXT response)

Some resolvers block non-ASCII data in TXT responses. The mode that yields the higher MTU wins and is recorded.

#### Note on EDNS0

MasterDnsVPN always sends EDNS0 in outgoing queries (4096-byte buffer). However, **EDNS0 support is not a hard requirement**:

- **Upload direction:** VPN data lives in DNS labels (the subdomain), not the response. A resolver that strips EDNS0 from forwarded queries still delivers the full payload to the VPN server.
- **Download direction:** If the resolver strips EDNS0, the server detects this (checks `ar_count` in the incoming packet) and does not add EDNS0 to its response. The download is then capped at the standard 512-byte UDP limit (~450 bytes after DNS overhead). The MTU binary search converges to this limit automatically.

Resolvers without EDNS0 support are **not eliminated** — they simply receive a lower download MTU score and rank lower in the final output. Stage 6 captures this naturally.

---

### Stage 7 — Full E2E Session Handshake

**Cost:** ~5–10 packets
**Concurrency:** 5–10 parallel

Runs the actual `SESSION_INIT` → session ID exchange. Confirms:
- The resolver correctly forwards the encrypted VPN packet to the server
- The server decrypts it, assigns a session ID, and responds
- The full chain works end-to-end with real encryption

Optional: after session init, send a `SET_MTU_REQ` and a `PING` to verify the session is actually live.

---

## Concurrency Strategy

| Stage | Concurrency | Reason |
|---|---|---|
| 1–2 | 500–1000 | Pure network I/O, no VPN server involved |
| 3–4 | 200–500 | Still generic but slightly heavier |
| 5 | 100–200 | Touches the VPN domain |
| 6 | 20–50 | Hits the VPN server, avoid hammering it |
| 7 | 5–10 | Creates real sessions, be conservative |

---

## Output

Each resolver that passes all stages gets a result record:

| Field | Example |
|---|---|
| IP | `185.x.x.x` |
| Latency (avg RTT) | `42ms` |
| Upload MTU | `210 bytes` |
| Download MTU | `180 bytes` |
| Base encode needed | `false` |
| E2E session verified | `true` |
| Score | `94/100` |

### Scoring Formula

- Upload MTU: **40%** (bigger = more upstream bandwidth)
- Download MTU: **40%** (bigger = more downstream bandwidth)
- Latency (avg RTT): **20%** (lower = better)

Final output: a sorted list ready to paste directly into `RESOLVER_DNS_SERVERS` in the client config.

---

## Implementation Notes

- Stages 1–5 work without a VPN server and can be used as a standalone pre-filter.
- Stages 6–7 require the VPN server to be running and the domain and encryption key to be
  configured (mirroring `client_config.toml`).
- The Go protocol layer (`cli/protocol/vpn.go`) reimplements `dns_utils` from the MasterDnsVPN
  source. The Python source is the reference — behavior must match exactly.
- Full Scan on a large pool (e.g. multiple /16 ranges) can generate millions of UDP probes.
  The tool should print estimated time and IP count before starting Full Scan and prompt for
  confirmation.

### Protocol Details (from latest source)

**Upload vs download MTU test asymmetry:**
- Upload test sends data as raw hex with `encode_data=False` — no encryption applied
- Download test encrypts payload with `codec_transform` and uses `encode_data=True`
- These are not symmetric and must be implemented separately

**SESSION_INIT payload (18 bytes):**
```
[0–15]  16 ASCII hex chars   random init token
[16]    1 byte               base-encode flag (0x01 or 0x00)
[17]    1 byte               compression nibbles: (upload_type << 4) | download_type
```
For scanner use, set compression byte to `0x00` (both directions OFF). Compression is
irrelevant for connectivity testing and disabled automatically below 100-byte MTU anyway.

**Encryption methods and crypto overhead:**

| ID | Cipher | Key derivation | Overhead |
|---|---|---|---|
| 0 | None | — | 0 bytes |
| 1 | XOR | key zero-padded to 32 bytes | 0 bytes |
| 2 | ChaCha20 | SHA256(key) → 32 bytes | 16 bytes |
| 3 | AES-128-GCM | MD5(key) → 16 bytes | 28 bytes |
| 4 | AES-192-GCM | key zero-padded → 24 bytes | 28 bytes |
| 5 | AES-256-GCM | SHA256(key) → 32 bytes | 28 bytes |

**VPN header size by packet type:**
- `PING` / `SESSION_INIT`: 2 bytes (session_id + packet_type only)
- `MTU_UP_REQ` / `MTU_DOWN_REQ`: 10 bytes (+ stream_id + seq_num + frag fields)
- Stream data packets: 10+ bytes depending on fragment and compression fields

**MasterDnsVPN server's built-in MTU save feature:**
The server config has `SAVE_MTU_SERVERS_TO_FILE`, `MTU_SERVERS_FILE_NAME`, and
`MTU_SERVERS_FILE_FORMAT` which saves valid resolver results automatically. Our scanner
output format should be compatible with the client's `RESOLVER_DNS_SERVERS` list format.

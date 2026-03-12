# MasterDnsVPN Connectivity Test Flow

The test phase runs during startup via `test_mtu_sizes()`. It has two sequential steps per
resolver-domain pair, followed by session negotiation.

---

## Step 1 — Upload MTU Test (`MTU_UP_REQ` / `MTU_UP_RES`)

**Client side** (`send_upload_mtu_test`):
1. Calculates how many characters fit inside DNS query labels for the given domain + MTU size
   via `calculate_upload_mtu(domain, mtu)`
2. Generates a random hex string of exactly `mtu_char_len` characters. First character is the
   base-encode flag: `"1"` if `BASE_ENCODE_DATA=true`, `"0"` otherwise
3. Builds a DNS TXT query using `packet_type=MTU_UP_REQ` with `encode_data=False` — data is
   sent as raw hex, NOT encrypted or base32-encoded (upload test bypasses encryption)
4. Sends to the DNS resolver, waits `MTU_TEST_TIMEOUT` seconds
5. **Success**: server replies `MTU_UP_RES` → upload path alive at this size
6. **Failure**: timeout, wrong packet type, or `ERROR_DROP` → size too large or path dead

**Server side** (`_handle_mtu_up`):
- Receives the query, checks first label character for base-encode flag
- Replies immediately with `MTU_UP_RES` + `b"1"` — pure reachability echo, no data processing

**Binary search**: `_binary_search_mtu()` finds the largest payload (30 → `MAX_UPLOAD_MTU`,
default 220 bytes) the resolver will forward without dropping. Results cached per-MTU-value
to avoid re-testing the same size.

---

## Step 2 — Download MTU Test (`MTU_DOWN_REQ` / `MTU_DOWN_RES`)

**Client side** (`send_download_mtu_test`):
1. Builds payload:
   - 1 byte: base-encode flag (`0x01` or `0x00`)
   - 4 bytes: requested download size (big-endian uint32)
   - Remaining bytes: random padding up to upload MTU size
2. Encrypts payload with `codec_transform(data_bytes, encrypt=True)` — download test IS encrypted
3. Builds DNS TXT query with `packet_type=MTU_DOWN_REQ`, `encode_data=True`
4. Waits for response, validates `len(returned_data) == requested_size` exactly
5. **Success**: exact-length response received → download path works at this size
6. **Failure**: timeout, wrong packet type, or data length mismatch

**Server side** (`_handle_mtu_down`):
- Decrypts and parses: `flag (1 byte)` + `download_size (4 bytes big-endian)` + padding
- Rejects if `download_size < 29`
- Pads with `os.urandom()` to exactly `download_size` bytes
- Replies with `MTU_DOWN_RES` containing the exact-length blob, encrypted

**Binary search**: Same `_binary_search_mtu()` — finds max download size (30 → `MAX_DOWNLOAD_MTU`,
default 200 bytes).

---

## After Both Steps Pass

A connection is marked `is_valid=True` with its upload/download MTU values. After all pairs
are tested, the client selects the minimum MTU across all valid connections and proceeds:

### Session Init (`SESSION_INIT` / `SESSION_ACCEPT`)

Client sends `SESSION_INIT` with `session_id=0` (no session yet). Payload:
- 16 ASCII hex chars: random init token (8 random bytes as hex)
- 1 byte: base-encode flag (`0x01` / `0x00`)
- 1 byte: compression nibbles — upper 4 bits = upload compression type,
  lower 4 bits = download compression type (0=OFF, 1=ZSTD, 2=LZ4, 3=ZLIB)

Full payload is encrypted then base32-encoded before sending.

Server replies `SESSION_ACCEPT` with `init_token + ":" + session_id`. Client validates the
token matches, extracts the assigned session ID (1–255).

### MTU Sync (`SET_MTU_REQ` / `SET_MTU_RES`)

Client sends `SET_MTU_REQ` with:
- 4 bytes: agreed upload MTU (big-endian uint32)
- 4 bytes: agreed download MTU (big-endian uint32)
- 8 bytes: random sync token

Server stores MTU values in the session, subtracts `crypto_overhead`, recalculates
`max_packed_blocks`, and echoes back the sync token as confirmation.

---

## Encryption and Crypto Overhead

All encrypted fields subtract `crypto_overhead` from the usable MTU:

| Method ID | Cipher | Key Derivation | Overhead |
|---|---|---|---|
| 0 | None | — | 0 bytes |
| 1 | XOR | key padded to 32 bytes | 0 bytes |
| 2 | ChaCha20 | SHA256(key) | 16 bytes (nonce) |
| 3 | AES-128-GCM | MD5(key) | 28 bytes (12 nonce + 16 tag) |
| 4 | AES-192-GCM | key zero-padded to 24 bytes | 28 bytes |
| 5 | AES-256-GCM | SHA256(key) | 28 bytes |

Encryption is applied before base32 encoding in all cases.

---

## Compression

Supported compression types negotiated during `SESSION_INIT`:

| ID | Type |
|---|---|
| 0 | OFF (no compression) |
| 1 | ZSTD |
| 2 | LZ4 |
| 3 | ZLIB (default) |

Compression is automatically disabled if negotiated MTU falls below 100 bytes — compressing
tiny payloads adds overhead without benefit.

---

## Stream Teardown Phases (inside `close_stream()`)

Unrelated to the connectivity test — documents the two-phase stream close:
- Phase 1: graceful drain — initiates `_initiate_graceful_close()`, waits for in-flight data
- Phase 2: final cleanup — sets status to `CLOSING`, flushes TX queue, closes ARQ/stream object

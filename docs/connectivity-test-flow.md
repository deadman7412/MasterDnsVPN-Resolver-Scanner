# MasterDnsVPN Connectivity Test Flow (MTU Testing Phase)

The test phase runs during startup via `test_mtu_sizes()` (client.py:826). It has **two sequential steps per resolver-domain pair**:

---

## Step 1 — Upload MTU Test (`MTU_UP_REQ`)

**Client side** (`send_upload_mtu_test`, line 340):
1. Calculates how many characters fit inside a DNS query label for the given domain + MTU size
2. Builds a DNS TXT query packed with random hex data (max ~512 bytes) using packet type `MTU_UP_REQ`
3. Sends it to the DNS resolver and waits `mtu_test_timeout` seconds for a response
4. **Success**: server replies with `MTU_UP_RES` → upload path is working
5. **Failure**: no response, wrong packet type, or `ERROR_DROP` → connection is dead

**Server side** (`_handle_mtu_up`, line 1199):
- Simply receives the packet and replies with `MTU_UP_RES` + `b"1"` — it's a pure echo/reachability check
- Checks the first label character to detect if base encoding is requested

**Binary search**: The client doesn't just test one size — it runs `_binary_search_mtu()` to find the **maximum payload that the upstream resolver forwards without dropping** (range: 30–512 bytes).

---

## Step 2 — Download MTU Test (`MTU_DOWN_REQ`)

**Client side** (`send_download_mtu_test`, line 406):
1. Sends a DNS TXT query with `MTU_DOWN_REQ` — the payload contains:
   - 1 byte: base-encoding flag
   - 4 bytes: the requested download size (big-endian uint32)
   - Padding random bytes up to the upload MTU size
2. Waits for a response and **validates that the returned payload length exactly matches the requested size**
3. **Success**: server replies `MTU_DOWN_RES` with `len(returned_data) == mtu_size` → download path works at that size
4. **Failure**: no response, wrong packet type, or data size mismatch

**Server side** (`_handle_mtu_down`, line 1154):
- Decodes the requested download size from the packet
- Pads or slices the payload to exactly that many bytes using `os.urandom()` for padding
- Replies with `MTU_DOWN_RES` containing the exact-length blob

**Binary search**: Same `_binary_search_mtu()` — finds the max download size the resolver allows through (tested up to `max_download_mtu` from config).

---

## After Both Steps Pass

If a connection passes both tests, it's marked `is_valid = True` with its discovered upload/download MTU values. After all pairs are tested:

1. **MTU sync** (`_sync_mtu_with_server`, line 913): Client sends `SET_MTU_REQ` with the agreed upload/download MTU values. Server stores them in the session and echoes back a token to confirm.
2. **Session init** (`_init_session`, line 998): Client sends `SESSION_INIT` to get a session ID — only then is the VPN tunnel active.

---

## The "Phase 2" Comment (line 2286 — Unrelated to Connectivity Test)

That comment is inside `close_stream()` and refers to **stream teardown phases**:
- Phase 1 (line 2272): graceful drain — waits for in-flight data
- Phase 2 (line 2286): final cleanup — closes the ARQ/stream object and flushes the TX queue

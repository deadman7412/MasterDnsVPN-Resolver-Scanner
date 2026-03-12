# MasterDnsVPN Resolver Scanner

A tool that finds working DNS resolvers for MasterDnsVPN in censored environments.
Takes a pool of IPs, filters unsuitable resolvers through a 7-stage pipeline, and
tests survivors end-to-end against a live MasterDnsVPN server.

## Docs

- `docs/project-plan.md` — goals, phases, pipeline overview, technology choices
- `docs/project-structure.md` — folder layout for `cli/` and `app/`
- `docs/tool-design.md` — full pipeline spec: IP pool, scan modes, all 7 stages, scoring
- `docs/connectivity-test-flow.md` — how MasterDnsVPN tests connectivity internally (reference)

## Project Structure

```
cli/          Go CLI (Phase 1 — build and validate logic here first)
app/          Flutter app (Phase 2 — UI shell, Dart reimplements scanning logic)
docs/         Documentation
scripts/      Build and cross-compilation scripts
MasterDnsVPN-main-source/   Reference source only — read but never import
```

## Rules

- No emojis anywhere — in code, CLI output, logs, docs, or comments
- Use bracketed tags for all status indicators: `[ok]` `[success]` `[warn]` `[error]` `[info]` `[skip]` `[fail]`
- Example: `[ok] 8.8.8.8 — upload MTU: 210, download MTU: 180, RTT: 42ms`

## Key Decisions

- **Go for CLI** — single static binary, built-in cross-compilation, goroutines for concurrency
- **Flutter for app** — one codebase for Android + macOS + Linux + Windows, pure Dart (no Go bridge)
- **CLI first** — prove logic correct in Go, then port to Dart for Flutter
- **Protocol** — `cli/protocol/vpn.go` reimplements `MasterDnsVPN-main-source/dns_utils/` in Go; behavior must match exactly

## Scan Modes

| Mode | Source | Stops when |
|---|---|---|
| Quick | Bundled curated list (~10k IPs) | min_valid_resolvers reached |
| Range | Bundled + user CIDR ranges | min_valid_resolvers reached |
| Full | All tiers | Every IP exhausted |

## Pipeline Stages (in order)

1. UDP port 53 reachability
2. Valid DNS response
3. Poisoning / hijack detection
4. TXT record support
5. VPN domain NS resolution
6. MasterDnsVPN MTU binary search (requires live VPN server)
7. Full E2E session handshake (requires live VPN server)

Stages 6–7 need: VPN domain + encryption key configured (mirrors `client_config.toml`).

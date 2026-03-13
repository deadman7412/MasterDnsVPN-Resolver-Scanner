# MasterDnsVPN Resolver Scanner

Finds working DNS resolvers for MasterDnsVPN in censored environments.
Tests IPs through a 7-stage pipeline and scores survivors by MTU and latency.

## Docs

- `docs/project-plan.md` — goals, phases, milestones
- `docs/tool-design.md` — full pipeline spec: stages, scoring, CIDR strategy
- `docs/project-structure.md` — folder layout for `cli/` and `app/`
- `docs/connectivity-test-flow.md` — MasterDnsVPN internal protocol reference
- `docs/cli-usage.md` — user-facing CLI reference: flags, modes, examples
- `docs/logging.md` — how session logging works (`--log` flag)
- `docs/build.md` — build procedure, asset cleaning, cross-compilation targets

## Project Structure

```
cli/          Go CLI (Phase 1)
app/          Flutter app (Phase 2)
docs/         Documentation
scripts/      Build and cross-compilation scripts
MasterDnsVPN-main-source/   Reference source — read but never import
```

## Rules

- No emojis anywhere — code, output, logs, docs, comments
- Use bracketed tags for all status: `[ok]` `[success]` `[warn]` `[error]` `[info]` `[skip]` `[fail]`
- **Sync rule**: when any flag, mode, or stage changes — update `docs/cli-usage.md`,
  `docs/logging.md` (if logging-related), and `flag.Usage` in `cli/main.go` together
- **Build rule**: always run `go run ./tools/cleanassets` before embedding asset files;
  `scripts/build.sh` does this automatically for release builds

## Key Decisions

- **Go CLI** — static binary, built-in cross-compilation, goroutines for concurrency
- **Flutter app** — one codebase for Android + macOS + Linux + Windows, pure Dart, no bridge
- **CLI first** — prove logic in Go, then port to Dart
- **Logger** — `cli/logger/logger.go`; stdout has no timestamps, log file has millisecond timestamps; off by default, enabled with `--log`
- **Protocol** — `cli/protocol/vpn.go` reimplements `MasterDnsVPN-main-source/dns_utils/`; must match exactly

## Pipeline Stages

1. UDP port 53 reachability
2. Valid DNS response
3. Poisoning / hijack detection
4. TXT record support
5. VPN domain NS resolution — requires `--domain`
6. MTU binary search — requires `--domain` + `--key`
7. Full E2E session handshake — requires `--domain` + `--key`

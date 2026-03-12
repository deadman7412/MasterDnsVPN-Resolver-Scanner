# Project Structure

## Overview

The repository is split into two top-level folders matching the two development phases.
`cli/` is Phase 1 (Go), `app/` is Phase 2 (Flutter). They share no code — the Flutter app
reimplements the scanning logic in Dart using the Go code as the reference.

```
masterDnsScanner/
│
├── cli/                          # Phase 1 — Go CLI
├── app/                          # Phase 2 — Flutter App
├── docs/                         # Documentation
├── scripts/                      # Build and release scripts
├── MasterDnsVPN-main-source/     # Reference source (read only, not imported)
└── .github/
    └── workflows/
        └── release.yml           # Auto-build binaries on git tag
```

---

## cli/ — Go CLI

```
cli/
├── go.mod
├── go.sum
├── main.go
├── config/
│   └── config.go             # Domain, encryption key, timeouts, concurrency limits per stage
├── input/
│   └── input.go              # IP list file, CIDR range, single IP parsing
├── stages/
│   ├── stage1_reach.go       # UDP port 53 reachability
│   ├── stage2_valid.go       # Valid DNS response check
│   ├── stage3_poison.go      # Poisoning / hijack detection
│   ├── stage4_txt.go         # TXT record support
│   ├── stage5_ns.go          # VPN domain NS resolution
│   ├── stage6_mtu.go         # MasterDnsVPN MTU binary search
│   └── stage7_e2e.go         # Full E2E session handshake
├── protocol/
│   └── vpn.go                # MasterDnsVPN packet building and parsing (reimplemented from dns_utils)
└── output/
    └── output.go             # Result ranking, scoring, formatting, export
```

### Key conventions

- Each stage is a self-contained file that receives a resolver IP and returns a pass/fail result
  plus any collected metrics (RTT, MTU values, etc.)
- `protocol/vpn.go` is the Go reimplementation of the MasterDnsVPN wire protocol from
  `MasterDnsVPN-main-source/dns_utils/`. It covers label encoding, VPN header creation,
  all cipher modes (XOR, ChaCha20, AES-128/192/256-GCM), and DNS packet send/receive.
- `config/config.go` mirrors the fields in `MasterDnsVPN-main-source/client_config.toml.simple`
  relevant to scanning: domain, encryption key, MTU limits, timeouts.
- `output/output.go` handles result ranking, scoring, and formatting. It has no dependency on
  any scanning stage — it only operates on result data structures.

### Cross-compilation targets

Produced by `scripts/build.sh`:

```
cli/bin/
├── scanner-linux-amd64
├── scanner-linux-arm64
├── scanner-windows-amd64.exe
├── scanner-macos-amd64
├── scanner-macos-arm64
└── scanner-android-arm64
```

All targets use `CGO_ENABLED=0` for fully static binaries with no runtime dependencies.

---

## app/ — Flutter App

```
app/
├── pubspec.yaml
├── lib/
│   ├── main.dart
│   ├── config/               # App configuration (mirrors cli/config/)
│   ├── models/               # Result data structures (mirrors Go output types)
│   ├── scanner/              # Dart reimplementation of all scanning stages
│   │   ├── stage1_reach.dart
│   │   ├── stage2_valid.dart
│   │   ├── stage3_poison.dart
│   │   ├── stage4_txt.dart
│   │   ├── stage5_ns.dart
│   │   ├── stage6_mtu.dart
│   │   └── stage7_e2e.dart
│   ├── protocol/             # MasterDnsVPN wire protocol in Dart
│   ├── screens/              # UI screens
│   └── widgets/              # Reusable UI components
├── android/
├── macos/
├── linux/
└── windows/
```

### Key conventions

- `scanner/` mirrors `cli/stages/` exactly in structure and behavior. The Go implementation
  is the reference — if results differ between Go and Dart for the same input, the Go result
  is correct.
- `protocol/` mirrors `cli/protocol/vpn.go` in Dart. Same packet format, same cipher modes.
- `models/` defines the same result data structures as the Go output types so that UI components
  have a stable, predictable shape to work with.
- No bridging to the Go binary. The Flutter app is fully self-contained in Dart.

---

## docs/

```
docs/
├── project-plan.md           # Overall project goals, phases, pipeline description
├── project-structure.md      # This file
├── tool-design.md            # Detailed scanner pipeline design and stage specifications
└── connectivity-test-flow.md # How MasterDnsVPN tests connectivity internally (reference)
```

---

## scripts/

```
scripts/
└── build.sh                  # Cross-compiles all CLI targets into cli/bin/
```

---

## .github/workflows/

```
.github/workflows/
└── release.yml               # Triggered on git tag push. Builds all 6 CLI binaries
                              # and attaches them to the GitHub release automatically.
```

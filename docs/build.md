# Building the CLI

---

## Quick Build (current machine)

```bash
cd cli/
go build -o scanner .
./scanner --help
```

---

## Release Build (all platforms)

The build script cleans the asset files first, then cross-compiles every target:

```bash
./scripts/build.sh
```

Output goes to `cli/bin/`:

```
cli/bin/
├── scanner-linux-amd64
├── scanner-linux-arm64
├── scanner-windows-amd64.exe
├── scanner-macos-amd64
├── scanner-macos-arm64
└── scanner-android-arm64
```

All targets are fully static binaries (`CGO_ENABLED=0`) with no runtime dependencies.

---

## Asset Cleaning

The binary embeds two data files at compile time:

| File | Content |
|---|---|
| `cli/assets/resolvers.txt` | Bundled curated resolver IPs (tier 1) |
| `cli/assets/ranges.txt` | Bundled CIDR ranges (tier 2) |

Before any build, run the cleaner to remove invalid entries and duplicates:

```bash
cd cli/
go run ./tools/cleanassets
```

The cleaner:
- Validates every non-comment line — IPs in `resolvers.txt`, CIDRs in `ranges.txt`
- Removes lines that fail validation (partial downloads, typos, corrupt entries)
- Removes duplicate entries after canonicalization
- Preserves all comment lines and blank lines in place
- Writes the cleaned file back in-place
- Reports a summary

Example output when files are clean:

```
[ok]   assets/resolvers.txt: 947 entries, all valid
[ok]   assets/ranges.txt: 609 entries, all valid
```

Example output when bad entries are found and removed:

```
[warn] assets/resolvers.txt: invalid entry removed: "6.61"
[warn] assets/resolvers.txt: duplicate removed: "8.8.8.8"
[warn] assets/resolvers.txt: 947 valid, 1 invalid removed, 1 duplicate removed
[ok]   assets/ranges.txt: 609 entries, all valid
```

The `scripts/build.sh` script always runs the cleaner before compiling, so a
release build will never embed corrupted data.

---

## Updating Bundled Data

To replace the bundled resolver list or CIDR ranges with fresh data:

1. Overwrite the file:
   ```bash
   cp /path/to/new-resolvers.txt cli/assets/resolvers.txt
   cp /path/to/new-ranges.txt    cli/assets/ranges.txt
   ```

2. Run the cleaner to validate:
   ```bash
   cd cli/
   go run ./tools/cleanassets
   ```

3. Fix any reported issues, then rebuild.

The findns project (`github.com/SamNet-dev/findns`) is the upstream source for
the bundled data. The files to use are `ir-resolvers.txt` and `ir-cidrs.txt`.

---

## Manual Cross-Compilation

Individual targets without the build script:

```bash
cd cli/

GOOS=linux   GOARCH=amd64 CGO_ENABLED=0 go build -o bin/scanner-linux-amd64   .
GOOS=linux   GOARCH=arm64 CGO_ENABLED=0 go build -o bin/scanner-linux-arm64   .
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o bin/scanner-windows-amd64.exe .
GOOS=darwin  GOARCH=amd64 CGO_ENABLED=0 go build -o bin/scanner-macos-amd64   .
GOOS=darwin  GOARCH=arm64 CGO_ENABLED=0 go build -o bin/scanner-macos-arm64   .
GOOS=android GOARCH=arm64 CGO_ENABLED=0 go build -o bin/scanner-android-arm64 .
```

---

## Development Workflow

Run without building (uses `go run`):

```bash
cd cli/
go run . --mode quick
go run . --help
```

Clean assets then run:

```bash
cd cli/
go run ./tools/cleanassets && go run . --mode quick
```

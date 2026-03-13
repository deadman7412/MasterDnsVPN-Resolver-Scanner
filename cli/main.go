package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/deadman7412/MasterDnsVPN-Resolver-Scanner/config"
	"github.com/deadman7412/MasterDnsVPN-Resolver-Scanner/logger"
	"github.com/deadman7412/MasterDnsVPN-Resolver-Scanner/pool"
)

// inputList is a flag.Value that accepts multiple --input flags and also
// splits comma-separated values within a single flag.
type inputList []string

func (l *inputList) String() string { return strings.Join(*l, ",") }
func (l *inputList) Set(s string) error {
	for _, v := range strings.Split(s, ",") {
		v = strings.TrimSpace(v)
		if v != "" {
			*l = append(*l, v)
		}
	}
	return nil
}

func main() {
	cfg := config.Default()

	var mode string
	var inputs inputList
	var minValid int
	var domain string
	var key string
	var method int
	var logPath string

	flag.Usage = func() {
		fmt.Print(`MasterDnsVPN Resolver Scanner

Finds working DNS resolvers in censored environments. Each candidate IP is
tested through a 7-stage pipeline and survivors are scored by MTU and latency.

Usage:
  scanner [flags]

Scan modes (--mode):
  quick   Scan the bundled curated resolver list (~7,800 pre-verified IPs).
          Stops when --min valid resolvers are found. No setup required.

  range   Scan the bundled CIDR ranges (~10.8M IPs) plus any --input targets.
          Stops when --min valid resolvers are found.

  full    Scan all sources combined. Never stops early -- exhausts every IP.
          Asks for confirmation before starting due to the large IP count.

Flags:
  --mode string     Scan mode: quick | range | full  (default: quick)

  --input string    IP, CIDR, wildcard, or file path. Repeatable. Also accepts
                    comma-separated values in a single flag.
                    Formats: 8.8.8.8 | 185.51.0.0/16 | 2.144.x.x | list.txt

  --min int         Stop after finding N valid resolvers. Applies to quick and
                    range modes. No effect in full mode.  (default: 20)

  --domain string   VPN domain name. Required for stage 5 and above.

  --key string      Encryption key. Required for stages 6 and 7.

  --method int      Encryption method (must match server config):
                      0 = none (plaintext)
                      1 = XOR
                      2 = ChaCha20-Poly1305
                      3 = AES-128-GCM
                      4 = AES-192-GCM
                      5 = AES-256-GCM
                    (default: 0)

  --log string      Save session log to file. Disabled by default.
                      --log auto            timestamped filename in current dir
                      --log scan.log        explicit path
                    Log file includes timestamps on every line. Stdout is unchanged.

Pipeline stages:
  1  UDP port 53 reachability         no config required   (750 concurrent)
  2  Valid DNS response               no config required   (750 concurrent)
  3  Poisoning / hijack detection     no config required   (350 concurrent)
  4  TXT record support               no config required   (350 concurrent)
  5  VPN domain NS resolution         requires --domain    (150 concurrent)
  6  MTU binary search                requires --domain + --key  (35 concurrent)
  7  Full E2E session handshake       requires --domain + --key   (7 concurrent)

  Stages 1-4 work with no config and are useful as a standalone DNS pre-filter.
  Stages 5-7 require a live MasterDnsVPN server.

Examples:
  scanner --mode quick
  scanner --mode quick --log auto
  scanner --mode range --input 185.51.0.0/16
  scanner --mode range --input 2.144.x.x --min 50
  scanner --mode range --input resolvers.txt
  scanner --mode quick --domain vpn.example.com --key mykey --method 2
  scanner --mode full  --domain vpn.example.com --key mykey --method 2 --log scan.log

For full documentation see docs/cli-usage.md and docs/logging.md
`)
	}

	flag.StringVar(&mode, "mode", "quick", "scan mode: quick | range | full")
	flag.Var(&inputs, "input", "IP, CIDR, wildcard, or file path (repeatable, comma-separated)")
	flag.IntVar(&minValid, "min", cfg.MinValidResolvers, "stop after N valid resolvers (quick/range modes)")
	flag.StringVar(&domain, "domain", "", "VPN domain (required for stages 5-7)")
	flag.StringVar(&key, "key", "", "encryption key (required for stages 6-7)")
	flag.IntVar(&method, "method", 0, "encryption method: 0=none 1=xor 2=chacha20 3=aes128 4=aes192 5=aes256")
	flag.StringVar(&logPath, "log", "", "save session log to file ('auto' for timestamped filename)")
	flag.Parse()

	// Initialize logger before anything else so all output is captured.
	log, err := logger.New(logPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[error] %v\n", err)
		os.Exit(1)
	}
	defer log.Close()

	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "quick":
		cfg.Mode = config.ModeQuick
	case "range":
		cfg.Mode = config.ModeRange
	case "full":
		cfg.Mode = config.ModeFull
	default:
		log.Error("unknown mode %q -- use quick, range, or full", mode)
		os.Exit(1)
	}

	cfg.MinValidResolvers = minValid
	cfg.Domain = domain
	cfg.EncryptionKey = key
	cfg.EncryptMethod = method
	cfg.Inputs = []string(inputs)

	// Resolve domain/key display values for the session header.
	domainDisplay := domain
	if domainDisplay == "" {
		domainDisplay = "-"
	}
	keyDisplay := "-"
	if key != "" {
		keyDisplay = "set"
	}

	log.SessionStart(
		"mode", cfg.Mode.String(),
		"min", fmt.Sprintf("%d", cfg.MinValidResolvers),
		"domain", domainDisplay,
		"key", keyDisplay,
		"method", fmt.Sprintf("%d", cfg.EncryptMethod),
		"inputs", fmt.Sprintf("%d extra", len(cfg.Inputs)),
	)
	defer log.SessionEnd()

	if log.LogPath() != "" {
		log.Info("logging to %s", log.LogPath())
	}

	p, err := pool.New(cfg)
	if err != nil {
		log.Error("pool: %v", err)
		os.Exit(1)
	}

	estimate := p.TotalEstimate()
	log.Info("mode: %s  estimated pool: %d IPs", cfg.Mode, estimate)

	if cfg.Mode == config.ModeFull {
		log.Warn("full scan will probe all %d IPs -- continue? [y/N] ", estimate)
		var answer string
		fmt.Scanln(&answer)
		if strings.ToLower(strings.TrimSpace(answer)) != "y" {
			log.Info("aborted")
			os.Exit(0)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Placeholder: count IPs emitted by the pool.
	// Scanning stages will be wired in during M2.
	count := 0
	for range p.Stream(ctx) {
		count++
	}

	log.Ok("pool ready -- %d IPs", count)
}

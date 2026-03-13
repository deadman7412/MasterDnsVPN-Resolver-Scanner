package config

import "time"

// ScanMode controls which IP tiers are used and when to stop.
type ScanMode int

const (
	ModeQuick ScanMode = iota // tier 1 only, stops at min_valid_resolvers
	ModeRange                 // tier 2 + tier 3, stops at min_valid_resolvers
	ModeFull                  // all tiers, exhausts every IP
)

func (m ScanMode) String() string {
	switch m {
	case ModeQuick:
		return "quick"
	case ModeRange:
		return "range"
	case ModeFull:
		return "full"
	}
	return "unknown"
}

// Config holds all runtime configuration for the scanner.
type Config struct {
	// VPN server settings — required for stages 5-7
	Domain        string
	EncryptionKey string
	// EncryptMethod: 0=none 1=xor 2=chacha20 3=aes128gcm 4=aes192gcm 5=aes256gcm
	EncryptMethod int

	// Scan behavior
	Mode              ScanMode
	MinValidResolvers int
	StepWindow        int // IPs per shuffle window when scanning CIDR ranges

	// User-supplied input strings: single IPs, CIDRs, wildcards, or file paths
	Inputs []string

	// Timeouts per stage. Index = stage number (1-based); index 0 is unused.
	Timeout [8]time.Duration

	// Concurrency limits per stage. Index = stage number (1-based); index 0 is unused.
	Concurrency [8]int
}

// Default returns a Config populated with production defaults.
func Default() *Config {
	return &Config{
		Mode:              ModeQuick,
		MinValidResolvers: 20,
		StepWindow:        1000,
		Timeout: [8]time.Duration{
			0,                      // index 0 unused
			500 * time.Millisecond, // stage 1 — reachability
			2 * time.Second,        // stage 2 — valid DNS response
			4 * time.Second,        // stage 3 — poison/hijack check
			4 * time.Second,        // stage 4 — TXT record support
			4 * time.Second,        // stage 5 — VPN domain NS resolution
			10 * time.Second,       // stage 6 — MTU binary search
			15 * time.Second,       // stage 7 — E2E session handshake
		},
		Concurrency: [8]int{
			0,   // index 0 unused
			750, // stage 1
			750, // stage 2
			350, // stage 3
			350, // stage 4
			150, // stage 5
			35,  // stage 6
			7,   // stage 7
		},
	}
}

package input

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
)

// EntryType distinguishes a single IP from a CIDR range.
type EntryType int

const (
	TypeSingle EntryType = iota
	TypeCIDR
)

// Entry is a parsed input: either a single IP or a network range.
type Entry struct {
	Type EntryType
	IP   net.IP     // valid when Type == TypeSingle
	Net  *net.IPNet // valid when Type == TypeCIDR
}

// Size returns the number of IP addresses this entry covers.
func (e Entry) Size() int64 {
	if e.Type == TypeSingle {
		return 1
	}
	ones, bits := e.Net.Mask.Size()
	return 1 << int64(bits-ones)
}

// Parse parses a single input string. Accepted formats:
//
//	"8.8.8.8"          single IPv4 address
//	"185.51.0.0/16"    CIDR notation
//	"2.144.x.x"        wildcard — normalized to "2.144.0.0/16"
//	"2.144.x.x/24"     wildcard with explicit prefix — normalized to "2.144.0.0/24"
func Parse(s string) (Entry, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Entry{}, fmt.Errorf("empty string")
	}

	normalized := normalizeWildcard(s)

	if strings.Contains(normalized, "/") {
		_, ipnet, err := net.ParseCIDR(normalized)
		if err != nil {
			return Entry{}, fmt.Errorf("invalid CIDR %q: %w", s, err)
		}
		// Store IPv4 networks as 4-byte addresses.
		if v4 := ipnet.IP.To4(); v4 != nil {
			ipnet.IP = v4
		}
		return Entry{Type: TypeCIDR, Net: ipnet}, nil
	}

	ip := net.ParseIP(normalized)
	if ip == nil {
		return Entry{}, fmt.Errorf("invalid IP %q", s)
	}
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	return Entry{Type: TypeSingle, IP: ip}, nil
}

// ParseFile reads path and parses each non-blank, non-comment line.
// Lines beginning with '#' are skipped. Returns an error if any line is invalid.
func ParseFile(path string) ([]Entry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []Entry
	sc := bufio.NewScanner(f)
	lineNum := 0
	for sc.Scan() {
		lineNum++
		text := strings.TrimSpace(sc.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		e, err := Parse(text)
		if err != nil {
			return nil, fmt.Errorf("%s line %d: %w", path, lineNum, err)
		}
		entries = append(entries, e)
	}
	return entries, sc.Err()
}

// ParseLines parses a slice of raw strings (e.g. from an embedded asset).
// Blank lines and lines starting with '#' are skipped.
// Invalid lines are silently skipped — embedded files may contain partial
// entries due to download corruption or manual edits.
func ParseLines(lines []string) ([]Entry, error) {
	var entries []Entry
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		e, err := Parse(line)
		if err != nil {
			continue
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// normalizeWildcard converts wildcard notation to standard CIDR.
//
//	"2.144.x.x"    -> "2.144.0.0/16"
//	"2.144.x.x/24" -> "2.144.0.0/24"
//	"185.x.0.0"    -> "185.0.0.0/8"
func normalizeWildcard(s string) string {
	if !strings.ContainsAny(s, "xX") {
		return s
	}

	// Count wildcards to infer prefix length when none is given.
	lower := strings.ToLower(s)
	hasPrefix := strings.Contains(lower, "/")

	result := strings.NewReplacer("x", "0", "X", "0").Replace(s)

	if !hasPrefix {
		octets := strings.Split(lower, ".")
		wildcards := 0
		for _, o := range octets {
			if o == "x" {
				wildcards++
			}
		}
		prefix := 32 - wildcards*8
		result = fmt.Sprintf("%s/%d", result, prefix)
	}

	return result
}

// cleanassets validates and deduplicates the bundled asset files before building.
//
// Run from the cli/ directory before go build:
//
//	go run ./tools/cleanassets
//
// For each asset file it:
//   - Removes lines that are not valid IPs (resolvers.txt) or CIDRs (ranges.txt)
//   - Removes duplicate entries
//   - Preserves all comment lines and blank lines in place
//   - Writes the cleaned file back in-place
//   - Prints a summary of what was kept and removed
package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
)

func main() {
	ok := true
	ok = cleanFile("assets/resolvers.txt", validateIP) && ok
	ok = cleanFile("assets/ranges.txt", validateCIDR) && ok
	if !ok {
		os.Exit(1)
	}
}

// cleanFile reads path, validates every non-comment non-blank line with the
// given validator, removes invalid and duplicate entries, and writes the result
// back to path. Returns true on success.
func cleanFile(path string, validate func(string) (string, bool)) bool {
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[error] open %s: %v\n", path, err)
		return false
	}

	type line struct {
		text    string
		isData  bool
		keep    bool
		reason  string
	}

	var lines []line
	seen := map[string]bool{}
	total, removed, dupes := 0, 0, 0

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		text := sc.Text()
		trimmed := strings.TrimSpace(text)

		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			lines = append(lines, line{text: text})
			continue
		}

		total++
		normalized, valid := validate(trimmed)

		if !valid {
			fmt.Printf("[warn] %s: invalid entry removed: %q\n", path, trimmed)
			removed++
			lines = append(lines, line{text: text, isData: true, keep: false, reason: "invalid"})
			continue
		}

		if seen[normalized] {
			fmt.Printf("[warn] %s: duplicate removed: %q\n", path, normalized)
			dupes++
			lines = append(lines, line{text: text, isData: true, keep: false, reason: "duplicate"})
			continue
		}

		seen[normalized] = true
		lines = append(lines, line{text: normalized, isData: true, keep: true})
	}
	f.Close()

	if err := sc.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "[error] read %s: %v\n", path, err)
		return false
	}

	// Write cleaned content back.
	out, err := os.Create(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[error] write %s: %v\n", path, err)
		return false
	}
	w := bufio.NewWriter(out)
	for _, l := range lines {
		if l.isData && !l.keep {
			continue
		}
		fmt.Fprintln(w, l.text)
	}
	if err := w.Flush(); err != nil {
		out.Close()
		fmt.Fprintf(os.Stderr, "[error] flush %s: %v\n", path, err)
		return false
	}
	out.Close()

	kept := total - removed - dupes
	if removed > 0 || dupes > 0 {
		fmt.Printf("[warn] %s: %d valid, %d invalid removed, %d duplicates removed\n",
			path, kept, removed, dupes)
	} else {
		fmt.Printf("[ok]   %s: %d entries, all valid\n", path, kept)
	}
	return true
}

// validateIP returns the canonical IPv4/IPv6 string for a valid IP address.
func validateIP(s string) (string, bool) {
	ip := net.ParseIP(s)
	if ip == nil {
		return "", false
	}
	if v4 := ip.To4(); v4 != nil {
		return v4.String(), true
	}
	return ip.String(), true
}

// validateCIDR normalizes wildcards and returns the canonical network address
// string for a valid CIDR range.
func validateCIDR(s string) (string, bool) {
	normalized := normalizeWildcard(s)
	_, ipnet, err := net.ParseCIDR(normalized)
	if err != nil {
		return "", false
	}
	return ipnet.String(), true
}

// normalizeWildcard converts wildcard notation to standard CIDR.
// Kept in sync with input/input.go — duplicated here to keep the tool
// self-contained and avoid build-time import cycles.
func normalizeWildcard(s string) string {
	if !strings.ContainsAny(s, "xX") {
		return s
	}
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

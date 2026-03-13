package assets

import (
	_ "embed"
	"strings"
)

//go:embed resolvers.txt
var resolversRaw string

//go:embed ranges.txt
var rangesRaw string

// ResolverLines returns lines from the bundled curated IP list (tier 1).
func ResolverLines() []string {
	return strings.Split(strings.TrimSpace(resolversRaw), "\n")
}

// RangeLines returns lines from the bundled default CIDR ranges (tier 2).
func RangeLines() []string {
	return strings.Split(strings.TrimSpace(rangesRaw), "\n")
}

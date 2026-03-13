package pool

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"strings"

	"github.com/deadman7412/MasterDnsVPN-Resolver-Scanner/assets"
	"github.com/deadman7412/MasterDnsVPN-Resolver-Scanner/config"
	"github.com/deadman7412/MasterDnsVPN-Resolver-Scanner/input"
)

// Pool assembles the IP pool from bundled assets and user input, then streams
// IPs to the pipeline according to the configured scan mode.
//
// Tier 1 — bundled curated IP list        (quick, full)
// Tier 2 — bundled default CIDR ranges    (range, full)
// Tier 3 — user-supplied input            (range, full)
type Pool struct {
	cfg   *config.Config
	tier1 []net.IP      // flat list of IPs from the curated resolver list
	tier2 []input.Entry // bundled CIDR ranges
	tier3 []input.Entry // user-supplied IPs, CIDRs, and file contents
}

// New creates a Pool from cfg. Bundled tier 1 and tier 2 assets are loaded
// automatically. User inputs from cfg.Inputs are parsed into tier 3.
func New(cfg *config.Config) (*Pool, error) {
	p := &Pool{cfg: cfg}

	// Tier 1 — bundled curated IP list.
	// resolvers.txt contains only single IPv4 addresses; CIDRs are skipped.
	for _, line := range assets.ResolverLines() {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		ip := net.ParseIP(line)
		if ip == nil {
			// Skip malformed lines — resolvers.txt may be user-edited or partially downloaded.
			continue
		}
		if v4 := ip.To4(); v4 != nil {
			ip = v4
		}
		p.tier1 = append(p.tier1, ip)
	}

	// Tier 2 — bundled default CIDR ranges.
	t2, err := input.ParseLines(assets.RangeLines())
	if err != nil {
		return nil, fmt.Errorf("ranges.txt: %w", err)
	}
	p.tier2 = t2

	// Tier 3 — user-supplied input from cfg.Inputs.
	// Each element is tried first as an IP/CIDR/wildcard, then as a file path.
	for _, raw := range cfg.Inputs {
		e, err := input.Parse(raw)
		if err == nil {
			p.tier3 = append(p.tier3, e)
			continue
		}
		entries, ferr := input.ParseFile(raw)
		if ferr != nil {
			return nil, fmt.Errorf("input %q: not a valid IP, CIDR, or readable file", raw)
		}
		p.tier3 = append(p.tier3, entries...)
	}

	return p, nil
}

// TotalEstimate returns the total number of IPs in the pool for the current
// scan mode. For large CIDR ranges this may be a very large number.
func (p *Pool) TotalEstimate() int64 {
	var total int64
	switch p.cfg.Mode {
	case config.ModeQuick:
		total = int64(len(p.tier1))
	case config.ModeRange:
		for _, e := range p.tier2 {
			total += e.Size()
		}
		for _, e := range p.tier3 {
			total += e.Size()
		}
	case config.ModeFull:
		total = int64(len(p.tier1))
		for _, e := range p.tier2 {
			total += e.Size()
		}
		for _, e := range p.tier3 {
			total += e.Size()
		}
	}
	return total
}

// Stream returns a channel of IPs produced in order of the scan mode tiers.
// IPs within each step window are shuffled before being sent to avoid scanning
// subnets linearly. The channel is closed when exhausted or ctx is cancelled.
func (p *Pool) Stream(ctx context.Context) <-chan net.IP {
	ch := make(chan net.IP, 256)
	go func() {
		defer close(ch)
		switch p.cfg.Mode {
		case config.ModeQuick:
			p.emitIPs(ctx, ch, p.tier1)
		case config.ModeRange:
			p.emitEntries(ctx, ch, p.tier2)
			p.emitEntries(ctx, ch, p.tier3)
		case config.ModeFull:
			p.emitIPs(ctx, ch, p.tier1)
			p.emitEntries(ctx, ch, p.tier2)
			p.emitEntries(ctx, ch, p.tier3)
		}
	}()
	return ch
}

// emitIPs sends a flat IP list through ch using shuffle-within-step-window.
func (p *Pool) emitIPs(ctx context.Context, ch chan<- net.IP, ips []net.IP) {
	size := p.cfg.StepWindow
	for start := 0; start < len(ips); start += size {
		end := start + size
		if end > len(ips) {
			end = len(ips)
		}
		window := make([]net.IP, end-start)
		copy(window, ips[start:end])
		rand.Shuffle(len(window), func(i, j int) { window[i], window[j] = window[j], window[i] })
		for _, ip := range window {
			select {
			case ch <- ip:
			case <-ctx.Done():
				return
			}
		}
	}
}

// emitEntries dispatches each Entry to the appropriate emitter.
func (p *Pool) emitEntries(ctx context.Context, ch chan<- net.IP, entries []input.Entry) {
	for _, e := range entries {
		select {
		case <-ctx.Done():
			return
		default:
		}
		switch e.Type {
		case input.TypeSingle:
			select {
			case ch <- e.IP:
			case <-ctx.Done():
				return
			}
		case input.TypeCIDR:
			p.emitCIDR(ctx, ch, e.Net)
		}
	}
}

// emitCIDR streams IPs from a CIDR range using shuffle-within-step-window.
// IPs are generated lazily — the full range is never held in memory at once.
func (p *Pool) emitCIDR(ctx context.Context, ch chan<- net.IP, network *net.IPNet) {
	size := p.cfg.StepWindow
	window := make([]net.IP, 0, size)
	ip := cloneIP(network.IP)

	for network.Contains(ip) {
		select {
		case <-ctx.Done():
			return
		default:
		}

		window = append(window, cloneIP(ip))
		if len(window) == size {
			flushWindow(ctx, ch, window)
			window = window[:0]
			if ctx.Err() != nil {
				return
			}
		}
		incrementIP(ip)
	}

	// Flush any remaining IPs that did not fill a complete window.
	if len(window) > 0 {
		flushWindow(ctx, ch, window)
	}
}

// flushWindow shuffles the window in-place and sends all IPs through ch.
func flushWindow(ctx context.Context, ch chan<- net.IP, window []net.IP) {
	rand.Shuffle(len(window), func(i, j int) { window[i], window[j] = window[j], window[i] })
	for _, ip := range window {
		select {
		case ch <- ip:
		case <-ctx.Done():
			return
		}
	}
}

// cloneIP returns a copy of ip so callers own their own allocation.
func cloneIP(ip net.IP) net.IP {
	dup := make(net.IP, len(ip))
	copy(dup, ip)
	return dup
}

// incrementIP increments ip by 1 in-place (big-endian byte order).
func incrementIP(ip net.IP) {
	for i := len(ip) - 1; i >= 0; i-- {
		ip[i]++
		if ip[i] != 0 {
			break
		}
	}
}

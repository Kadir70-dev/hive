// Package clientip resolves the real client address without trusting
// forwarding headers from arbitrary callers.
//
// Trust boundary:
//   - X-Forwarded-For is honoured only when the TCP peer is inside a configured
//     trusted proxy CIDR (Caddy on the Docker network). The right-most address
//     that is not itself a trusted proxy is used, so values a client prepends
//     are ignored.
//   - X-Hive-Client-IP is honoured only when the request also carries the
//     shared proxy secret (X-Hive-Proxy-Secret), which only the Next.js server
//     on Netlify knows. Netlify Functions have no stable egress IPs, so a shared
//     secret, not an IP allow-list, authenticates that hop.
package clientip

import (
	"crypto/subtle"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

const (
	HeaderClientIP    = "X-Hive-Client-IP"
	HeaderProxySecret = "X-Hive-Proxy-Secret"
)

// Resolver extracts client IPs according to the configured trust boundary.
type Resolver struct {
	trusted []netip.Prefix
	secret  []byte
}

// New parses a comma-separated CIDR/IP list. secret may be empty, in which
// case X-Hive-Client-IP is never trusted.
func New(trustedCIDRs, proxySecret string) (*Resolver, error) {
	r := &Resolver{secret: []byte(proxySecret)}
	for _, raw := range strings.Split(trustedCIDRs, ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if !strings.Contains(raw, "/") {
			addr, err := netip.ParseAddr(raw)
			if err != nil {
				return nil, fmt.Errorf("trusted proxy %q: %w", raw, err)
			}
			r.trusted = append(r.trusted, netip.PrefixFrom(addr.Unmap(), addr.Unmap().BitLen()))
			continue
		}
		p, err := netip.ParsePrefix(raw)
		if err != nil {
			return nil, fmt.Errorf("trusted proxy %q: %w", raw, err)
		}
		r.trusted = append(r.trusted, p.Masked())
	}
	return r, nil
}

func (r *Resolver) isTrusted(a netip.Addr) bool {
	for _, p := range r.trusted {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

func parse(s string) (netip.Addr, bool) {
	s = strings.TrimSpace(s)
	if host, _, err := net.SplitHostPort(s); err == nil {
		s = host
	}
	a, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Addr{}, false
	}
	return a.Unmap(), true
}

// IP returns the best-known client address for rate limiting.
func (r *Resolver) IP(req *http.Request) string {
	peer, ok := parse(req.RemoteAddr)
	if !ok {
		return "unknown"
	}
	client := peer
	if r.isTrusted(peer) {
		hops := strings.Split(req.Header.Get("X-Forwarded-For"), ",")
		for i := len(hops) - 1; i >= 0; i-- {
			a, ok := parse(hops[i])
			if !ok {
				break
			}
			client = a
			if !r.isTrusted(a) {
				break
			}
		}
	}
	if len(r.secret) > 0 {
		given := []byte(req.Header.Get(HeaderProxySecret))
		if len(given) > 0 && subtle.ConstantTimeCompare(given, r.secret) == 1 {
			if a, ok := parse(req.Header.Get(HeaderClientIP)); ok {
				return a.String()
			}
		}
	}
	return client.String()
}

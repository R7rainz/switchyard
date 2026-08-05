package execution

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
)

// IsPrivate covers RFC1918 and IPv6 unique-local addresses, not every
// non-public range that can route to an internal service.
var blockedPrefixes = [...]netip.Prefix{
	netip.MustParsePrefix("100.64.0.0/10"), // shared address space
	netip.MustParsePrefix("192.0.0.0/24"),  // protocol assignments
	netip.MustParsePrefix("192.0.2.0/24"),  // documentation
	netip.MustParsePrefix("198.18.0.0/15"), // benchmarking
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("2001:db8::/32"), // IPv6 documentation
	netip.MustParsePrefix("240.0.0.0/4"),   // reserved
}

// lookupIPFunc is injectable only so the runner can be tested without making
// a real request. Production always uses the system resolver.
type lookupIPFunc func(context.Context, string) ([]net.IP, error)

func lookupHost(ctx context.Context, host string) ([]net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		return []net.IP{ip}, nil
	}
	return net.DefaultResolver.LookupIP(ctx, "ip", host)
}

// newSSRFProtectedClient disables proxy environment variables and pins each
// connection to an address checked immediately before dialing. URL checks
// alone leave a DNS-rebinding gap between validation and the transport dial.
func newSSRFProtectedClient(client *http.Client, lookup lookupIPFunc) *http.Client {
	if lookup == nil {
		lookup = lookupHost
	}
	if client == nil {
		client = &http.Client{}
	}

	copy := *client
	copy.CheckRedirect = rejectRedirect
	base := copy.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	transport, ok := base.(*http.Transport)
	if !ok {
		// A custom RoundTripper owns its own dial policy. The request-level
		// DNS check still applies, but it cannot be safely rewritten here.
		return &copy
	}

	transport = transport.Clone()
	transport.Proxy = nil
	transport.Dial = nil
	transport.DialTLS = nil
	transport.DialTLSContext = nil
	transport.DialContext = guardedDialContext(lookup)
	copy.Transport = transport
	return &copy
}

func guardedDialContext(lookup lookupIPFunc) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("http node address is invalid: %w", err)
		}
		ips, err := lookup(ctx, strings.TrimSuffix(host, "."))
		if err != nil {
			return nil, fmt.Errorf("http node host lookup failed: %w", err)
		}
		if err := validateIPs(ips); err != nil {
			return nil, err
		}

		var lastErr error
		for _, ip := range ips {
			if strings.HasSuffix(network, "4") && ip.To4() == nil {
				continue
			}
			if strings.HasSuffix(network, "6") && ip.To4() != nil {
				continue
			}
			conn, err := (&net.Dialer{}).DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if err == nil {
				return conn, nil
			}
			lastErr = err
		}
		if lastErr == nil {
			lastErr = fmt.Errorf("no address matches network %s", network)
		}
		return nil, lastErr
	}
}

func validateHTTPURL(ctx context.Context, raw string, lookup lookupIPFunc) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("http node url is invalid: %w", err)
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("http node only supports http and https urls")
	}
	if parsed.Hostname() == "" {
		return fmt.Errorf("http node url needs a host")
	}
	if parsed.User != nil {
		return fmt.Errorf("http node url must not contain userinfo")
	}
	if lookup == nil {
		lookup = lookupHost
	}
	ips, err := lookup(ctx, strings.TrimSuffix(parsed.Hostname(), "."))
	if err != nil {
		return fmt.Errorf("http node host lookup failed: %w", err)
	}
	if err := validateIPs(ips); err != nil {
		return err
	}
	return nil
}

func validateIPs(ips []net.IP) error {
	if len(ips) == 0 {
		return fmt.Errorf("http node host has no addresses")
	}
	for _, ip := range ips {
		addr, ok := netip.AddrFromSlice(ip)
		if !ok {
			return fmt.Errorf("http node host returned an invalid address")
		}
		addr = addr.Unmap()
		if addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast() ||
			addr.IsLinkLocalMulticast() || addr.IsMulticast() || addr.IsUnspecified() || blocked(addr) {
			return fmt.Errorf("http node cannot reach private or local addresses")
		}
	}
	return nil
}

func blocked(addr netip.Addr) bool {
	for _, prefix := range blockedPrefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

func rejectRedirect(_ *http.Request, _ []*http.Request) error {
	return fmt.Errorf("http node redirects are not allowed")
}

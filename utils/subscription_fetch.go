package utils

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

const SubscriptionResponseLimit int64 = 4 << 20

// FetchSubscription is the sole subscription network boundary. A supplied transport
// contributes only its TLS policy and dialer (e.g. a configured proxy adapter).
// The dialer always receives a validated numeric target, never a DNS hostname.
// The returned body is buffered, bounded decoded data; network resources are closed.
func FetchSubscription(req *http.Request, base *http.Client) (*http.Response, error) {
	return fetchSubscription(req, base, net.DefaultResolver.LookupIP)
}

func fetchSubscription(req *http.Request, base *http.Client, lookup func(context.Context, string, string) ([]net.IP, error)) (*http.Response, error) {
	ctx, cancel := context.WithTimeout(req.Context(), 30*time.Second)
	defer cancel()
	req = req.Clone(ctx)
	resolve := func(ctx context.Context, host string) ([]net.IP, error) {
		ips, err := lookup(ctx, "ip", host)
		if err != nil || len(ips) == 0 {
			return nil, errors.New("subscription host resolution failed")
		}
		for _, ip := range ips {
			if IsDisallowedSubscriptionIP(ip) {
				return nil, errors.New("subscription destination is not allowed")
			}
		}
		return ips, nil
	}
	validate := func(r *http.Request) error {
		if r.URL == nil || (r.URL.Scheme != "http" && r.URL.Scheme != "https") || r.URL.Hostname() == "" || r.URL.User != nil {
			return errors.New("invalid subscription URL")
		}
		_, err := resolve(r.Context(), r.URL.Hostname())
		return err
	}
	dial := (&net.Dialer{Timeout: 8 * time.Second}).DialContext
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}, ResponseHeaderTimeout: 15 * time.Second, DisableCompression: true}
	if base != nil {
		if t, ok := base.Transport.(*http.Transport); ok {
			if t.DialContext != nil {
				dial = t.DialContext
			}
			if t.TLSClientConfig != nil {
				transport.TLSClientConfig = t.TLSClientConfig.Clone()
			}
		}
	}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		ips, err := resolve(ctx, host)
		if err != nil {
			return nil, err
		}
		return dial(ctx, network, net.JoinHostPort(ips[0].String(), port))
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 30 * time.Second}
	client.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many subscription redirects")
		}
		if err := validate(r); err != nil {
			return err
		}
		if base != nil && base.CheckRedirect != nil {
			if err := base.CheckRedirect(r, via); err != nil {
				return err
			}
		}
		if len(via) > 0 && !strings.EqualFold(r.URL.Host, via[len(via)-1].URL.Host) {
			r.Header = make(http.Header) //nolint:gosec // G119: cross-origin redirects discard all headers after destination validation.
		}
		return nil
	}
	if err := validate(req); err != nil {
		return nil, err
	}
	req.Header = req.Header.Clone()
	req.Header.Set("Accept-Encoding", "gzip")
	resp, err := client.Do(req) //nolint:gosec // G704: initial and redirect URLs are validated; DialContext revalidates DNS and pins a public numeric IP.
	if resp != nil && resp.Body != nil {
		// Capture the network body before replacing it with the buffered payload.
		defer func(body io.Closer) { _ = body.Close() }(resp.Body)
	}
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("subscription HTTP status %d", resp.StatusCode)
	}
	var reader io.Reader = resp.Body
	switch strings.ToLower(resp.Header.Get("Content-Encoding")) {
	case "gzip":
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, err
		}
		// Closing a read-only gzip reader cannot fail and does not close the network body.
		defer func() { _ = gz.Close() }()
		reader = gz
	case "", "identity":
	default:
		return nil, errors.New("unsupported subscription content encoding")
	}
	data, err := io.ReadAll(io.LimitReader(reader, SubscriptionResponseLimit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > SubscriptionResponseLimit {
		return nil, errors.New("subscription response exceeds size limit")
	}
	resp.Body = io.NopCloser(bytes.NewReader(data))
	resp.ContentLength = int64(len(data))
	resp.Header.Del("Content-Encoding")
	return resp, nil
}

var disallowedSubscriptionPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("224.0.0.0/4"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("::/96"),
	netip.MustParsePrefix("::/128"),
	netip.MustParsePrefix("::1/128"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001::/32"),
	netip.MustParsePrefix("2001:2::/48"),
	netip.MustParsePrefix("2001:10::/28"),
	netip.MustParsePrefix("2001:20::/28"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("fec0::/10"),
	netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("ff00::/8"),
}

func IsDisallowedSubscriptionIP(ip net.IP) bool {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return true
	}
	addr = addr.Unmap()
	if !addr.IsGlobalUnicast() {
		return true
	}
	for _, prefix := range disallowedSubscriptionPrefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

func ValidateSubscriptionDestination(ctx context.Context, rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Hostname() == "" || (!strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https")) {
		return errors.New("invalid subscription URL")
	}
	addresses, err := net.DefaultResolver.LookupIP(ctx, "ip", parsed.Hostname())
	if err != nil || len(addresses) == 0 {
		return errors.New("subscription host resolution failed")
	}
	for _, ip := range addresses {
		if IsDisallowedSubscriptionIP(ip) {
			return errors.New("subscription destination is not allowed")
		}
	}
	return nil
}

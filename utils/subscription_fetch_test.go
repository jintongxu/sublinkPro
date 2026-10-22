package utils

import (
	"compress/gzip"
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSubscriptionFetchPolicy(t *testing.T) {
	for _, raw := range []string{"http://127.0.0.1", "http://169.254.169.254", "http://[::1]", "file:///etc/passwd"} {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, raw, nil)
		resp, err := FetchSubscription(req, nil)
		if resp != nil && resp.Body != nil {
			defer func(body io.Closer) { _ = body.Close() }(resp.Body)
		}
		if err == nil {
			t.Fatalf("allowed %s", raw)
		}
	}
}
func TestSubscriptionFetchBoundaries(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/large":
			w.Header().Set("Content-Encoding", "gzip")
			gz := gzip.NewWriter(w)
			_, _ = io.Copy(gz, strings.NewReader(strings.Repeat("x", int(SubscriptionResponseLimit+1))))
			_ = gz.Close()
		case "/redirect":
			http.Redirect(w, r, "http://127.0.0.1/private", http.StatusFound)
		case "/loop":
			http.Redirect(w, r, "/loop", http.StatusFound)
		case "/status":
			w.WriteHeader(503)
		default:
			_, _ = w.Write([]byte("ok"))
		}
	}))
	defer server.Close()
	lookup := func(ctx context.Context, network, host string) ([]net.IP, error) {
		if host == "public.example" {
			return []net.IP{net.ParseIP("8.8.8.8")}, nil
		}
		return []net.IP{net.ParseIP(host)}, nil
	}
	transport := &http.Transport{
		// Only the local httptest TLS fixture bypasses verification; the strict case below restores it.
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // G402: self-signed httptest fixture, dialer connects only to the local test server.
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			if !strings.HasPrefix(address, "8.8.8.8:") {
				t.Errorf("unpinned dial: %s", address)
			}
			return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
		}}
	base := &http.Client{Transport: transport}
	for _, path := range []string{"/", "/large", "/redirect", "/loop", "/status"} {
		t.Run(path, func(t *testing.T) {
			req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://public.example"+path, nil)
			resp, err := fetchSubscription(req, base, lookup)
			if resp != nil && resp.Body != nil {
				defer func(body io.Closer) { _ = body.Close() }(resp.Body)
			}
			if path != "/" {
				if err == nil {
					t.Fatal("expected failure")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(resp.Body)
			if string(body) != "ok" {
				t.Fatal(string(body))
			}
		})
	}
	transport.TLSClientConfig.InsecureSkipVerify = false
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://public.example/", nil)
	resp, err := fetchSubscription(req, base, lookup)
	if resp != nil && resp.Body != nil {
		defer func(body io.Closer) { _ = body.Close() }(resp.Body)
	}
	if err == nil {
		t.Fatal("TLS validation bypassed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req, _ = http.NewRequestWithContext(ctx, http.MethodGet, "https://public.example/", nil)
	resp, err = fetchSubscription(req, base, lookup)
	if resp != nil && resp.Body != nil {
		defer func(body io.Closer) { _ = body.Close() }(resp.Body)
	}
	if err == nil {
		t.Fatal("cancellation ignored")
	}
}

func TestSubscriptionFetchRedirectStripsHeadersAndRebinding(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "http://other.example/end", http.StatusFound)
			return
		}
		if r.Header.Get("X-Secret") != "" || r.Header.Get("Authorization") != "" {
			t.Error("cross-host credentials leaked")
		}
		if _, err := w.Write([]byte("ok")); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	lookup := func(context.Context, string, string) ([]net.IP, error) { return []net.IP{net.ParseIP("8.8.8.8")}, nil }
	base := &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}}}
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://public.example/start", nil)
	req.Header.Set("X-Secret", "private")
	req.Header.Set("Authorization", "Bearer secret")
	resp, err := fetchSubscription(req, base, lookup)
	if err != nil {
		t.Fatal(err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Error(err)
	}
	calls := 0
	rebinding := func(context.Context, string, string) ([]net.IP, error) {
		calls++
		if calls == 1 {
			return []net.IP{net.ParseIP("8.8.8.8")}, nil
		}
		return []net.IP{net.ParseIP("127.0.0.1")}, nil
	}
	resp, err = fetchSubscription(req, base, rebinding)
	if resp != nil && resp.Body != nil {
		defer func(body io.Closer) { _ = body.Close() }(resp.Body)
	}
	if err == nil {
		t.Fatal("DNS rebinding accepted")
	}
}

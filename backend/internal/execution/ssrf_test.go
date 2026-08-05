package execution

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func TestHTTPRunnerRejectsPrivateAndLocalAddresses(t *testing.T) {
	runner := &httpRunner{client: &http.Client{}}
	for _, raw := range []string{
		"http://127.0.0.1/",
		"http://[::1]/",
		"http://10.0.0.1/",
		"http://169.254.169.254/latest/meta-data/",
	} {
		t.Run(raw, func(t *testing.T) {
			_, err := runner.Run(context.Background(), Input{Data: []byte(`{"url":"` + raw + `"}`)})
			if err == nil || !strings.Contains(err.Error(), "private or local") {
				t.Fatalf("Run error = %v, want private-address rejection", err)
			}
		})
	}
}

func TestHTTPRunnerRejectsPrivateDNSResults(t *testing.T) {
	runner := &httpRunner{
		client: &http.Client{},
		lookupIP: func(context.Context, string) ([]net.IP, error) {
			return []net.IP{net.ParseIP("192.168.1.20")}, nil
		},
	}
	_, err := runner.Run(context.Background(), Input{Data: []byte(`{"url":"https://example.com"}`)})
	if err == nil || !strings.Contains(err.Error(), "private or local") {
		t.Fatalf("Run error = %v, want private DNS rejection", err)
	}
}

func TestHTTPRunnerRejectsRedirects(t *testing.T) {
	runner := &httpRunner{
		client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusFound,
				Header:     http.Header{"Location": []string{"https://example.com/next"}},
				Body:       io.NopCloser(strings.NewReader("")),
				Request:    req,
			}, nil
		})},
		lookupIP: func(context.Context, string) ([]net.IP, error) {
			return []net.IP{net.ParseIP("93.184.216.34")}, nil
		},
	}
	_, err := runner.Run(context.Background(), Input{Data: []byte(`{"url":"https://example.com"}`)})
	if err == nil || !strings.Contains(err.Error(), "redirects are not allowed") {
		t.Fatalf("Run error = %v, want redirect rejection", err)
	}
}

func TestHTTPRunnerBlocksDNSRebinding(t *testing.T) {
	var calls atomic.Int32
	lookup := func(context.Context, string) ([]net.IP, error) {
		if calls.Add(1) == 1 {
			return []net.IP{net.ParseIP("93.184.216.34")}, nil
		}
		return []net.IP{net.ParseIP("10.0.0.8")}, nil
	}
	runner := &httpRunner{
		client:   newSSRFProtectedClient(&http.Client{Transport: &http.Transport{}}, lookup),
		lookupIP: lookup,
	}
	_, err := runner.Run(context.Background(), Input{Data: []byte(`{"url":"https://example.com"}`)})
	if err == nil || !strings.Contains(err.Error(), "private or local") {
		t.Fatalf("Run error = %v, want DNS-rebinding rejection", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

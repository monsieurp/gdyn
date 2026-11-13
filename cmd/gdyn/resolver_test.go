package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

// Exercise HTTP request handling without external services or listening sockets.
func handlerClient(handler http.HandlerFunc) *http.Client {
	client := newHTTPClient(false)
	client.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if err := request.Context().Err(); err != nil {
			return nil, err
		}
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		return recorder.Result(), nil
	})
	return client
}

func TestResolveIP(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		want       string
	}{
		{"IPv4", "1.1.1.1", 200, "1.1.1.1"},
		{"whitespace", " \n8.8.8.8\r\n", 200, "8.8.8.8"},
		{"invalid octets", "999.999.999.999", 200, ""},
		{"IPv6", "2606:4700:4700::1111", 200, ""},
		{"private", "192.168.1.1", 200, ""},
		{"loopback", "127.0.0.1", 200, ""},
		{"multicast", "224.0.0.1", 200, ""},
		{"unspecified", "0.0.0.0", 200, ""},
		{"HTML", "<p>1.1.1.1</p>", 200, ""},
		{"multiple addresses", "1.1.1.1\n8.8.8.8", 200, ""},
		{"empty", "", 200, ""},
		{"oversized", strings.Repeat(" ", 4097) + "1.1.1.1", 200, ""},
		{"HTTP error", "1.1.1.1", 503, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := handlerClient(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.Header.Get("Accept") != "text/plain" {
					t.Errorf("unexpected resolver request: %s %v", r.Method, r.Header)
				}
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			})
			ip, err := resolveIP(context.Background(), client, "https://resolver.example.com/ip")
			if tc.want == "" {
				if err == nil {
					t.Fatalf("accepted invalid response: %s", ip)
				}
			} else if err != nil || ip.String() != tc.want {
				t.Fatalf("got %s, %v; want %s", ip, err, tc.want)
			}
		})
	}
}

func TestResolveIPTransportFailure(t *testing.T) {
	for _, cause := range []error{context.DeadlineExceeded, context.Canceled, errors.New("connection failed")} {
		client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, cause
		})}
		if _, err := resolveIP(context.Background(), client, "https://resolver.example.com"); !errors.Is(err, cause) {
			t.Fatalf("got %v; want wrapped %v", err, cause)
		}
	}
}

func TestResolverRejectsRedirect(t *testing.T) {
	calls := 0
	client := handlerClient(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Location", "https://other.example.com/ip")
		w.WriteHeader(http.StatusFound)
	})
	_, err := resolveIP(context.Background(), client, "https://resolver.example.com")
	if err == nil || calls != 1 {
		t.Fatalf("redirect: err=%v, requests=%d; want error after one request", err, calls)
	}
}

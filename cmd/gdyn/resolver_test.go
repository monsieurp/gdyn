package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

// Exercise HTTP request handling without external services or listening sockets.
func handlerClient(handler http.HandlerFunc) *retryClient {
	client := newHTTPClient(false)
	client.wait = skipRetryWait
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

func skipRetryWait(ctx context.Context, _ time.Duration) error {
	return ctx.Err()
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
		client := &retryClient{Client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, cause
		})}, wait: skipRetryWait}
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

func TestResolverFallback(t *testing.T) {
	for _, tc := range []struct {
		name, primaryBody, fallbackBody, wantIP string
		primaryStatus, fallbackStatus           int
		primaryError                            error
		disabled                                bool
		wantCalls                               []string
		wantError                               string
	}{
		{
			name: "primary success skips fallback", primaryStatus: 200, primaryBody: "1.1.1.1",
			wantIP: "1.1.1.1", wantCalls: []string{"primary.example.com"},
		},
		{
			name: "HTTP error uses fallback", primaryStatus: 503, fallbackStatus: 200, fallbackBody: "8.8.8.8",
			wantIP: "8.8.8.8", wantCalls: []string{"primary.example.com", "primary.example.com", "primary.example.com", "fallback.example.com"},
		},
		{
			name: "invalid IP uses fallback", primaryStatus: 200, primaryBody: "999.999.999.999", fallbackStatus: 200, fallbackBody: "8.8.8.8",
			wantIP: "8.8.8.8", wantCalls: []string{"primary.example.com", "fallback.example.com"},
		},
		{
			name: "timeout uses fallback", primaryError: context.DeadlineExceeded, fallbackStatus: 200, fallbackBody: "8.8.8.8",
			wantIP: "8.8.8.8", wantCalls: []string{"primary.example.com", "primary.example.com", "primary.example.com", "fallback.example.com"},
		},
		{
			name: "connection error uses fallback", primaryError: errors.New("connection failed"), fallbackStatus: 200, fallbackBody: "8.8.8.8",
			wantIP: "8.8.8.8", wantCalls: []string{"primary.example.com", "fallback.example.com"},
		},
		{
			name: "disabled fallback", primaryStatus: 503, disabled: true,
			wantCalls: []string{"primary.example.com", "primary.example.com", "primary.example.com"}, wantError: "primary resolver failed",
		},
		{
			name: "both services fail", primaryStatus: 503, fallbackStatus: 502,
			wantCalls: []string{"primary.example.com", "primary.example.com", "primary.example.com", "fallback.example.com", "fallback.example.com", "fallback.example.com"}, wantError: "fallback resolver failed",
		},
		{
			name: "fallback IP must also be valid", primaryStatus: 503, fallbackStatus: 200, fallbackBody: "192.168.1.1",
			wantCalls: []string{"primary.example.com", "primary.example.com", "primary.example.com", "fallback.example.com"}, wantError: "fallback resolver failed",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls []string
			var logs bytes.Buffer
			client := handlerClient(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Host == "primary.example.com" {
					w.WriteHeader(tc.primaryStatus)
					_, _ = io.WriteString(w, tc.primaryBody)
				} else {
					w.WriteHeader(tc.fallbackStatus)
					_, _ = io.WriteString(w, tc.fallbackBody)
				}
			})
			transport := client.Transport
			client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls = append(calls, r.URL.Host)
				if r.URL.Host == "primary.example.com" && tc.primaryError != nil {
					return nil, tc.primaryError
				}
				return transport.RoundTrip(r)
			})
			fallbackURL := "https://fallback.example.com/ip"
			if tc.disabled {
				fallbackURL = ""
			}
			ip, err := resolvePublicIP(context.Background(), client, "https://primary.example.com/ip", fallbackURL, newLogger(&logs, false))
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) || !strings.Contains(err.Error(), "primary resolver failed") || ip.IsValid() {
					t.Fatalf("got IP %s, error %v; want %q and no usable IP", ip, err, tc.wantError)
				}
			} else if err != nil || ip.String() != tc.wantIP {
				t.Fatalf("got IP %s, error %v; want %s", ip, err, tc.wantIP)
			}
			if !reflect.DeepEqual(calls, tc.wantCalls) {
				t.Errorf("requests: got %v; want %v", calls, tc.wantCalls)
			}
			usedFallback := tc.wantCalls[len(tc.wantCalls)-1] == "fallback.example.com"
			if strings.Contains(logs.String(), " WARN  gdyn  ") != usedFallback {
				t.Errorf("unexpected fallback warning: %s", &logs)
			}
		})
	}
}

func TestResolverCancellationSkipsFallback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	client := &retryClient{Client: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		cancel()
		return nil, r.Context().Err()
	})}, wait: skipRetryWait}
	var logs bytes.Buffer
	_, err := resolvePublicIP(ctx, client, "https://primary.example.com/ip", "https://fallback.example.com/ip", newLogger(&logs, false))
	if !errors.Is(err, context.Canceled) || calls != 1 || logs.Len() != 0 {
		t.Fatalf("cancellation: got error %v, %d requests, logs %q", err, calls, &logs)
	}
}

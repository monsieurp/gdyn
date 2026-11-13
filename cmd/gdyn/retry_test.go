package main

import (
	"context"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestRetryHTTPStatuses(t *testing.T) {
	for _, tc := range []struct {
		name, retryAfter string
		status, calls    int
	}{
		{"success", "", 200, 1},
		{"rate limit", "", 429, 3},
		{"server error", "", 500, 3},
		{"unavailable", "", 503, 3},
		{"unauthorized", "", 401, 1},
		{"forbidden", "", 403, 1},
		{"not found", "", 404, 1},
		{"excessive server wait", "11", 429, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			var waits []time.Duration
			client := handlerClient(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Retry-After", tc.retryAfter)
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, "response")
			})
			client.wait = func(ctx context.Context, delay time.Duration) error {
				waits = append(waits, delay)
				return ctx.Err()
			}
			request, err := http.NewRequest(http.MethodGet, "https://api.example.com", nil)
			if err != nil {
				t.Fatal(err)
			}
			response, err := client.do(request, 4096)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if calls != tc.calls || response.StatusCode != tc.status || len(waits) != tc.calls-1 {
				t.Fatalf("got %d calls, status %d, waits %v; want %d calls and status %d", calls, response.StatusCode, waits, tc.calls, tc.status)
			}
			if tc.calls == 3 && !reflect.DeepEqual(waits, []time.Duration{time.Second, 2 * time.Second}) {
				t.Fatalf("unexpected waits: %v", waits)
			}
		})
	}
}

func TestGandiRetryReplaysPUT(t *testing.T) {
	var bodies []string
	client := &gandiClient{baseURL: "https://api.example.com", token: "test-token"}
	client.http = handlerClient(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		bodies = append(bodies, string(body))
		if r.Method != http.MethodPut || r.URL.Path != "/domains/example.com/records/@/A" || r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("retry changed the request: %s %s %v", r.Method, r.URL, r.Header)
		}
		if len(bodies) < 3 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	if err := client.update(context.Background(), "example.com", "@", "1.1.1.1", 3600); err != nil {
		t.Fatal(err)
	}
	if len(bodies) != 3 {
		t.Fatalf("got %d attempts; want 3", len(bodies))
	}
	for _, body := range bodies {
		if body != `{"rrset_values":["1.1.1.1"],"rrset_ttl":3600}` {
			t.Errorf("retry lost or changed the PUT body: %q", body)
		}
	}
}

func TestRetryNetworkErrors(t *testing.T) {
	for _, tc := range []struct {
		name  string
		cause error
		retry bool
	}{
		{"timeout", context.DeadlineExceeded, true},
		{"connection reset", &net.OpError{Op: "read", Net: "tcp", Err: errors.New("connection reset")}, true},
		{"EOF", io.EOF, true},
		{"truncated body", io.ErrUnexpectedEOF, true},
		{"cancellation", context.Canceled, false},
		{"certificate", x509.UnknownAuthorityError{}, false},
		{"nonexistent host", &net.DNSError{IsNotFound: true, Name: "missing.example.com"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			client := newHTTPClient(false)
			client.wait = skipRetryWait
			client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					return nil, tc.cause
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("1.1.1.1"))}, nil
			})
			ip, err := resolveIP(context.Background(), client, "https://resolver.example.com")
			if tc.retry {
				if err != nil || ip.String() != "1.1.1.1" || calls != 2 {
					t.Fatalf("got %s, %v after %d calls; want recovery on attempt 2", ip, err, calls)
				}
			} else if !errors.Is(err, tc.cause) || calls != 1 {
				t.Fatalf("got %v after %d calls; want original error without retry", err, calls)
			}
		})
	}
}

type failingResponseBody struct {
	closed bool
}

func (body *failingResponseBody) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func (body *failingResponseBody) Close() error {
	body.closed = true
	return nil
}

func TestRetryBodyReadFailure(t *testing.T) {
	first := &failingResponseBody{}
	calls := 0
	client := newHTTPClient(false)
	client.wait = skipRetryWait
	client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		var body io.ReadCloser = first
		if calls > 1 {
			if !first.closed {
				t.Error("previous response body was not closed before retry")
			}
			body = io.NopCloser(strings.NewReader("1.1.1.1"))
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: body}, nil
	})
	ip, err := resolveIP(context.Background(), client, "https://resolver.example.com")
	if err != nil || ip.String() != "1.1.1.1" || calls != 2 {
		t.Fatalf("got %s, %v after %d calls; want recovery after a body read error", ip, err, calls)
	}
}

func TestRetryAfter(t *testing.T) {
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, header string
		delay        time.Duration
		allowed      bool
	}{
		{"absent", "", time.Second, true},
		{"invalid", "invalid", time.Second, true},
		{"negative", "-1", time.Second, true},
		{"immediate", "0", 0, true},
		{"seconds", " 5 ", 5 * time.Second, true},
		{"limit", "10", 10 * time.Second, true},
		{"too long", "11", 0, false},
		{"overflow", "999999999999999999999999", 0, false},
		{"date", now.Add(5 * time.Second).Format(http.TimeFormat), 5 * time.Second, true},
		{"past date", now.Add(-time.Second).Format(http.TimeFormat), 0, true},
		{"distant date", now.Add(time.Minute).Format(http.TimeFormat), 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			delay, allowed := retryDelay(tc.header, 1, now)
			if delay != tc.delay || allowed != tc.allowed {
				t.Fatalf("got %s, %v; want %s, %v", delay, allowed, tc.delay, tc.allowed)
			}
		})
	}
}

func TestRetryCancellationDuringWait(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	client := handlerClient(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	client.wait = func(ctx context.Context, delay time.Duration) error {
		cancel()
		return waitForRetry(ctx, delay)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.do(request, 4096)
	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("got %v after %d requests; want cancellation without another attempt", err, calls)
	}
}

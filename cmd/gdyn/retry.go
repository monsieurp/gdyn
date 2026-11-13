package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type retryClient struct {
	*http.Client
	wait func(context.Context, time.Duration) error
}

func (client *retryClient) do(request *http.Request, bodyLimit int64) (*http.Response, error) {
	attempts := 3
	if request.Method != http.MethodGet && request.Method != http.MethodPut || request.Body != nil && request.GetBody == nil {
		attempts = 1
	}
	for attempt := 1; ; attempt++ {
		if err := request.Context().Err(); err != nil {
			return nil, err
		}
		next := request.Clone(request.Context())
		if attempt > 1 && request.GetBody != nil {
			body, err := request.GetBody()
			if err != nil {
				return nil, fmt.Errorf("replay HTTP request: %w", err)
			}
			next.Body = body
		}
		response, err := client.Client.Do(next)
		if err == nil {
			if attempt < attempts && (response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500 && response.StatusCode <= 599) {
				delay, allowed := retryDelay(response.Header.Get("Retry-After"), attempt, time.Now())
				if allowed {
					response.Body.Close()
					if err := client.wait(request.Context(), delay); err != nil {
						return nil, err
					}
					continue
				}
			}
			// Buffer the bounded response so timeouts during reads can also retry.
			body, readErr := io.ReadAll(io.LimitReader(response.Body, bodyLimit+1))
			response.Body.Close()
			if int64(len(body)) > bodyLimit {
				return nil, fmt.Errorf("HTTP response exceeds %d bytes", bodyLimit)
			}
			if readErr == nil {
				response.Body = io.NopCloser(bytes.NewReader(body))
				return response, nil
			}
			if response.StatusCode < 200 || response.StatusCode >= 300 {
				return nil, readErr
			}
			err = readErr
		}
		if attempt >= attempts || !retryableNetworkError(err) {
			return nil, err
		}
		if err := client.wait(request.Context(), time.Duration(attempt)*time.Second); err != nil {
			return nil, err
		}
	}
}

func retryableNetworkError(err error) bool {
	if errors.Is(err, context.Canceled) {
		return false
	}
	var dnsError *net.DNSError
	if errors.As(err, &dnsError) && !dnsError.IsTimeout && !dnsError.IsTemporary {
		return false
	}
	var networkError net.Error
	var operationError *net.OpError
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, context.DeadlineExceeded) || errors.As(err, &operationError) ||
		errors.As(err, &networkError) && (networkError.Timeout() || networkError.Temporary())
}

func retryDelay(header string, attempt int, now time.Time) (time.Duration, bool) {
	const maxWait = 10 * time.Second
	header = strings.TrimSpace(header)
	if seconds, err := strconv.ParseUint(header, 10, 64); err == nil {
		if seconds > uint64(maxWait/time.Second) {
			return 0, false
		}
		return time.Duration(seconds) * time.Second, true
	}
	if date, err := http.ParseTime(header); err == nil {
		delay := date.Sub(now)
		if delay > maxWait {
			return 0, false
		}
		return max(0, delay), true
	}
	// An overflowing delta-seconds value still asks for more than the limit.
	if header != "" && strings.Trim(header, "0123456789") == "" {
		return 0, false
	}
	return time.Duration(attempt) * time.Second, true
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return ctx.Err()
	}
}

package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestGandiRequests(t *testing.T) {
	for _, baseURL := range []string{"https://api.example.com/custom", "https://api.example.com/custom/"} {
		t.Run(baseURL, func(t *testing.T) {
			methods := []string{}
			client := &gandiClient{baseURL: baseURL, token: "test-token"}
			client.http = handlerClient(func(w http.ResponseWriter, r *http.Request) {
				methods = append(methods, r.Method)
				if r.URL.Host != "api.example.com" || r.Header.Get("Authorization") != "Bearer test-token" || r.Header.Get("X-Api-Key") != "" || r.Header.Get("Accept") != "application/json" {
					t.Errorf("unexpected destination or authentication: %s %v", r.URL, r.Header)
				}
				switch r.Method {
				case http.MethodGet:
					if r.URL.Path != "/custom/domains/example.com/records" {
						t.Errorf("GET path: %s", r.URL.Path)
					}
					_, _ = io.WriteString(w, `[{"rrset_name":"@","rrset_type":"A","rrset_values":["1.1.1.1"],"rrset_ttl":3600}]`)
				case http.MethodPut:
					if r.URL.Path != "/custom/domains/example.com/records/@/A" || r.Header.Get("Content-Type") != "application/json" {
						t.Errorf("PUT path or content type: %s %v", r.URL.Path, r.Header)
					}
					var payload map[string]json.RawMessage
					if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
						t.Error(err)
					}
					if len(payload) != 2 || string(payload["rrset_values"]) != `["8.8.8.8"]` || string(payload["rrset_ttl"]) != "3600" {
						t.Errorf("unexpected update body: %s", payload)
					}
					w.WriteHeader(http.StatusNoContent)
				default:
					t.Errorf("unexpected method: %s", r.Method)
				}
			})
			records, err := client.records(context.Background(), "example.com")
			want := []recordSet{{Name: "@", Type: "A", Values: []string{"1.1.1.1"}, TTL: 3600}}
			if err != nil || !reflect.DeepEqual(records, want) {
				t.Fatalf("records: got %+v, %v; want %+v", records, err, want)
			}
			if err := client.update(context.Background(), "example.com", "@", "8.8.8.8", 3600); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(methods, []string{"GET", "PUT"}) {
				t.Fatalf("unexpected requests: %v", methods)
			}
		})
	}
}

func TestGandiErrors(t *testing.T) {
	for _, tc := range []struct {
		name, body, want string
		status           int
	}{
		{"authentication", `{"message":"expired test-token"}`, "Gandi HTTP 403: expired [redacted]", 403},
		{"plain error", "upstream unavailable", "Gandi HTTP 502: Bad Gateway", 502},
		{"invalid JSON", "not JSON", "decode Gandi response", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			client := &gandiClient{baseURL: "https://api.example.com", token: "test-token"}
			client.http = handlerClient(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			})
			_, err := client.records(context.Background(), "example.com")
			if err == nil || !strings.Contains(err.Error(), tc.want) || strings.Contains(err.Error(), "test-token") || calls != 1 {
				t.Fatalf("got %v after %d requests; want %q without token or retry", err, calls, tc.want)
			}
		})
	}
}

func TestGandiCancellation(t *testing.T) {
	client := &gandiClient{baseURL: "https://api.example.com", token: "test-token"}
	client.http = handlerClient(func(w http.ResponseWriter, r *http.Request) {
		t.Error("canceled request reached handler")
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.records(ctx, "example.com"); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v; want cancellation", err)
	}
}

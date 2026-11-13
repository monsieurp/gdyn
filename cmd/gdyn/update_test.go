package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/netip"
	"reflect"
	"strings"
	"testing"
)

func TestUpdateRecords(t *testing.T) {
	a := func(name, ip string, ttl int) recordSet {
		return recordSet{Name: name, Type: "A", Values: []string{ip}, TTL: ttl}
	}
	for _, tc := range []struct {
		name          string
		records       []recordSet
		wantWrites    []string
		dryRun, force bool
		failRecord    string
		fetchError    bool
		wantError     bool
		wantLog       string
	}{
		{
			name:       "matching first record does not hide stale second record",
			records:    []recordSet{a("@", "1.1.1.1", 3600), a("www", "8.8.8.8", 3600)},
			wantWrites: []string{"www"}, wantLog: "record updated",
		},
		{
			name:    "matching records need no writes",
			records: []recordSet{a("@", "1.1.1.1", 3600), a("www", "1.1.1.1", 3600)},
			wantLog: "record unchanged",
		},
		{
			name:    "force updates matching records",
			records: []recordSet{a("@", "1.1.1.1", 3600), a("www", "1.1.1.1", 3600)},
			force:   true, wantWrites: []string{"@", "www"},
		},
		{
			name: "dry run makes no writes", dryRun: true,
			records: []recordSet{a("@", "8.8.8.8", 3600)}, wantLog: "would update record",
		},
		{
			name: "force cannot override dry run", dryRun: true, force: true,
			records: []recordSet{a("@", "1.1.1.1", 3600), a("www", "1.1.1.1", 3600)}, wantLog: "would update record",
		},
		{
			name:    "TTL change triggers update",
			records: []recordSet{a("@", "1.1.1.1", 300), a("www", "1.1.1.1", 3600)}, wantWrites: []string{"@"},
		},
		{
			name: "replace multiple addresses with one",
			records: []recordSet{
				{Name: "@", Type: "A", Values: []string{"1.1.1.1", "8.8.8.8"}, TTL: 3600},
				a("www", "1.1.1.1", 3600),
			}, wantWrites: []string{"@"},
		},
		{
			name: "create missing records", wantWrites: []string{"@", "www"},
		},
		{
			name:       "CNAME conflict does not stop other records",
			records:    []recordSet{{Name: "@", Type: "CNAME", Values: []string{"other.example.com."}}},
			wantWrites: []string{"www"}, wantError: true, wantLog: "CNAME",
		},
		{
			name: "continue after partial failure", failRecord: "@",
			wantWrites: []string{"@", "www"}, wantError: true, wantLog: "record update failed",
		},
		{
			name: "failed fetch prevents writes", fetchError: true, wantError: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var writes []string
			var logs bytes.Buffer
			client := &gandiClient{baseURL: "https://api.example.com", token: "test-token"}
			client.http = handlerClient(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case http.MethodGet:
					if tc.fetchError {
						w.WriteHeader(http.StatusForbidden)
						return
					}
					if err := json.NewEncoder(w).Encode(tc.records); err != nil {
						t.Error(err)
					}
				case http.MethodPut:
					name := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/domains/example.com/records/"), "/A")
					writes = append(writes, name)
					var payload recordSet
					if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
						t.Error(err)
					}
					if payload.TTL != 3600 || !reflect.DeepEqual(payload.Values, []string{"1.1.1.1"}) {
						t.Errorf("unexpected update: %+v", payload)
					}
					if name == tc.failRecord {
						w.WriteHeader(http.StatusServiceUnavailable)
						return
					}
					w.WriteHeader(http.StatusNoContent)
				default:
					t.Errorf("unexpected method: %s", r.Method)
				}
			})
			err := updateRecords(context.Background(), client, testConfig(), netip.MustParseAddr("1.1.1.1"), tc.dryRun, tc.force, newLogger(&logs, true))
			if (err != nil) != tc.wantError {
				t.Fatalf("got error %v; wantError=%v", err, tc.wantError)
			}
			if !reflect.DeepEqual(writes, tc.wantWrites) {
				t.Errorf("writes: got %v; want %v", writes, tc.wantWrites)
			}
			if !strings.Contains(logs.String(), tc.wantLog) {
				t.Errorf("missing %q in logs: %s", tc.wantLog, &logs)
			}
		})
	}
}

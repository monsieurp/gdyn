package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type recordSet struct {
	Name   string   `json:"rrset_name,omitempty"`
	Type   string   `json:"rrset_type,omitempty"`
	Values []string `json:"rrset_values"`
	TTL    int      `json:"rrset_ttl"`
}

type gandiClient struct {
	http    *retryClient
	baseURL string
	token   string
}

func (client *gandiClient) records(ctx context.Context, domain string) ([]recordSet, error) {
	var records []recordSet
	err := client.request(ctx, http.MethodGet, "domains/"+url.PathEscape(domain)+"/records", nil, &records)
	return records, err
}

func (client *gandiClient) update(ctx context.Context, domain, name, ip string, ttl int) error {
	path := "domains/" + url.PathEscape(domain) + "/records/" + url.PathEscape(name) + "/A"
	return client.request(ctx, http.MethodPut, path, recordSet{Values: []string{ip}, TTL: ttl}, nil)
}

func (client *gandiClient) request(ctx context.Context, method, path string, payload any, result any) error {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("encode Gandi request: %w", err)
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(client.baseURL, "/")+"/"+path, body)
	if err != nil {
		return fmt.Errorf("create Gandi request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+client.token)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	response, err := client.http.do(req, 8<<20)
	if err != nil {
		return fmt.Errorf("Gandi request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var detail struct {
			Message string `json:"message"`
		}
		_ = json.NewDecoder(io.LimitReader(response.Body, 8192)).Decode(&detail)
		message := strings.TrimSpace(strings.ReplaceAll(detail.Message, client.token, "[redacted]"))
		if message == "" {
			message = http.StatusText(response.StatusCode)
		}
		return fmt.Errorf("Gandi HTTP %d: %s", response.StatusCode, message)
	}
	if result != nil {
		if err := json.NewDecoder(io.LimitReader(response.Body, 8<<20)).Decode(result); err != nil {
			return fmt.Errorf("decode Gandi response: %w", err)
		}
	}
	return nil
}

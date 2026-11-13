package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"
)

func newHTTPClient(ipv4 bool) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if ipv4 {
		dialer := &net.Dialer{Timeout: 10 * time.Second}
		transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
			return dialer.DialContext(ctx, "tcp4", address)
		}
	}
	return &http.Client{
		Transport: transport,
		Timeout:   20 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return fmt.Errorf("HTTP redirects are not supported; configure the final URL")
		},
	}
}

func resolveIP(ctx context.Context, client *http.Client, resolverURL string) (netip.Addr, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, resolverURL, nil)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("create IP resolver request: %w", err)
	}
	req.Header.Set("Accept", "text/plain")
	response, err := client.Do(req)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("discover public IP: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return netip.Addr{}, fmt.Errorf("IP resolver returned HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 4097))
	if err != nil {
		return netip.Addr{}, fmt.Errorf("read IP resolver response: %w", err)
	}
	if len(body) > 4096 {
		return netip.Addr{}, fmt.Errorf("IP resolver response exceeds 4096 bytes")
	}
	ip, err := netip.ParseAddr(strings.TrimSpace(string(body)))
	if err != nil || !ip.Is4() || !ip.IsGlobalUnicast() || ip.IsPrivate() {
		return netip.Addr{}, fmt.Errorf("IP resolver must return a single public IPv4 address as plain text")
	}
	return ip, nil
}

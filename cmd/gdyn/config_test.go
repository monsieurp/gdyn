package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func testConfig() config {
	cfg := defaultConfig()
	cfg.Gandi.Token = "test-token"
	cfg.DNS.Domain = "example.com"
	cfg.DNS.Records = []string{"@", "www"}
	return cfg
}

func TestLoadConfigDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[gandi]\ntoken = 'test-token'\n[dns]\ndomain = 'example.com'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Gandi.URL != "https://api.gandi.net/v5/livedns/" || cfg.DNS.TTL != 3600 || cfg.IP.ResolverURL != "https://me.gandi.net" || !reflect.DeepEqual(cfg.DNS.Records, []string{"@"}) {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	if cfg.IP.FallbackResolverURL != "https://api.ipify.org" {
		t.Fatal("unexpected default fallback resolver")
	}
}

func TestFallbackCanBeDisabled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	cfg := testConfig()
	cfg.IP.FallbackResolverURL = ""
	if err := saveConfig(path, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.IP.FallbackResolverURL != "" {
		t.Fatal("load restored the default after fallback was explicitly disabled")
	}
}

func TestLoadConfigErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if _, err := loadConfig(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing file: got %v", err)
	}
	for _, tc := range []struct {
		name, content, want string
	}{
		{"unknown key", "[dns]\nttll = 3600\n", "dns.ttll"},
		{"syntax", "[gandi]\ntoken = 'secret-token' broken", "invalid TOML"},
		{"wrong type", "[dns]\nttl = 'secret-token'", "invalid TOML"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(tc.content), 0600); err != nil {
				t.Fatal(err)
			}
			_, err := loadConfig(path)
			if err == nil || !strings.Contains(err.Error(), tc.want) || strings.Contains(err.Error(), "secret-token") {
				t.Fatalf("got %v; want %q without the token", err, tc.want)
			}
		})
	}
}

func TestConfigValidation(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*config)
		want string
	}{
		{"empty token", func(c *config) { c.Gandi.Token = "" }, "gandi.token"},
		{"token injection", func(c *config) { c.Gandi.Token = "token\r\nHeader: value" }, "gandi.token"},
		{"HTTP API", func(c *config) { c.Gandi.URL = "http://api.example.com" }, "gandi.url"},
		{"API credentials", func(c *config) { c.Gandi.URL = "https://user:pass@api.example.com" }, "gandi.url"},
		{"API query", func(c *config) { c.Gandi.URL = "https://api.example.com/?key=value" }, "gandi.url"},
		{"API fragment", func(c *config) { c.Gandi.URL = "https://api.example.com/#fragment" }, "gandi.url"},
		{"missing domain", func(c *config) { c.DNS.Domain = "" }, "dns.domain"},
		{"domain label", func(c *config) { c.DNS.Domain = "-invalid.com" }, "dns.domain"},
		{"no records", func(c *config) { c.DNS.Records = nil }, "dns.records"},
		{"empty record", func(c *config) { c.DNS.Records = []string{"@", ""} }, "dns.records"},
		{"record path", func(c *config) { c.DNS.Records = []string{"../www"} }, "dns.records"},
		{"duplicate record", func(c *config) { c.DNS.Records = []string{"www", " WWW "} }, "duplicate"},
		{"low TTL", func(c *config) { c.DNS.TTL = 299 }, "dns.ttl"},
		{"high TTL", func(c *config) { c.DNS.TTL = 2592001 }, "dns.ttl"},
		{"HTTP resolver", func(c *config) { c.IP.ResolverURL = "http://resolver.example.com" }, "ip.resolver_url"},
		{"resolver credentials", func(c *config) { c.IP.ResolverURL = "https://user:pass@resolver.example.com" }, "ip.resolver_url"},
		{"HTTP fallback", func(c *config) { c.IP.FallbackResolverURL = "http://fallback.example.com" }, "ip.fallback_resolver_url"},
		{"fallback credentials", func(c *config) { c.IP.FallbackResolverURL = "https://user:pass@fallback.example.com" }, "ip.fallback_resolver_url"},
		{"fallback fragment", func(c *config) { c.IP.FallbackResolverURL = "https://fallback.example.com/#fragment" }, "ip.fallback_resolver_url"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := testConfig()
			tc.edit(&cfg)
			if err := cfg.validate(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v; want error containing %q", err, tc.want)
			}
		})
	}
	for _, ttl := range []int{300, 3600, 2592000} {
		cfg := testConfig()
		cfg.Gandi.URL = " https://api.example.com/livedns/ "
		cfg.DNS.Domain = " Example.COM. "
		cfg.DNS.Records = []string{" @ ", " WWW ", "*.home"}
		cfg.DNS.TTL = ttl
		cfg.IP.FallbackResolverURL = " https://fallback.example.com/ip "
		if err := cfg.validate(); err != nil {
			t.Fatal(err)
		}
		if cfg.DNS.Domain != "example.com" || cfg.Gandi.URL != "https://api.example.com/livedns/" || !reflect.DeepEqual(cfg.DNS.Records, []string{"@", "www", "*.home"}) {
			t.Fatalf("normalization failed: %+v", cfg)
		}
		if cfg.IP.FallbackResolverURL != "https://fallback.example.com/ip" {
			t.Fatal("fallback URL was not trimmed")
		}
	}
}

func TestSaveConfigRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("old content"), 0644); err != nil {
		t.Fatal(err)
	}
	want := testConfig()
	want.Gandi.URL = "https://custom.example.com/api/"
	want.IP.ResolverURL = "https://resolver.example.com/ip"
	want.IP.FallbackResolverURL = "https://fallback.example.com/ip"
	if err := saveConfig(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip: got %+v; want %+v", got, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("permissions: got %o; want 600", info.Mode().Perm())
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want.Gandi.Token = ""
	if err := saveConfig(path, want); err == nil {
		t.Fatal("invalid configuration was saved")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("failed save changed the existing configuration")
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatal("save left temporary files behind")
	}
}

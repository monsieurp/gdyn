package main

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/BurntSushi/toml"
)

type config struct {
	Gandi struct {
		URL   string `toml:"url"`
		Token string `toml:"token"`
	} `toml:"gandi"`
	DNS struct {
		Domain  string   `toml:"domain"`
		Records []string `toml:"records"`
		TTL     int      `toml:"ttl"`
	} `toml:"dns"`
	IP struct {
		ResolverURL string `toml:"resolver_url"`
	} `toml:"ip"`
}

func defaultConfig() config {
	var cfg config
	cfg.Gandi.URL = "https://api.gandi.net/v5/livedns/"
	cfg.DNS.Records = []string{"@"}
	cfg.DNS.TTL = 3600
	cfg.IP.ResolverURL = "https://me.gandi.net"
	return cfg
}

func loadConfig(path string) (config, error) {
	cfg := defaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("read configuration: %w", err)
	}
	metadata, err := toml.Decode(string(data), &cfg)
	if err != nil {
		// Do not echo configuration contents, which may contain the token.
		return cfg, fmt.Errorf("invalid TOML syntax or value types in %s", path)
	}
	if keys := metadata.Undecoded(); len(keys) != 0 {
		return cfg, fmt.Errorf("unknown configuration key: %s", keys[0])
	}
	return cfg, nil
}

func (cfg *config) validate() error {
	cfg.Gandi.URL = strings.TrimSpace(cfg.Gandi.URL)
	apiURL, err := url.Parse(cfg.Gandi.URL)
	if err != nil || apiURL.Scheme != "https" || apiURL.Hostname() == "" || apiURL.User != nil || apiURL.Fragment != "" || apiURL.RawQuery != "" || apiURL.ForceQuery || apiURL.Opaque != "" {
		return fmt.Errorf("gandi.url must be an HTTPS URL without credentials, a query or a fragment")
	}
	cfg.Gandi.Token = strings.TrimSpace(cfg.Gandi.Token)
	if cfg.Gandi.Token == "" || strings.ContainsFunc(cfg.Gandi.Token, unicode.IsSpace) || strings.ContainsFunc(cfg.Gandi.Token, unicode.IsControl) {
		return fmt.Errorf("gandi.token must be nonempty and contain no whitespace or control characters")
	}
	cfg.DNS.Domain = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(cfg.DNS.Domain), "."))
	if !validDNSName(cfg.DNS.Domain, false) || !strings.Contains(cfg.DNS.Domain, ".") {
		return fmt.Errorf("dns.domain must be a domain name such as example.com (use punycode for international names)")
	}
	if len(cfg.DNS.Records) == 0 {
		return fmt.Errorf("dns.records must contain at least one record")
	}
	seen := make(map[string]bool)
	for i, record := range cfg.DNS.Records {
		record = strings.ToLower(strings.TrimSpace(record))
		if record != "@" && (!validDNSName(record, true) || len(record)+1+len(cfg.DNS.Domain) > 253) {
			return fmt.Errorf("dns.records entry %d must be @ or a relative DNS name", i+1)
		}
		if seen[record] {
			return fmt.Errorf("dns.records entry %d is a duplicate", i+1)
		}
		seen[record] = true
		cfg.DNS.Records[i] = record
	}
	if cfg.DNS.TTL < 300 || cfg.DNS.TTL > 2592000 {
		return fmt.Errorf("dns.ttl must be between 300 and 2592000 seconds")
	}
	cfg.IP.ResolverURL = strings.TrimSpace(cfg.IP.ResolverURL)
	u, err := url.Parse(cfg.IP.ResolverURL)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" || u.Opaque != "" {
		return fmt.Errorf("ip.resolver_url must be an HTTPS URL without credentials or a fragment")
	}
	return nil
}

func validDNSName(name string, wildcard bool) bool {
	if name == "" || len(name) > 253 {
		return false
	}
	for i, label := range strings.Split(name, ".") {
		if wildcard && i == 0 && label == "*" {
			continue
		}
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				return false
			}
		}
	}
	return true
}

func saveConfig(path string, cfg config) error {
	if err := cfg.validate(); err != nil {
		return err
	}
	var data bytes.Buffer
	if err := toml.NewEncoder(&data).Encode(cfg); err != nil {
		return fmt.Errorf("encode configuration: %w", err)
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".gdyn-config-*")
	if err != nil {
		return fmt.Errorf("create configuration: %w", err)
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if _, err := file.Write(data.Bytes()); err != nil {
		return fmt.Errorf("write configuration: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync configuration: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close configuration: %w", err)
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return fmt.Errorf("replace configuration: %w", err)
	}
	return nil
}

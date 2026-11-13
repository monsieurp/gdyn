package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"strings"
)

func updateRecords(ctx context.Context, client *gandiClient, cfg config, ip netip.Addr, dryRun, force bool, logger *slog.Logger) error {
	logger.Debug("reading DNS records", "domain", cfg.DNS.Domain)
	records, err := client.records(ctx, cfg.DNS.Domain)
	if err != nil {
		return err
	}
	current := make(map[string]recordSet)
	cname := make(map[string]bool)
	for _, record := range records {
		name := strings.ToLower(record.Name)
		if record.Type == "A" {
			current[name] = record
		}
		if record.Type == "CNAME" {
			cname[name] = true
		}
	}
	var failures []error
	for _, name := range cfg.DNS.Records {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(failures, err)...)
		}
		log := logger.With("domain", cfg.DNS.Domain, "record", name)
		if cname[name] {
			log.Error("record failed", "error", "a CNAME exists at this name")
			failures = append(failures, fmt.Errorf("%s has a conflicting CNAME", name))
			continue
		}
		record := current[name]
		matches := len(record.Values) == 1 && record.Values[0] == ip.String() && record.TTL == cfg.DNS.TTL
		log.Debug("comparing record", "old", record.Values, "new", ip, "old_ttl", record.TTL, "new_ttl", cfg.DNS.TTL)
		if matches && !force {
			log.Info("record unchanged", "ip", ip)
			continue
		}
		if dryRun {
			log.Info("would update record", "old", record.Values, "new", ip, "ttl", cfg.DNS.TTL, "forced", force)
			continue
		}
		if err := client.update(ctx, cfg.DNS.Domain, name, ip.String(), cfg.DNS.TTL); err != nil {
			log.Error("record update failed", "error", err)
			failures = append(failures, fmt.Errorf("%s: %w", name, err))
			continue
		}
		log.Info("record updated", "old", record.Values, "new", ip, "ttl", cfg.DNS.TTL)
	}
	if len(failures) > 0 {
		return fmt.Errorf("%d record(s) failed: %w", len(failures), errors.Join(failures...))
	}
	return nil
}

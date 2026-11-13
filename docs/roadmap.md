# Future releases

These are proposals, not committed release plans.

## v1.2 candidates

- [ ] **IPv6 support.** Update `AAAA` records alongside `A` records or independently. Discover each address family separately.
- [ ] **Explicit IP override.** Accept `--ip ADDRESS` to skip discovery when a router or script already knows the address.

These two features are the preferred scope for v1.2.
Configuration changes and behavior for partial IPv4/IPv6 failures still need a design discussion.

## Other ideas

- [ ] **Connection check.** A proposed `gdyn check` command validates configuration, resolver responses and Gandi access without DNS writes.
  Decide whether a separate command adds enough value beyond `--dry-run`.
- [ ] **JSON logs.** Add an optional `--log-format json` for monitoring tools. Keep the current text format as the default.
- [ ] **Multiple domains.** Update several zones from one configuration, with separate results for each domain.
  This requires a larger configuration and UI change.
- [ ] **Terminal UI tests.** Use a simulated terminal to cover field saves, F2 theme changes and keyboard navigation.

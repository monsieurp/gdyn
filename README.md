# gdyn

A dynamic DNS client for Gandi LiveDNS, written in Go.
It discovers your public IPv4 address and updates configured `A` records, then exits.
Includes a terminal configuration form, resolver fallback, bounded retries and protection against overlapping runs.

## Requirements

- Go 1.24 or later and Make to build.
- A domain that uses Gandi LiveDNS.
- A Gandi Personal Access Token with permission to read and update DNS.

## Build

```sh
make build
```

The Makefile embeds the Git version and disables cgo by default, including on FreeBSD.
See the [build details](docs/usage.md#build) for overrides and direct Go commands.

## Quick start

```sh
./gdyn configure   # create or edit config.toml
./gdyn --dry-run   # inspect planned changes
./gdyn             # apply changes
```

The form requires an interactive terminal. F2 cycles through four themes.
All commands use `./config.toml`. Use `--config PATH` for another location.

## Configuration

```toml
[gandi]
url = "https://api.gandi.net/v5/livedns/"
token = "your-personal-access-token"

[dns]
domain = "example.com"
records = ["@"]
ttl = 3600

[ip]
resolver_url = "https://me.gandi.net"
fallback_resolver_url = "https://api.ipify.org"
```

`@` means the domain itself. Add `www` to update `www.example.com` too.
Each configured `A` record contains one IPv4 address. Updates replace its existing address list.
See [configuration details](docs/usage.md#toml-file) or [config.example.toml](config.example.toml).

## CLI flags

| Flag | Purpose |
| --- | --- |
| `--config PATH` | Configuration file (default: `./config.toml`) |
| `--dry-run` | Show planned changes without DNS writes |
| `--force` | Update all configured records even if unchanged |
| `--verbose` | Include progress and record comparisons |
| `--log PATH` | Append logs only to this file instead of the terminal |
| `--version` | Print the version |
| `--help` | Print help |

`gdyn configure` accepts `--config` and `--help`.
Logs always include timestamps.

## Scheduling

Add a cron entry to run every five minutes:

```cron
*/5 * * * * /path/to/gdyn --config /path/to/config.toml --log /path/to/gdyn.log
```

Use absolute paths. The configuration directory must permit writes for the lock file.
An overlapping run logs `run already active` and exits.

## Tests

```sh
make test
```

See the [usage guide](docs/usage.md) for UI controls, validation, retries, fallback, logs, locks and exit codes.

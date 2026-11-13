# gdyn

`gdyn` keeps Gandi DNS records pointed at your public IPv4 address.
Each run discovers the address, compares each configured `A` record and updates records that differ.
The program then exits. Cron or a systemd timer can run it on a schedule.

## Requirements

- Go 1.24 or later to build the program.
- A domain that uses Gandi LiveDNS.
- A Gandi Personal Access Token with permission to read and update the domain's DNS configuration.
- HTTPS access to the Gandi API and an IP resolver.
- An interactive terminal for `gdyn configure`.

The DNS update command does not need an interactive terminal.

## Build

From the repository root, run:

```sh
go build -o gdyn ./cmd/gdyn
```

The binary includes its Go dependencies. It does not need a separate Go installation to run.

### Release version

A normal build reports `gdyn dev`.
For a release build, embed the release tag:

```sh
go build -ldflags="-X main.version=v1" -o gdyn ./cmd/gdyn
./gdyn --version
# gdyn v1
```

Replace `v1` with the tag for that release.
On FreeBSD, prefix the build command with `env CGO_ENABLED=0` to avoid the C linker dependency.

## Configure

Open the configuration form:

```sh
./gdyn configure
```

This command reads `config.toml` from the current directory.
If the file does not exist, the form uses defaults.
Save validates the fields and writes the file. Cancel exits without a write.

For a different path, use:

```sh
./gdyn configure --config /path/to/config.toml
```

The parent directory must already exist.
The form masks the token. The saved TOML file contains the token as plain text.
Save replaces the file atomically with owner-only read and write permissions (`0600`).
Save rewrites the TOML file without existing comments or custom formatting.

The centered form contains these fields, in order:

1. Gandi token
2. Domain
3. Records
4. TTL (seconds)
5. API URL
6. IP resolver

| Key | Action |
| --- | --- |
| Tab | Move to the next field or button |
| Shift+Tab | Move to the previous field or button |
| Enter | Advance from a field or activate a button |
| F2 | Cycle through slate, nord, dracula and paper |
| Esc | Cancel the form or close an error dialog |

The form starts with the slate theme. Theme changes apply only to the current session.

### TOML file

You can also edit the configuration file directly.
The [example file](config.example.toml) contains all supported keys:

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
```

| Key | Meaning | Default |
| --- | --- | --- |
| `gandi.url` | Gandi LiveDNS API base URL | `https://api.gandi.net/v5/livedns/` |
| `gandi.token` | Personal Access Token | Required |
| `dns.domain` | Exact DNS zone to update | Required |
| `dns.records` | Record names relative to the zone | `["@"]` |
| `dns.ttl` | DNS cache lifetime in seconds | `3600` |
| `ip.resolver_url` | Service that returns the public IPv4 address | `https://me.gandi.net` |

`@` means the domain itself. For `example.com`, the name `www` means `www.example.com`.
To update both names, use `records = ["@", "www"]`.
In the form, enter the same names as `@, www`.

The DNS record type is always `A`. It does not belong in the records list.
An entry named `A` instead means `a.example.com`.
The program converts domain names and record names to lowercase.
International domain names must use punycode.

The program rejects unknown TOML keys, empty record lists and duplicate record names.
TTL must be between `300` and `2592000` seconds.

Both URLs must use HTTPS without embedded credentials or a fragment.
The API URL must also exclude a query string.
Requests do not follow redirects. Each request has a 20-second timeout.

The resolver must return one public IPv4 address as plain text.
Surrounding whitespace is acceptable. HTML, JSON and IPv6 responses are not acceptable.

## Run

First, inspect the planned changes:

```sh
./gdyn --dry-run
```

Then apply the changes:

```sh
./gdyn
```

Both commands use `./config.toml` by default.
For a different path, use `--config`:

```sh
./gdyn --config /path/to/config.toml --dry-run
```

### Command-line flags

```text
gdyn [flags]
gdyn configure [flags]
```

| Flag | Behavior |
| --- | --- |
| `--config PATH` | Read the specified file. Default: `./config.toml`. |
| `--dry-run` | Show planned changes without DNS or configuration writes. |
| `--force` | Update every configured record even if its address and TTL match. |
| `--verbose` | Include progress and record comparisons in the logs. |
| `--log PATH` | Append logs only to this file. No default file. |
| `--version` | Print the embedded release version and exit. Default: `gdyn dev`. |
| `--help` | Print help and exit. |

`gdyn configure` accepts only `--config` and `--help`.
Flags go after the command. Positional arguments are not supported.
With both `--force` and `--dry-run`, the program shows forced updates without DNS writes.

### Update behavior

The program reads the zone's records once and compares each configured name independently.
A matching record contains exactly one address equal to the discovered IP and has the configured TTL.
An address difference, a TTL difference or multiple addresses trigger an update.
The program also creates missing `A` records.

Each configured `A` record belongs to `gdyn`. An update replaces its entire address list with one IPv4 address.
The program leaves other record names and record types unchanged.
A CNAME at a configured name prevents an `A` update at that name.

If an individual update fails, the program reports the error and continues with the remaining names.
Successful updates remain in place. The program does not retry requests or restore earlier records.

| Exit status | Meaning |
| --- | --- |
| `0` | Success, successful dry-run, help or version output |
| `1` | Configuration, resolver, API or record update error |
| `2` | Invalid command-line flags or arguments |

The program supports one domain and IPv4 `A` records per configuration file.
It does not manage IPv6, PTR records or zone snapshots.

## Logs

Logs use timestamped plain text with one event per line:

```text
time=2026-11-13T14:05:00+01:00 level=INFO msg="record unchanged" domain=example.com record=@ ip=1.1.1.1
```

By default, logs go to the terminal through standard error.
With `--log`, logs append only to the specified file:

```sh
./gdyn --log /path/to/gdyn.log
```

The log directory must already exist. New log files use permissions `0600`.
The program rejects a log path that points to the configuration file.
If the program cannot open the log file, it reports that error in the terminal and exits.

`--verbose` includes DEBUG events. Normal runs report unchanged records, updated records and errors.
The program does not rotate log files.

## Schedule with cron

Open your crontab:

```sh
crontab -e
```

Add a run every five minutes:

```cron
*/5 * * * * /path/to/gdyn --config /path/to/config.toml --log /path/to/gdyn.log
```

Use absolute paths because cron can use a different working directory.
Choose an interval longer than the expected run duration to avoid overlapping runs.
The program does not prevent concurrent runs.

## Tests and source

Run the tests:

```sh
go test ./...
```

The tests cover configuration, IP responses, API requests, update decisions, CLI flags and log output.
HTTP tests use simulated responses. They do not contact Gandi or change DNS.
The tests do not exercise the interactive form.

All Go files belong to one package under `cmd/gdyn`:

| File | Responsibility |
| --- | --- |
| `main.go` | CLI flags, log destination and exit status |
| `config.go` | TOML defaults, validation and file writes |
| `resolver.go` | Public IPv4 discovery and HTTP timeouts |
| `gandi.go` | Gandi API requests and responses |
| `update.go` | Record comparisons and updates |
| `ui.go` | Configuration form and themes |

Dependencies are BurntSushi/toml, tview and tcell. Other dependencies are indirect.

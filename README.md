# Codex Quota Statistics

[English](README.md) | [简体中文](README.zh-CN.md)

CPA native plugin for recorded Codex usage, not quota predictions.
Release: **0.22.0**. This fork uses its own CPA plugin ID and public plugin
source; it is not automatically included in CPA's default marketplace registry.

## Accounting

- All-account totals and independently expandable account sections.
- Account statistics deletion with confirmation, leaving CPA credentials intact.
- Cumulative requests, failures, Tokens and USD/Credits reference values.
- Subscription intervals between manually entered actual payment dates.
- Separate monthly subscription, weekly and five-hour ledgers, newest first,
  with active-period badges.
- Actual model/tier usage over 1, 7 or 30 days.
- Recorded primary/secondary quota percentages; no forecasts, capacity
  estimation, learned weights or pace projections.

K/M/B means 1,000 / 1,000,000 / 1,000,000,000. Units remain explicit;
rounding changes display only. USD/Credits are token-rate reference conversions,
**not actual bills or subscription quota limits**. Reasoning/cache counts
overlap output/input counts; do not add them again. Do not sum percentages
or usage across overlapping quota pools. Monthly intervals follow payment
dates, not calendar months. Missing pre-installation or failed-to-store history
cannot be reconstructed; first quota observations can be partial baselines.

## CPA Integration

Plugin ID is `cpa-codex-quota-stats`, distinct from the original estimator.
Do not load this fork and an old statistics/estimator binary together against
the same SQLite file. Existing database schemas are retained.
The integration baseline is CPA **8.0.16**, Linux amd64, native ABI v1.
Other CPA versions and platforms require separate verification.

Merge [docs/config.example.yaml](docs/config.example.yaml) into your CPA
configuration. Preserve the existing `data_path` on upgrades. The example is
not a complete CPA config and deliberately contains no host or credential.

Dashboard: `/v0/resource/plugins/cpa-codex-quota-stats/dashboard`.
It is a public static resource; account data is fetched only through CPA's
authenticated management API. Use your existing HTTPS management entry.
The same-origin management-panel authorization bridge is supported.

Routes under `/v0/management/cpa-codex-quota-stats`:

| Method | Path | Result |
| --- | --- | --- |
| GET | `/overview` | Accounts, version, dropped-event count |
| GET | `/accounting?account=<AuthID>&limit=50&offset=0` | Cumulative, subscription and reset ledgers |
| GET | `/usage?account=<AuthID>&days=7` | Actual model/tier usage |
| POST | `/subscriptions` | Add/edit `{id,account,paid_at,amount_usd}` |
| DELETE | `/subscriptions` | Delete `{account,id}` |
| DELETE | `/accounts` | Delete active statistics for `{account,confirm:true}` |

Payment timestamps are Unix seconds. Dashboard dates use Asia/Shanghai.
Missing rate conversions are marked rather than inferred from quota usage.

## Marketplace Installation And Release

In CPA's plugin market, add this public plugin source and refresh:

```text
https://raw.githubusercontent.com/jiao-ye/cpa-codex-quota-stats/main/registry.json
```

Select **Codex Quota Statistics** from this source and install.
`plugin.json` supplies the manifest; `registry.json` marks the source entry
`auth_required: false`. No GitHub credential is needed for public downloads,
subject to normal GitHub API rate limits.

When migrating from `cpa-quota-estimator`, back up the database, unload/remove
the old library, and configure the new ID's `data_path` to the **existing**
SQLite file. The included example keeps the legacy file basename for this
purpose. An empty path elsewhere creates an independent ledger, not a data
migration. CPA may require a restart to load/unload native libraries.

Build on Linux amd64 with Go 1.22.12 and a C compiler:

```sh
go test ./...
go vet ./...
make build VERSION=0.22.0
make package VERSION=0.22.0
```

CPA release assets:

- `cpa-codex-quota-stats_0.22.0_linux_amd64.zip`
- `checksums.txt` containing the ZIP's SHA-256 hash
- `plugin.json` describing version `0.22.0`, release tag `v0.22.0`

The ZIP contains `cpa-codex-quota-stats.so` at its root. Verify its checksum before
installation. Use SQLite's online backup API while WAL is active; copying only
the main database file can lose recent transactions. Replace the plugin through
your normal CPA maintenance procedure, preserving data and a rollback copy.
This repository contains no remote-host deployment automation or credentials.

The **Release** GitHub workflow is manual-only. It runs tests, race tests and
vet, packages Linux amd64 assets, then creates a **draft** release.
It does not deploy or submit to the default marketplace. CI performs only
source-disclosure and syntax checks. Review the successful workflow and release
assets before publishing the draft.
CPA's release discovery requires a published release; a draft is not installable
through normal plugin-store discovery until the owner explicitly publishes it.

Account deletion atomically removes this plugin's raw usage, archived usage,
quota samples/cycles, payment records and lifetime totals for the selected
account. It is irreversible in the current database. Backups and inert legacy
learning tables are not scrubbed. Later traffic starts a new ledger. Deletion
does not revoke CPA credentials or stop that account from serving requests.

## Privacy And Attribution

The database includes account identifiers, usage and payment records. Treat it,
its WAL/SHM files, exports and backups as confidential. Header capture defaults
off and, when enabled, stores only the quota header allowlist. The default
price-catalog fetch sends no account/usage payload, but reveals normal network
metadata to `models.dev`. See [SECURITY.md](SECURITY.md) and
[docs/CODE_REVIEW.md](docs/CODE_REVIEW.md) for scope and remaining risks.

Based on Autsunset's MIT-licensed `cpa-quota-estimator`, reviewed starting
commit `ab33be577bec7f8b652b9228cfd7c953d4859826`. Original copyright is
preserved in [LICENSE](LICENSE); embedded Lucide icons are ISC-licensed.
See [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).

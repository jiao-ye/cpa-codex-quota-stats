# Security And Privacy

The owner authorized public source publication on 2026-10-07.
Public source and release assets must not include runtime databases,
operational reports, credentials or account screenshots.

## Sensitive Data

The SQLite database stores account identifiers, model usage, payment dates and
amounts, quota observations and optional allowlisted quota headers. Database,
WAL/SHM, backups, exported API responses and production screenshots must not be
committed. No prompts, response bodies or authorization headers are intentionally
stored by this plugin. A misconfigured upstream can still put sensitive strings
in account/model/header fields; treat all runtime data as confidential.

New database directories use mode 0700; database files use 0600 on POSIX.
Existing database and existing WAL/SHM file permissions are tightened on open.
Existing parent directories are not chmod'ed to avoid changing shared paths.
Run CPA as a dedicated user, protect parents and backups, and avoid untrusted
symlinks. Windows chmod is not an access-control guarantee; use proper ACLs.
Previously captured headers and old backups are not automatically scrubbed.

## Network And Authentication

The static dashboard contains no live account data or credentials. Management
API protection depends on CPA; do not bypass CPA authentication with custom
proxy routes. Use HTTPS. Loopback HTTP is allowed for local development.
The dashboard checks same-origin authorization-bridge messages. Its own fallback
management key is stored in session storage; it can reuse the management panel's
same-origin stored authorization. A same-origin XSS or browser extension can
read browser storage, so isolate the management origin and restrict access.

The default outbound request is a GET to `https://models.dev/catalog.json`
for price data, with a plugin/version User-Agent and no account or usage body.
Network address, request timing and plugin version remain observable there.
Administrators control any configured price-source URL and egress policy.
The project does not add analytics, telemetry or external icon/CDN requests.

Account-statistics deletion requires authenticated CPA management access and
explicit confirmation. It does not delete CPA credentials. It removes active
accounting tables, not old backups or inert historical learning tables; it is
not a comprehensive data-erasure mechanism.

## Publication Guard

Before committing:

```sh
git add .
node tools/audit-staged.cjs
```

The guard inspects the staged index and rejects likely credentials, personal
paths, non-loopback IPv4 addresses, runtime databases and binary artifacts.
It reports only rule names and locations, never matched values. This is a
heuristic, not a proof of absence: manually review the staged diff too.
CI repeats this check. It cannot remove information already pushed into history.
Rotate any exposed credential immediately, restrict access and address history
separately; deleting the latest file alone is insufficient.

Report vulnerabilities privately to the repository owner. Do not include live
credentials, account data or production addresses in issues or pull requests.

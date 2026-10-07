# Publication Review

Date: 2026-10-07. Scope: the 0.21.4 source publication candidate, CPA native ABI
registration, management/dashboard data paths, persistence permissions,
release packaging and staged-file disclosure checks. Operational reports,
server configuration, live databases and credentials are excluded.

## Findings Addressed

| Severity | Location | Finding and change |
| --- | --- | --- |
| P2 | `store.go` | Default database creation could allow other host users to read account/payment data. New directories are 0700, database creation/open tightens to 0600, and existing WAL/SHM permissions are restricted. |
| P2 | `app.go` | Opt-in capture accepted broad X-Codex fields, including unknown sensitive identifiers. It now accepts only seven quota/plan fields; default remains off. |
| P2 | `web/accounting.js` | Primary-only weekly windows could appear in the five-hour ledger. Weekly classification now recognizes weekly window lengths as well as explicit scope. |
| P2 | `package-release.sh` | Packaging rejected safe private version suffixes and reused working-tree artifacts. It now builds in a temporary directory, produces a clean ZIP and leaves existing build outputs alone. |
| P3 | Repository metadata/docs | Upstream-only identity and host-specific deployment examples were unsuitable for this fork. Metadata, generic configuration, private-source manifests and owner attribution were aligned; external logo removed. |

Added a focused POSIX permissions test and extended the header-capture test.
These tests have not been executed locally for this publication task.

## Disclosure Review

- No hard-coded production credential, live database or live account data was
  identified in the candidate source during review. This is a scoped finding,
  not an absolute claim that secrets cannot be present.
- Private operational reports and release binaries are outside the publication
  directory. Source-only initialization avoids importing previous history.
- Outbound price sync sends a catalog GET without account/usage payload. It
  still discloses connection metadata and plugin version; see `SECURITY.md`.
- Public dashboard HTML embeds code only. Account APIs are registered through
  CPA management routes; authentication is delegated to the host.
- Account/model/error text uses DOM text assignment; HTML insertion is used for
  fixed icon/templates. Bridge messages check origin, parent source and request
  identity. Browser-stored authorization remains sensitive to same-origin XSS.
- Original MIT and embedded Lucide ISC notices are preserved.

## Remaining Limits

- Existing header data and backups are not purged. Existing shared parent
  directories and Windows ACLs require administrator protection.
- Large management ledger reads take the app mutex and can delay ingestion.
  This publication does not refactor concurrency or claim a throughput guarantee.
- Account visibility preferences store account identifiers locally in browser
  storage; use a trusted browser profile.
- An initial quota reading or missing upstream headers cannot recover earlier
  usage, and missing/failed events remain visible as accounting limitations.
- No browser verification, functional tests, race tests or binary build was run
  locally for this publication task. A manual Linux release workflow is provided
  to run tests and build the candidate before distribution.
- Source review is not a dependency vulnerability audit or penetration test.
- Candidate fixes are not deployed by this task. Production remains unchanged.

## 0.22.0 Release Follow-Up

The owner authorized public publication on 2026-10-07. Version 0.22.0 adds
confirmed account-statistics deletion and uses independent public plugin ID
`cpa-codex-quota-stats`. Migration retains the old SQLite format and requires
unloading the legacy statistics library before using the same database.

The deletion API requires a nonempty exact account ID and `confirm:true`;
CPA management authentication still applies. Six active accounting tables
are cleared in one transaction. Parameterized queries preserve other accounts,
and a failed deletion rolls back all tables. Existing backups and inactive
learning tables are not erased. New traffic can recreate the account.

GitHub Actions run 37606207893 passed ordinary tests, race tests, vet and
Linux amd64 CGO shared-library packaging for source commit
`6056c20a17e67496abd3ab16af081f400467b249`. Added tests cover explicit
confirmation, cross-account isolation, quoted account IDs, migration after
deletion, rollback on failure and fresh usage after deletion.
No browser verification was performed.

Both source commits were checked by the disclosure guard before changing
repository visibility. Public publication does not automatically add the
plugin to CPA's default registry; the repository supplies its own public source.

# Changelog

## 0.22.0

- Add confirmed account-statistics deletion through the dashboard and protected
  management API. Clear raw/archive usage, quota evidence, payments and totals
  in one transaction; keep other accounts and CPA credentials unchanged.
- Change account names, avatars and open-state highlights to a restrained blue.
- Use independent plugin ID `cpa-codex-quota-stats` for public CPA source installs.
- Add public registry source and manual Linux amd64 release packaging.
- Include earlier publication hardening: restricted SQLite permissions,
  allowlisted optional quota-header capture and weekly-window classification.

Migration: unload the old `cpa-quota-estimator` statistics library before
installing this ID. Point the new configuration at the original database to
retain existing accounting records. Never load both against the same file.

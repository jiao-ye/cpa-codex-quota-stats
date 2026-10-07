package main

import (
	"context"
	"fmt"
	"strings"
)

func (s *store) deleteAccount(ctx context.Context, account string) error {
	if strings.TrimSpace(account) == "" {
		return fmt.Errorf("account is required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Remove both raw and archived evidence so migration cannot restore the totals.
	for _, query := range []string{
		"DELETE FROM quota_samples WHERE account=?",
		"DELETE FROM usage_events WHERE account=?",
		"DELETE FROM usage_ledger WHERE account=?",
		"DELETE FROM quota_cycles WHERE account=?",
		"DELETE FROM subscription_payments WHERE account=?",
		"DELETE FROM lifetime_ledger WHERE account=?",
	} {
		if _, err = tx.ExecContext(ctx, query, account); err != nil {
			return err
		}
	}
	return tx.Commit()
}

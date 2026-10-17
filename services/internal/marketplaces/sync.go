package marketplaces

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/store"
)

// SyncResult reports what Sync changed.
type SyncResult struct {
	Inserted  []string
	Upgraded  []string
	Unchanged []string
	// Skipped lists keys used by an administrator's own adapter; those rows are never overwritten.
	Skipped []string
}

// Sync upserts shipped definitions into source_adapters: missing ones are inserted (enabled, with
// the shipped priority); when a shipped version is newer than the stored one, the name, domains,
// rules, region and verification are replaced while the administrator's enabled flag, priority and
// description are kept. It runs in one transaction after migrations at API start-up.
func Sync(ctx context.Context, d *store.DB, defs []Definition, log *slog.Logger) (SyncResult, error) {
	var res SyncResult
	err := d.Tx(ctx, func(q *store.Queries, _ pgx.Tx) error {
		res = SyncResult{}
		for _, def := range defs {
			verification, err := json.Marshal(def.Verification)
			if err != nil {
				return err
			}
			cur, err := q.GetSourceAdapterByKey(ctx, def.Key)
			switch {
			case errors.Is(err, pgx.ErrNoRows):
				if _, err := q.InsertShippedRulesAdapter(ctx, store.InsertShippedRulesAdapterParams{
					Key: def.Key, Name: def.Name, DomainPatterns: def.Domains, Priority: def.Priority, Rules: def.Rules,
					Description: def.Description, DefinitionVersion: def.Version, Region: def.Region, Verification: verification,
				}); err != nil {
					return fmt.Errorf("insert marketplace %s: %w", def.Key, err)
				}
				res.Inserted = append(res.Inserted, def.Key)
			case err != nil:
				return err
			case !cur.Shipped || cur.Kind != "rules":
				res.Skipped = append(res.Skipped, def.Key)
			case cur.DefinitionVersion < def.Version:
				if _, err := q.UpgradeShippedRulesAdapter(ctx, store.UpgradeShippedRulesAdapterParams{
					Key: def.Key, Name: def.Name, DomainPatterns: def.Domains, Rules: def.Rules, DefinitionVersion: def.Version,
					Region: def.Region, Verification: verification,
				}); err != nil {
					return fmt.Errorf("upgrade marketplace %s: %w", def.Key, err)
				}
				res.Upgraded = append(res.Upgraded, def.Key)
			default:
				res.Unchanged = append(res.Unchanged, def.Key)
			}
		}
		return nil
	})
	if err != nil {
		return SyncResult{}, err
	}
	if log != nil {
		for _, k := range res.Skipped {
			log.Warn("marketplace definition not applied: an adapter you created uses the same key", slog.String("key", k))
		}
		log.Info("marketplace definitions are current", slog.Int("inserted", len(res.Inserted)), slog.Int("upgraded", len(res.Upgraded)),
			slog.Int("unchanged", len(res.Unchanged)), slog.Int("skipped", len(res.Skipped)))
	}
	return res, nil
}

// SyncShipped loads the embedded definitions and syncs them.
func SyncShipped(ctx context.Context, d *store.DB, log *slog.Logger) (SyncResult, error) {
	defs, err := Load()
	if err != nil {
		return SyncResult{}, err
	}
	return Sync(ctx, d, defs, log)
}

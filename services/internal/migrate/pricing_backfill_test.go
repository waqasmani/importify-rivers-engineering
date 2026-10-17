package migrate_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/migrate"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/testutil"
)

func hasColumn(t *testing.T, ctx context.Context, conn *pgx.Conn, table, column string) bool {
	t.Helper()
	var ok bool
	if err := conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = 'public'
		AND table_name = $1 AND column_name = $2)`, table, column).Scan(&ok); err != nil {
		t.Fatal(err)
	}
	return ok
}

// TestPricingMigrationBackfill rolls the database back to before the pricing migration, stores
// products the way earlier versions did (the observed marketplace price in price), and checks that
// applying the migration keeps the selling price, copies it to the source price, leaves it
// unlocked, keeps existing costs as manual costs and does not touch updated_at.
func TestPricingMigrationBackfill(t *testing.T) {
	ctx := context.Background()
	url := testutil.NewDatabase(t)
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	for hasColumn(t, ctx, conn, "products", "source_price") {
		if _, err := migrate.Down(ctx, url); err != nil {
			t.Fatalf("roll back: %v", err)
		}
	}
	if hasColumn(t, ctx, conn, "product_variants", "price_locked") {
		t.Fatal("variant pricing columns remain after rolling back the pricing migration")
	}

	old := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	var observed, withCost, noPrice string
	ins := `INSERT INTO products (original_title, title, normalized_title, price, price_currency, cost, cost_currency, created_at, updated_at)
		VALUES ($1, $1, lower($1), $2, $3, $4, $5, $6, $6) RETURNING id`
	if err := conn.QueryRow(ctx, ins, "Observed bottle", "24.99", "EUR", nil, nil, old).Scan(&observed); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(ctx, ins, "Costed lamp", "40", "USD", "12.5", "USD", old).Scan(&withCost); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(ctx, ins, "No price", nil, nil, nil, nil, old).Scan(&noPrice); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO product_variants (product_id, title, price, currency, created_at, updated_at)
		VALUES ($1, 'Black', 26.5, 'EUR', $2, $2), ($1, 'Plain', NULL, NULL, $2, $2)`, observed, old); err != nil {
		t.Fatal(err)
	}

	if _, err := migrate.Up(ctx, url); err != nil {
		t.Fatalf("apply: %v", err)
	}

	type row struct {
		price, source       *string
		priceCur, sourceCur *string
		locked, costManual  bool
		updated             time.Time
		ruleID              *string
	}
	get := func(id string) row {
		var r row
		if err := conn.QueryRow(ctx, `SELECT price::text, source_price::text, price_currency, source_currency, price_locked, cost_manual,
			updated_at, pricing_rule_id::text FROM products WHERE id = $1`, id).Scan(&r.price, &r.source, &r.priceCur, &r.sourceCur, &r.locked,
			&r.costManual, &r.updated, &r.ruleID); err != nil {
			t.Fatal(err)
		}
		return r
	}
	o := get(observed)
	if o.price == nil || *o.price != "24.9900" || o.source == nil || *o.source != "24.9900" || *o.sourceCur != "EUR" || *o.priceCur != "EUR" ||
		o.locked || o.costManual || o.ruleID != nil || !o.updated.Equal(old) {
		t.Fatalf("observed product backfill wrong: %+v", o)
	}
	c := get(withCost)
	if *c.price != "40.0000" || *c.source != "40.0000" || c.locked || !c.costManual || !c.updated.Equal(old) {
		t.Fatalf("costed product backfill wrong: %+v", c)
	}
	n := get(noPrice)
	if n.price != nil || n.source != nil || n.locked || n.costManual {
		t.Fatalf("product without price backfill wrong: %+v", n)
	}
	var vSource, vCur *string
	var vLocked bool
	var vUpdated time.Time
	if err := conn.QueryRow(ctx, `SELECT source_price::text, source_currency, price_locked, updated_at FROM product_variants WHERE title = 'Black'`).
		Scan(&vSource, &vCur, &vLocked, &vUpdated); err != nil {
		t.Fatal(err)
	}
	if vSource == nil || *vSource != "26.5000" || *vCur != "EUR" || vLocked || !vUpdated.Equal(old) {
		t.Fatalf("variant backfill wrong: %v %v %v %v", vSource, vCur, vLocked, vUpdated)
	}
	var plainSource *string
	if err := conn.QueryRow(ctx, `SELECT source_price::text FROM product_variants WHERE title = 'Plain'`).Scan(&plainSource); err != nil || plainSource != nil {
		t.Fatalf("variant without a price got a source price: %v %v", plainSource, err)
	}
	var defaults int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM pricing_rules WHERE is_default AND enabled AND markup_percent IS NULL AND multiplier IS NULL`).Scan(&defaults); err != nil || defaults != 1 {
		t.Fatalf("default rule (selling price = cost) not seeded: %d %v", defaults, err)
	}
	// Touch triggers work again after the backfill.
	if _, err := conn.Exec(ctx, `UPDATE products SET title = 'Observed bottle 2' WHERE id = $1`, observed); err != nil {
		t.Fatal(err)
	}
	if get(observed).updated.Equal(old) {
		t.Fatal("products_touch trigger was not re-enabled")
	}
}

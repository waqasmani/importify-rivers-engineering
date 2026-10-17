package marketplaces_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/marketplaces"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/store"
	"github.com/waqasmani/importify-rivers-engineering/services/internal/testutil"
)

func TestSyncInsertsUpgradesAndKeepsAdministratorChoices(t *testing.T) {
	d, _ := testutil.OpenDB(t)
	ctx := t.Context()
	defs, err := marketplaces.Load()
	if err != nil {
		t.Fatal(err)
	}

	// The migration marks the built-in code adapters as shipped.
	for _, a := range must(d.ListSourceAdapters(ctx)) {
		if a.Kind == "builtin" && !a.Shipped {
			t.Errorf("%s: built-in adapter not marked shipped", a.Key)
		}
	}

	res, err := marketplaces.Sync(ctx, d, defs, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Inserted) != len(defs) || len(res.Upgraded)+len(res.Unchanged)+len(res.Skipped) != 0 {
		t.Fatalf("first sync: %+v", res)
	}
	amazon := must(d.GetSourceAdapterByKey(ctx, "amazon"))
	if amazon.Kind != "rules" || !amazon.Shipped || !amazon.Enabled || amazon.Priority != 60 || amazon.DefinitionVersion != 1 || amazon.Region != "global" {
		t.Fatalf("inserted row: %+v", amazon)
	}
	var verification map[string]string
	if err := json.Unmarshal(amazon.Verification, &verification); err != nil || verification["status"] != "synthetic_fixture" {
		t.Fatalf("verification not stored: %s", amazon.Verification)
	}
	if !strings.Contains(string(amazon.Rules), "productTitle") {
		t.Fatalf("rules not stored: %s", amazon.Rules)
	}

	// An administrator disables Amazon and moves it to priority 7.
	if _, err := d.UpdateSourceAdapter(ctx, store.UpdateSourceAdapterParams{ID: amazon.ID, Name: amazon.Name, DomainPatterns: amazon.DomainPatterns,
		Selectors: amazon.Selectors, Rules: amazon.Rules, Enabled: false, Priority: 7, Description: "checked by ops"}); err != nil {
		t.Fatal(err)
	}

	// Re-running with the same versions changes nothing.
	res, err = marketplaces.Sync(ctx, d, defs, nil)
	if err != nil || len(res.Unchanged) != len(defs) {
		t.Fatalf("second sync: %+v %v", res, err)
	}

	// REA ships version 2 of the Amazon definition.
	next := make([]marketplaces.Definition, len(defs))
	copy(next, defs)
	for i := range next {
		if next[i].Key == "amazon" {
			next[i].Version = 2
			next[i].Name = "Amazon (all stores)"
			next[i].Domains = append([]string{"amazon.ie", "*.amazon.ie"}, next[i].Domains...)
			next[i].Rules = json.RawMessage(strings.Replace(string(next[i].Rules), "#productTitle", "#productTitle, #title", 1))
			next[i].Verification.Note = "Version 2: also reads #title."
		}
	}
	res, err = marketplaces.Sync(ctx, d, next, nil)
	if err != nil || len(res.Upgraded) != 1 || res.Upgraded[0] != "amazon" {
		t.Fatalf("upgrade sync: %+v %v", res, err)
	}
	up := must(d.GetSourceAdapterByKey(ctx, "amazon"))
	if up.Enabled || up.Priority != 7 || up.Description != "checked by ops" {
		t.Fatalf("administrator choices lost: enabled=%v priority=%d description=%q", up.Enabled, up.Priority, up.Description)
	}
	if up.Name != "Amazon (all stores)" || up.DomainPatterns[0] != "amazon.ie" || up.DefinitionVersion != 2 || !strings.Contains(string(up.Rules), "#title") ||
		!strings.Contains(string(up.Verification), "Version 2") {
		t.Fatalf("definition not upgraded: %+v", up)
	}

	// An older shipped version never downgrades a newer stored one.
	res, err = marketplaces.Sync(ctx, d, defs, nil)
	if err != nil || len(res.Upgraded) != 0 || must(d.GetSourceAdapterByKey(ctx, "amazon")).DefinitionVersion != 2 {
		t.Fatalf("downgrade happened: %+v %v", res, err)
	}
}

func TestSyncNeverOverwritesAnAdministratorsAdapter(t *testing.T) {
	d, _ := testutil.OpenDB(t)
	ctx := t.Context()
	custom := must(d.CreateSourceAdapter(ctx, store.CreateSourceAdapterParams{Key: "temu", Name: "Our Temu selectors", Kind: "selector",
		DomainPatterns: []string{"temu.com"}, Enabled: true, Priority: 5, Selectors: json.RawMessage(`{"title":"h1"}`), Description: ""}))
	res, err := marketplaces.SyncShipped(ctx, d, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Skipped) != 1 || res.Skipped[0] != "temu" {
		t.Fatalf("expected temu to be skipped: %+v", res)
	}
	after := must(d.GetSourceAdapterByKey(ctx, "temu"))
	if after.ID != custom.ID || after.Kind != "selector" || after.Shipped || after.Name != "Our Temu selectors" {
		t.Fatalf("custom adapter overwritten: %+v", after)
	}
	// Shipped adapters cannot be deleted; custom ones can.
	if n := must(d.DeleteSourceAdapter(ctx, must(d.GetSourceAdapterByKey(ctx, "amazon")).ID)); n != 0 {
		t.Fatal("a shipped definition was deleted")
	}
	if n := must(d.DeleteSourceAdapter(ctx, custom.ID)); n != 1 {
		t.Fatal("a custom adapter could not be deleted")
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

package catalog

import (
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/store"
)

func ev(id int64, tx int64, pid uuid.UUID, source, op string, was, is bool, fields ...string) store.CatalogChangeEvent {
	return store.CatalogChangeEvent{ID: id, Txid: tx, ProductID: pid, Source: source, Op: op, WasPublished: was, IsPublished: is,
		ChangedFields: fields, CreatedAt: time.Unix(1700000000+id, 0)}
}

func kinds(cs []change) []string {
	out := []string{}
	for _, c := range cs {
		out = append(out, c.kind)
	}
	return out
}

func TestClassifyTransitionsAndKinds(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	tag := uuid.New()
	tagEv := func(id, tx int64, op string) store.CatalogChangeEvent {
		e := ev(id, tx, a, "tag", op, true, true, "tags")
		e.RefID = &tag
		return e
	}
	cases := []struct {
		name   string
		events []store.CatalogChangeEvent
		want   []string
		fields []string
	}{
		{"publish", []store.CatalogChangeEvent{ev(1, 10, a, "product", "UPDATE", false, true, "review_status", "status")}, []string{"published"}, nil},
		{"unpublish", []store.CatalogChangeEvent{ev(1, 10, a, "product", "UPDATE", true, false, "status")}, []string{"unpublished"}, nil},
		{"delete published", []store.CatalogChangeEvent{ev(1, 10, a, "product", "DELETE", true, false)}, []string{"deleted"}, nil},
		{"insert and delete in one transaction", []store.CatalogChangeEvent{ev(1, 10, a, "product", "INSERT", false, true),
			ev(2, 10, a, "product", "DELETE", true, false)}, nil, nil},
		{"publish then unpublish", []store.CatalogChangeEvent{ev(1, 10, a, "product", "UPDATE", false, true, "status"),
			ev(2, 10, a, "product", "UPDATE", true, false, "status")}, nil, nil},
		{"price", []store.CatalogChangeEvent{ev(1, 10, a, "product", "UPDATE", true, true, "price", "margin_amount")}, []string{"price_changed"},
			[]string{"pricing.price", "sourcing.margin_amount"}},
		{"variant availability", []store.CatalogChangeEvent{ev(1, 10, a, "variant", "UPDATE", true, true, "variants", "variants.availability")},
			[]string{"availability_changed"}, []string{"variants", "variants.availability"}},
		{"price and availability", []store.CatalogChangeEvent{ev(1, 10, a, "product", "UPDATE", true, true, "availability", "price"),
			ev(2, 10, a, "image", "INSERT", true, true, "images")}, []string{"price_changed", "availability_changed"},
			[]string{"availability", "images", "pricing.price"}},
		{"title", []store.CatalogChangeEvent{ev(1, 10, a, "product", "UPDATE", true, true, "title")}, []string{"updated"}, []string{"title"}},
		{"tags cleared and re-added", []store.CatalogChangeEvent{tagEv(1, 10, "DELETE"), tagEv(2, 10, "INSERT")}, nil, nil},
		{"tag added", []store.CatalogChangeEvent{tagEv(1, 10, "INSERT")}, []string{"updated"}, []string{"tags"}},
		{"status only", []store.CatalogChangeEvent{ev(1, 10, a, "product", "UPDATE", true, true, "status")}, nil, nil},
	}
	for _, tc := range cases {
		got := classify(slices.Clone(tc.events))
		if !slices.Equal(kinds(got), tc.want) && !(len(got) == 0 && tc.want == nil) {
			t.Errorf("%s: kinds %v, want %v", tc.name, kinds(got), tc.want)
			continue
		}
		if tc.fields != nil && !slices.Equal(got[0].fields, tc.fields) {
			t.Errorf("%s: fields %v, want %v", tc.name, got[0].fields, tc.fields)
		}
	}
	// Groups are per transaction and product, in event order.
	got := classify([]store.CatalogChangeEvent{ev(3, 11, b, "product", "UPDATE", true, true, "title"),
		ev(1, 10, a, "product", "UPDATE", false, true, "status"), ev(2, 11, a, "product", "UPDATE", true, true, "brand")})
	if len(got) != 3 || got[0].productID != a || got[0].kind != "published" || got[1].productID != a || got[2].productID != b {
		t.Fatalf("grouping wrong: %+v", got)
	}
}

func TestSourcingOnlyChangesAreMarked(t *testing.T) {
	a := uuid.New()
	got := classify([]store.CatalogChangeEvent{ev(1, 10, a, "product", "UPDATE", true, true, "cost", "margin_percent")})
	if len(got) != 1 || !got[0].sourcingOnly || got[0].kind != "updated" {
		t.Fatalf("cost change should be a sourcing-only update: %+v", got)
	}
	if v := VisibleFields(got[0].fields, false); len(v) != 0 {
		t.Fatalf("sourcing fields leaked: %v", v)
	}
	mixed := classify([]store.CatalogChangeEvent{ev(1, 10, a, "product", "UPDATE", true, true, "cost", "title")})
	if mixed[0].sourcingOnly || !slices.Equal(VisibleFields(mixed[0].fields, false), []string{"title"}) {
		t.Fatalf("mixed change wrong: %+v", mixed)
	}
}

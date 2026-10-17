package orders

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/waqasmani/importify-rivers-engineering/services/internal/pricing"
)

func dec(s string) *decimal.Decimal {
	d := decimal.RequireFromString(s)
	return &d
}

func str(s string) *string { return &s }

func TestDeriveStatus(t *testing.T) {
	a := LineState{Assigned: true}
	u := LineState{}
	c := LineState{Cancelled: true}
	cases := []struct {
		name      string
		cancelled bool
		lines     []LineState
		sos       []string
		want      string
		attention bool
	}{
		{"cancelled order", true, []LineState{a}, []string{SODraft}, StatusCancelled, false},
		{"every line cancelled", false, []LineState{c, c}, nil, StatusCancelled, false},
		{"unmatched line", false, []LineState{a, u}, []string{SODraft}, StatusNeedsAttention, true},
		{"unmatched while another is placed", false, []LineState{a, u}, []string{SOPlaced}, StatusNeedsAttention, true},
		{"nothing prepared", false, []LineState{}, nil, StatusReceived, false},
		{"all drafts", false, []LineState{a, a}, []string{SODraft, SODraft}, StatusAwaitingConfirmation, false},
		{"draft and confirmed", false, []LineState{a, a}, []string{SODraft, SOConfirmed}, StatusInProgress, false},
		{"failed needs attention", false, []LineState{a}, []string{SOFailed}, StatusInProgress, true},
		{"draft and placed", false, []LineState{a, a}, []string{SODraft, SOPlaced}, StatusInProgress, false},
		{"all placed", false, []LineState{a, a}, []string{SOPlaced, SOPlaced}, StatusPlaced, false},
		{"one shipped", false, []LineState{a, a}, []string{SOPlaced, SOShipped}, StatusPartiallyShipped, false},
		{"one delivered one confirmed", false, []LineState{a, a}, []string{SOConfirmed, SODelivered}, StatusPartiallyShipped, false},
		{"all shipped", false, []LineState{a, a}, []string{SOShipped, SODelivered}, StatusShipped, false},
		{"all delivered", false, []LineState{a, a}, []string{SODelivered, SODelivered}, StatusDelivered, false},
		{"cancelled supplier orders ignored", false, []LineState{a, c}, []string{SOCancelled, SOPlaced}, StatusPlaced, false},
		{"cancelled line ignored", false, []LineState{a, c}, []string{SODraft}, StatusAwaitingConfirmation, false},
	}
	for _, tc := range cases {
		got, attention := Derive(tc.cancelled, tc.lines, tc.sos)
		if got != tc.want || attention != tc.attention {
			t.Errorf("%s: got %s/%v, want %s/%v", tc.name, got, attention, tc.want, tc.attention)
		}
	}
}

func sampleContent() Content {
	pid := uuid.MustParse("6a0e6d8e-6f43-4b5e-9d11-1f8c7c1a0001")
	return Content{
		SupplierOrderID: uuid.MustParse("6a0e6d8e-6f43-4b5e-9d11-1f8c7c1a0002"), SupplierName: "Ningbo Flasks", SourceDomain: "supplier.example",
		PlacementMethod: PlacementManual,
		Items: []Item{{LineID: uuid.MustParse("6a0e6d8e-6f43-4b5e-9d11-1f8c7c1a0003"), ProductID: &pid, SKU: "FL-1", Title: "Flask",
			Options: map[string]string{"Size": "1 L", "Color": "Black"}, Quantity: 2, SourceURL: str("https://supplier.example/p/1"),
			ExpectedUnitCost: dec("11.2000"), CostCurrency: str("EUR")}},
		ExpectedCostTotal: dec("22.4000"), CostCurrency: str("EUR"),
		ShipTo: Address{Name: "Ada", Line1: "1 Main St", City: "Berlin", PostalCode: "10115", Country: "DE"},
	}
}

func TestContentHashIsCanonical(t *testing.T) {
	c := sampleContent()
	h := ContentHash(c)
	if len(h) != 64 {
		t.Fatalf("hash %q", h)
	}
	// The same content with differently padded decimals and options built in another order.
	same := sampleContent()
	same.Items[0].ExpectedUnitCost = dec("11.2")
	same.Items[0].Options = map[string]string{"Color": "Black", "Size": "1 L"}
	same.ExpectedCostTotal = dec("22.40")
	if ContentHash(same) != h {
		t.Fatal("hash depends on decimal padding or option order")
	}
	// Every confirmed field changes the hash.
	mutations := map[string]func(*Content){
		"quantity":  func(c *Content) { c.Items[0].Quantity = 3 },
		"variant":   func(c *Content) { v := uuid.New(); c.Items[0].VariantID = &v },
		"options":   func(c *Content) { c.Items[0].Options["Color"] = "Red" },
		"url":       func(c *Content) { c.Items[0].SourceURL = str("https://supplier.example/p/2") },
		"cost":      func(c *Content) { c.Items[0].ExpectedUnitCost = dec("11.3") },
		"ship_to":   func(c *Content) { c.ShipTo.Line1 = "2 Main St" },
		"method":    func(c *Content) { c.PlacementMethod = PlacementAutomation },
		"note":      func(c *Content) { c.NoteToSupplier = "Gift wrap" },
		"supplier":  func(c *Content) { s := uuid.New(); c.SupplierID = &s },
		"warnings":  func(c *Content) { c.Warnings = []string{WarnMissingCost + ":"} },
		"new item":  func(c *Content) { c.Items = append(c.Items, Item{LineID: uuid.New(), Quantity: 1}) },
		"total":     func(c *Content) { c.ExpectedCostTotal = nil },
		"so id":     func(c *Content) { c.SupplierOrderID = uuid.New() },
		"recipient": func(c *Content) { c.ShipTo.Name = "Bob" },
	}
	for name, mutate := range mutations {
		m := sampleContent()
		mutate(&m)
		if ContentHash(m) == h {
			t.Errorf("changing %s does not change the hash", name)
		}
	}
}

func TestWarnings(t *testing.T) {
	pid, pid2, vid := uuid.New(), uuid.New(), uuid.New()
	l1, l2, l3 := uuid.New(), uuid.New(), uuid.New()
	items := []Item{
		{LineID: l1, ProductID: &pid, VariantID: &vid, Title: "Flask", VariantTitle: "Black", Quantity: 1, SourceURL: str("https://s.example/1"),
			ExpectedUnitCost: dec("12"), CostCurrency: str("EUR")},
		{LineID: l2, ProductID: &pid2, Title: "Mug", Quantity: 1, SourceURL: nil},
		{LineID: l3, ProductID: nil, Title: "Gone", Quantity: 1, SourceURL: str("https://s.example/3"), ExpectedUnitCost: dec("1"), CostCurrency: str("USD")},
	}
	in := WarningInput{
		Items:  items,
		ShipTo: Address{Name: "Ada", Line1: "1 Main St", Country: "DE"},
		Products: map[uuid.UUID]ProductState{
			pid:  {Published: true, Status: "active", ReviewStatus: "approved", Availability: "in_stock", HasVariants: true},
			pid2: {Published: false, Status: "draft", ReviewStatus: "pending", Availability: "unknown", HasVariants: true},
		},
		Variants:        map[uuid.UUID]string{vid: "out_of_stock"},
		SalePrices:      map[uuid.UUID]*decimal.Decimal{l1: dec("10"), l3: dec("5")},
		SaleCurrency:    "EUR",
		Rates:           pricing.Rates{Base: "EUR", Rates: map[string]decimal.Decimal{}},
		UnassignedLines: 1,
	}
	ws := Warnings(in)
	got := WarningKeys(ws)
	want := []string{
		WarnIncompleteAddress + ":", WarnMixedCurrencies + ":", WarnUnmatchedLines + ":",
		WarnAvailability + ":" + l1.String(), WarnNegativeMargin + ":" + l1.String(),
		WarnAvailability + ":" + l2.String(), WarnMissingCost + ":" + l2.String(), WarnNoSourceURL + ":" + l2.String(),
		WarnNotPublished + ":" + l2.String(), WarnVariantNotChosen + ":" + l2.String(),
		WarnProductMissing + ":" + l3.String(),
	}
	slices.Sort(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("warnings\n got: %v\nwant: %v", got, want)
	}
	for _, w := range ws {
		if w.Message == "" {
			t.Errorf("%s has no message", w.Code)
		}
	}
	// Without a rate, a USD cost cannot be compared with a EUR sale price: no margin warning.
	if slices.Contains(got, WarnNegativeMargin+":"+l3.String()) {
		t.Fatal("margin compared across currencies without a rate")
	}
	// With a rate, it can (5 EUR sale < 1 USD cost × 10).
	in.Rates.Rates["USD"] = decimal.NewFromInt(10)
	if !slices.Contains(WarningKeys(Warnings(in)), WarnNegativeMargin+":"+l3.String()) {
		t.Fatal("converted margin not checked")
	}

	clean := WarningInput{Items: items[:1], ShipTo: Address{Name: "Ada", Line1: "1 Main St", City: "Berlin", PostalCode: "10115", Country: "DE"},
		Products:   map[uuid.UUID]ProductState{pid: {Published: true, Availability: "in_stock", HasVariants: true}},
		Variants:   map[uuid.UUID]string{vid: "in_stock"},
		SalePrices: map[uuid.UUID]*decimal.Decimal{l1: dec("20")}, SaleCurrency: "EUR"}
	if ws := Warnings(clean); len(ws) != 0 {
		t.Fatalf("expected no warnings, got %v", ws)
	}
}

func TestTotal(t *testing.T) {
	sum, cur := Total([]Item{{Quantity: 2, ExpectedUnitCost: dec("1.5"), CostCurrency: str("EUR")}, {Quantity: 1, ExpectedUnitCost: dec("3"), CostCurrency: str("EUR")}})
	if sum == nil || sum.String() != "6" || *cur != "EUR" {
		t.Fatalf("total %v %v", sum, cur)
	}
	if s, _ := Total([]Item{{Quantity: 1, ExpectedUnitCost: dec("1"), CostCurrency: str("EUR")}, {Quantity: 1, ExpectedUnitCost: dec("1"), CostCurrency: str("USD")}}); s != nil {
		t.Fatal("mixed currencies must not sum")
	}
	if s, _ := Total([]Item{{Quantity: 1}}); s != nil {
		t.Fatal("a missing cost must not sum")
	}
}

func TestValidCountry(t *testing.T) {
	for _, ok := range []string{"DE", "US", "GB", "CN", "BR", "JP", "ZA", "CH"} {
		if !ValidCountry(ok) {
			t.Errorf("%s rejected", ok)
		}
	}
	for _, bad := range []string{"", "D", "DEU", "de", "ZZ", "EU", "UN", "XX", "QO", "1A", "UK", "SU", "AC", "EZ"} {
		if ValidCountry(bad) {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestEventErasureAndCosts(t *testing.T) {
	ev := Event{Type: EventSOConfirmed, Order: EventOrder{Customer: Customer{Name: "Ada Lovelace", Email: "ada@example.com", Phone: "+49 30 1"},
		ShippingAddress: Address{Name: "Ada Lovelace", Line1: "1 Analytical St", City: "London", PostalCode: "N1", Country: "GB"}, CustomerNote: "Ring twice"},
		SupplierOrder: &EventSupplierOrder{ShipTo: Address{Name: "Ada Lovelace", Line1: "1 Analytical St", Country: "GB"}, NoteToSupplier: "For Ada",
			TrackingNumber: "TRK1", Costs: &EventCosts{ExpectedCostTotal: dec("3")},
			Items: []EventItem{{Title: "Flask", Cost: &EventItemCost{ExpectedUnitCost: dec("3")}}}}}
	b, _ := json.Marshal(ev.Erased())
	for _, secret := range []string{"Ada", "ada@example.com", "+49", "Analytical", "London", "Ring twice"} {
		if strings.Contains(string(b), secret) {
			t.Errorf("erased event still contains %q: %s", secret, b)
		}
	}
	if !strings.Contains(string(b), `"country":"GB"`) || !strings.Contains(string(b), `"customer_erased":true`) || !strings.Contains(string(b), "TRK1") {
		t.Fatalf("erasure removed too much: %s", b)
	}
	plain, _ := json.Marshal(ev.WithoutCosts())
	if strings.Contains(string(plain), `"costs"`) || strings.Contains(string(plain), `"cost"`) {
		t.Fatalf("costs not removed: %s", plain)
	}
	if ev.SupplierOrder.Costs == nil || ev.SupplierOrder.Items[0].Cost == nil {
		t.Fatal("WithoutCosts modified the original")
	}
	for _, e := range EventTypes {
		if !IsEventType(e.Type) || e.Description == "" {
			t.Errorf("event type %v", e)
		}
	}
	if IsEventType("product.published") || IsEventType("order.created") {
		t.Fatal("unknown event accepted")
	}
}

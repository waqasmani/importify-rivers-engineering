package orders

// LineState is what status derivation needs to know about a customer order line.
type LineState struct {
	Cancelled bool
	// Assigned is true while the line belongs to a supplier order that is not cancelled.
	Assigned bool
}

// Derive computes a customer order's status and whether it needs attention from its lines and the
// statuses of its supplier orders. Cancelled supplier orders release their lines, so a line that is
// not assigned and not cancelled still has to be mapped, prepared or cancelled by a person.
//
//   - cancelled: the order was cancelled, or every line was cancelled
//   - needs_attention: some line is not in any supplier order (unmatched, or its supplier order
//     was cancelled)
//   - received: nothing has been prepared yet
//   - awaiting_confirmation: every supplier order is a draft
//   - delivered / shipped / partially_shipped / placed: by the supplier orders' progress
//   - in_progress: anything else (confirmed, failed, or a mix of drafts and placed orders)
//
// needsAttention is also true when a supplier order failed and waits for a retry or a switch to
// manual placement.
func Derive(cancelled bool, lines []LineState, supplierOrders []string) (status string, needsAttention bool) {
	if cancelled {
		return StatusCancelled, false
	}
	active := 0
	unassigned := 0
	for _, l := range lines {
		if l.Cancelled {
			continue
		}
		active++
		if !l.Assigned {
			unassigned++
		}
	}
	if len(lines) > 0 && active == 0 {
		return StatusCancelled, false
	}
	var open []string
	failed := false
	for _, st := range supplierOrders {
		if st == SOCancelled {
			continue
		}
		open = append(open, st)
		if st == SOFailed {
			failed = true
		}
	}
	needsAttention = unassigned > 0 || failed
	if unassigned > 0 {
		return StatusNeedsAttention, needsAttention
	}
	if len(open) == 0 {
		return StatusReceived, needsAttention
	}
	all := func(ok func(string) bool) bool {
		for _, st := range open {
			if !ok(st) {
				return false
			}
		}
		return true
	}
	some := func(ok func(string) bool) bool {
		for _, st := range open {
			if ok(st) {
				return true
			}
		}
		return false
	}
	shipped := func(st string) bool { return st == SOShipped || st == SODelivered }
	switch {
	case all(func(st string) bool { return st == SODraft }):
		return StatusAwaitingConfirmation, needsAttention
	case all(func(st string) bool { return st == SODelivered }):
		return StatusDelivered, needsAttention
	case all(shipped):
		return StatusShipped, needsAttention
	case some(shipped):
		return StatusPartiallyShipped, needsAttention
	case all(Placed):
		return StatusPlaced, needsAttention
	}
	return StatusInProgress, needsAttention
}

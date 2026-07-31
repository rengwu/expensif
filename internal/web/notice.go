package web

import (
	"fmt"
	"sync"
)

// Notice carries a message from a background worker to the next page a human
// loads. Generation runs on a ticker rather than on a request, so it has no
// response to write a Flash into — this is the whole of the seam between the
// two: the goroutine leaves a count here, the next rendered page takes it and
// shows it once.
//
// It is deliberately in memory and nowhere else. The message is a courtesy —
// the Expenses themselves are the durable record — so it is worth exactly as
// much as it costs, and a process that dies before anyone looks has lost
// nothing that matters. Persisting it would mean a table, a "seen" flag and a
// GET that writes, all to redeliver a sentence.
type Notice struct {
	mu      sync.Mutex
	charges int
}

// AddCharges records that generation minted n Expenses. Counts accumulate until
// somebody reads them, so two ticks either side of a page load report their sum
// rather than the last one only.
func (n *Notice) AddCharges(count int) {
	if count <= 0 {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	n.charges += count
}

// Take returns the pending message and clears it — a Flash is shown once — or
// an empty string when nothing is pending. The count is every Expense minted:
// the ones backfilled behind today and the one materialized ahead of it alike.
func (n *Notice) Take() string {
	n.mu.Lock()
	defer n.mu.Unlock()
	charges := n.charges
	n.charges = 0
	switch {
	case charges <= 0:
		return ""
	case charges == 1:
		return "1 recurring charge added"
	default:
		return fmt.Sprintf("%d recurring charges added", charges)
	}
}

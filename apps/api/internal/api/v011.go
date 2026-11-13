package api

import "net/http"

// registerV011 wires V0.1.1 additions separately from the V0.1 Register so the
// already-verified handlers are never rewritten. Each milestone adds one line.
func (a *API) registerV011(mux *http.ServeMux) {
	a.registerV011Credit(mux)
	a.registerV011Debt(mux)
	a.registerV011Installment(mux)
	a.route(mux, "GET /api/v1/schedule/runs", a.handleListScheduleRuns)
}

// ptrArg converts an optional JSON field into a SQL parameter: a nil pointer
// becomes SQL NULL so COALESCE(?, column) keeps the stored value.
func ptrArg[T any](p *T) any {
	if p == nil {
		return nil
	}
	return *p
}

// scopedKey namespaces an idempotency key by household. Two households may use
// the same client key without contending for one global UNIQUE slot, and a
// replay can never resolve to another household's business row.
func scopedKey(householdID, key string) string {
	if key == "" {
		return ""
	}
	return householdID + ":" + key
}

package api

import (
	"net/http"
	"testing"
)

// TestRouteBudget freezes the V2 route surface: Register must wire at most maxRoutes
// routes (method + path). The count is recorded by the API's registration helper,
// so this test fails as soon as a route is added without removing another.
func TestRouteBudget(t *testing.T) {
	a := New(nil)
	mux := http.NewServeMux()
	a.Register(mux)
	t.Logf("Register wired %d routes (budget %d)", a.routeCount, maxRoutes)
	if a.routeCount > maxRoutes {
		t.Fatalf("Register wired %d routes, budget is %d", a.routeCount, maxRoutes)
	}
	// Registering twice must recount from zero, not accumulate.
	a.Register(http.NewServeMux())
	if a.routeCount > maxRoutes {
		t.Fatalf("re-registration counted %d routes, budget is %d", a.routeCount, maxRoutes)
	}
}

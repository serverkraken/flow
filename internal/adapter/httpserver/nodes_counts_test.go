package httpserver_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

// TestCreateNode_CountsTowardTarget_ExplicitFalse verifies that posting
// countsTowardTarget:false persists false (overrides the server-side default true).
func TestCreateNode_CountsTowardTarget_ExplicitFalse(t *testing.T) {
	_, do, _ := newProjectsSrv(t)

	res := do("POST", "/api/v1/nodes", `{"name":"Work","kind":"engagement","countsTowardTarget":false}`)
	if res.StatusCode != http.StatusCreated {
		_ = res.Body.Close()
		t.Fatalf("create status %d, want 201", res.StatusCode)
	}
	var n domain.Node
	_ = json.NewDecoder(res.Body).Decode(&n)
	_ = res.Body.Close()
	if n.CountsTowardTarget {
		t.Fatal("countsTowardTarget explicit false: want false, got true")
	}
}

// TestCreateNode_CountsTowardTarget_OmittedDefaultsTrue verifies that omitting
// countsTowardTarget from the request preserves the domain default of true.
func TestCreateNode_CountsTowardTarget_OmittedDefaultsTrue(t *testing.T) {
	_, do, _ := newProjectsSrv(t)

	res := do("POST", "/api/v1/nodes", `{"name":"Work","kind":"engagement"}`)
	if res.StatusCode != http.StatusCreated {
		_ = res.Body.Close()
		t.Fatalf("create status %d, want 201", res.StatusCode)
	}
	var n domain.Node
	_ = json.NewDecoder(res.Body).Decode(&n)
	_ = res.Body.Close()
	if !n.CountsTowardTarget {
		t.Fatal("countsTowardTarget omitted: want true (default), got false")
	}
}

// TestUpdateNode_CountsTowardTarget verifies that PATCH with false persists false
// and that a subsequent PATCH omitting the field preserves the existing false.
func TestUpdateNode_CountsTowardTarget(t *testing.T) {
	_, do, _ := newProjectsSrv(t)

	// Create a node (default countsTowardTarget=true).
	res := do("POST", "/api/v1/nodes", `{"name":"Work","kind":"engagement"}`)
	if res.StatusCode != http.StatusCreated {
		_ = res.Body.Close()
		t.Fatalf("create status %d, want 201", res.StatusCode)
	}
	var n domain.Node
	_ = json.NewDecoder(res.Body).Decode(&n)
	_ = res.Body.Close()
	if !n.CountsTowardTarget {
		t.Fatalf("newly created node: want countsTowardTarget=true, got false")
	}

	// PATCH with explicit false → persists false.
	res = do("PATCH", "/api/v1/nodes/"+n.ID,
		`{"name":"Work","slug":"work","status":"active","countsTowardTarget":false}`)
	if res.StatusCode != http.StatusOK {
		_ = res.Body.Close()
		t.Fatalf("PATCH false status %d, want 200", res.StatusCode)
	}
	var upd domain.Node
	_ = json.NewDecoder(res.Body).Decode(&upd)
	_ = res.Body.Close()
	if upd.CountsTowardTarget {
		t.Fatal("after PATCH with false: want false, got true")
	}

	// PATCH omitting countsTowardTarget → existing false preserved.
	res = do("PATCH", "/api/v1/nodes/"+n.ID,
		`{"name":"Work","slug":"work","status":"active"}`)
	if res.StatusCode != http.StatusOK {
		_ = res.Body.Close()
		t.Fatalf("PATCH omit status %d, want 200", res.StatusCode)
	}
	var upd2 domain.Node
	_ = json.NewDecoder(res.Body).Decode(&upd2)
	_ = res.Body.Close()
	if upd2.CountsTowardTarget {
		t.Fatal("after PATCH with omit: want existing false preserved, got true")
	}
}

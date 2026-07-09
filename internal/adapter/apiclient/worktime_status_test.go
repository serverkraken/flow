package apiclient_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/apiclient"
)

func TestGetWorktimeStatus_Decodes(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/worktime/status" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"date":"2026-07-08","loggedMin":312,"targetMin":480,"running":true,"activeSessionId":"s1","activeStart":"2026-07-08T13:05:00+02:00","week":[{"date":"2026-07-06","loggedMin":480,"targetMin":480,"workday":true,"isToday":false,"dayOffKind":null}],"streak":4,"burndown":{"saldoMin":130,"targetMin":9600}}`))
	}))
	defer ts.Close()
	c := apiclient.New(ts.URL, "tok")
	st, err := c.GetWorktimeStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.LoggedMin != 312 || !st.Running || st.ActiveSessionID != "s1" || st.Streak != 4 || len(st.Week) != 1 {
		t.Fatalf("bad decode: %+v", st)
	}
	if st.Burndown.SaldoMin != 130 || st.Burndown.TargetMin != 9600 {
		t.Fatalf("bad burndown decode: %+v", st.Burndown)
	}
	if st.ActiveNodeID != nil {
		t.Errorf("absent activeNodeId should decode to nil, got %v", st.ActiveNodeID)
	}
}

package stack

import (
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A handler that panics ends its request in the log, and the server's
// error log keeps the panic.
func TestMiddleware_RecordsAPanickingHandler(t *testing.T) {
	reqs := &RequestLog{prefix: "t."}
	errs := &errorLog{}
	srv := httptest.NewUnstartedServer(middleware(reqs, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	})))
	srv.Config.ErrorLog = log.New(errs, "", 0)
	srv.Start()
	defer srv.Close()

	if resp, err := http.Get(srv.URL + "/x"); err == nil {
		resp.Body.Close()
	}
	got := reqs.Requests()
	if len(got) != 1 || !got[0].Done() || got[0].ID != "t.1" || got[0].Path != "/x" {
		t.Fatalf("requests = %+v, want t.1 done", got)
	}
	if len(reqs.Active()) != 0 {
		t.Errorf("the panicked request still runs: %+v", reqs.Active())
	}
	if e := errs.entries(); len(e) != 1 || !strings.Contains(e[0], "http: panic serving") || !strings.Contains(e[0], "boom") {
		t.Errorf("error log = %q", e)
	}
}

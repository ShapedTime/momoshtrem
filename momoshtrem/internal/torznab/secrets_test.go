package torznab

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestNoSecretLeaks drives every failure path and checks that neither the
// API key nor a download link reaches a returned error, a health or
// progress string, or a log line.
func TestNoSecretLeaks(t *testing.T) {
	const passkey = "tracker-passkey-0123"
	link := "http://jackett.test/dl/thepiratebay/?jackett_apikey=" + testKey + "&path=SECRETPATH&file=x"
	secrets := []string{testKey, passkey, "SECRETPATH", "jackett_apikey", "apikey="}

	// Jackett's own error text, quoting the key and a tracker URL.
	leakyDescription := fmt.Sprintf(
		`<error code="900" description="Jackett.Common.IndexerException: Exception (thepiratebay): GET https://tracker.example/announce?passkey=%s&amp;apikey=%s failed for key %s" />`,
		passkey, testKey, testKey)
	leakyJSON := fmt.Sprintf(`{"result":"error","error":"Bad request %s with key %s"}`, link, testKey)

	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	var texts []string
	collect := func(what string, err error) {
		t.Helper()
		if err == nil {
			t.Errorf("%s: want an error", what)
			return
		}
		texts = append(texts, what+": "+err.Error())
	}

	j := newFakeJackett(t)
	j.handle("thepiratebay", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("q") {
		case "doc":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(leakyDescription))
		case "json":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(leakyJSON))
		case "slow":
			<-r.Context().Done()
		default:
			_, _ = w.Write([]byte("not xml " + link))
		}
	})
	j.dl = func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("file") {
		case "redirect":
			http.Redirect(w, r, "http://elsewhere.example/get?jackett_apikey="+testKey, http.StatusFound)
		case "big":
			_, _ = w.Write(bytes.Repeat([]byte("x"), maxTorrent+1))
		case "doc":
			_, _ = w.Write([]byte(leakyDescription))
		default:
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(leakyJSON))
		}
	}
	c := New(j.URL, testKey, WithLogger(logger), WithTimeouts(50*time.Millisecond, 50*time.Millisecond))
	ctx := context.Background()

	for _, q := range []string{"doc", "json", "slow", "garbage"} {
		_, err := c.Search(ctx, "thepiratebay", Query{Q: q})
		collect("search "+q, err)
	}
	for _, file := range []string{"redirect", "big", "doc", "error"} {
		_, _, err := c.Download(ctx, strings.Replace(link, "file=x", "file="+file, 1))
		collect("download "+file, err)
	}
	_, _, err := c.Download(ctx, "http://jackett.test/elsewhere?jackett_apikey="+testKey)
	collect("download invalid link", err)

	responses := c.Fanout(ctx, []Request{{"thepiratebay", Query{Q: "doc"}}, {"thepiratebay", Query{Q: "json"}}},
		FanoutOptions{OnProgress: func(p Progress) { texts = append(texts, "progress: "+p.Error) }})
	for _, r := range responses {
		collect("fanout", r.Err)
	}
	for _, h := range c.Snapshot() {
		texts = append(texts, "health: "+h.Error)
	}

	// Transport errors from a Jackett that's gone.
	gone := httptest.NewServer(http.NotFoundHandler())
	gone.Close()
	dead := New(gone.URL, testKey, WithLogger(logger))
	_, err = dead.Indexers(ctx)
	collect("indexers, connection refused", err)
	_, _, err = dead.Download(ctx, link)
	collect("download, connection refused", err)

	all := strings.Join(texts, "\n") + "\n" + logs.String()
	for _, s := range secrets {
		if strings.Contains(all, s) {
			t.Errorf("%q leaked:\n%s", s, all)
		}
	}
	// The cleaned text still says what went wrong.
	if !strings.Contains(all, "GET [link] failed for key [key]") {
		t.Errorf("Jackett's error text lost its meaning:\n%s", all)
	}
}

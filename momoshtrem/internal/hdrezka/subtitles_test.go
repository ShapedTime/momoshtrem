package hdrezka

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
)

const vttBody = "WEBVTT\n\n00:00:01.000 --> 00:00:02.000\nHello.\n"

func TestSubtitle(t *testing.T) {
	big := bytes.Repeat([]byte("x"), maxSubtitle+1)
	tests := []struct {
		name      string
		handler   http.HandlerFunc
		want      string
		wantErr   error
		wantCalls int
	}{
		{"a file", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(vttBody)) }, vttBody, nil, 1},
		{"a refused link", withStatus(http.StatusForbidden), "", ErrLinkExpired, 1},
		{"a missing file", withStatus(http.StatusNotFound), "", ErrLinkExpired, 1},
		{"over 2 MB, said up front", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Length", strconv.Itoa(len(big)))
			_, _ = w.Write(big)
		}, "", ErrSubtitleTooLarge, 1},
		{"over 2 MB, streamed", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Transfer-Encoding", "chunked")
			w.(http.Flusher).Flush()
			_, _ = w.Write(big)
		}, "", ErrSubtitleTooLarge, 1},
		{"a server error, three times", withStatus(http.StatusBadGateway), "", &StatusError{StatusCode: http.StatusBadGateway}, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var mu sync.Mutex
			calls := 0
			var agent string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				calls++
				agent = r.Header.Get("User-Agent")
				mu.Unlock()
				tt.handler(w, r)
			}))
			defer srv.Close()
			c, _ := lengthClient()

			got, err := c.Subtitle(context.Background(), srv.URL+"/0be3bfb771ec26f0:2026101618:dlFOU2k3VzR6/en.vtt")
			switch want := tt.wantErr.(type) {
			case nil:
				if err != nil || string(got) != tt.want {
					t.Fatalf("Subtitle = %q, %v", got, err)
				}
			case *StatusError:
				var se *StatusError
				if !errors.As(err, &se) || se.StatusCode != want.StatusCode {
					t.Fatalf("err = %v, want %v", err, want)
				}
			default:
				if !errors.Is(err, want) {
					t.Fatalf("err = %v, want %v", err, want)
				}
			}
			if calls != tt.wantCalls {
				t.Errorf("calls = %d, want %d", calls, tt.wantCalls)
			}
			if agent != DefaultUserAgent {
				t.Errorf("User-Agent = %q, want the client's", agent)
			}
		})
	}
}

func TestSubtitle_RetriesThenSucceeds(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		calls++
		n := calls
		mu.Unlock()
		if n == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(vttBody))
	}))
	defer srv.Close()
	c, sleeps := lengthClient()

	got, err := c.Subtitle(context.Background(), srv.URL+"/en.vtt")
	if err != nil || string(got) != vttBody || len(*sleeps) != 1 {
		t.Fatalf("Subtitle = %q, %v after %d sleeps", got, err, len(*sleeps))
	}
}

func TestSubtitle_TransportErrorHidesLink(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	link := srv.URL + "/0be3bfb771ec26f0:2026101618:dlFOU2k3VzR6/en.vtt"
	srv.Close() // connection refused from here on

	c, _ := lengthClient()
	_, err := c.Subtitle(context.Background(), link)
	if err == nil {
		t.Fatal("want an error from a closed server")
	}
	if strings.Contains(err.Error(), "dlFOU2k3VzR6") || strings.Contains(err.Error(), "en.vtt") {
		t.Errorf("error leaks the signed link: %v", err)
	}
}

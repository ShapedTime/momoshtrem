package status

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// maxProbeBody caps how much of a probe response is read.
const maxProbeBody = 1 << 20

// JackettProber checks Jackett's Torznab caps endpoint, which needs a valid
// API key but doesn't query any indexer.
func JackettProber(client *http.Client, baseURL, apiKey string) Prober {
	endpoint := strings.TrimRight(baseURL, "/") + "/api/v2.0/indexers/all/results/torznab/api"
	return func(ctx context.Context) error {
		u, err := url.Parse(endpoint)
		if err != nil {
			return errors.New("invalid JACKETT_URL")
		}
		q := u.Query()
		q.Set("t", "caps")
		q.Set("apikey", apiKey)
		u.RawQuery = q.Encode()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return errors.New("invalid JACKETT_URL")
		}

		resp, err := client.Do(req)
		if err != nil {
			return withoutURL(err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(io.LimitReader(resp.Body, maxProbeBody))
		if err != nil {
			return fmt.Errorf("reading response: %w", withoutURL(err))
		}

		// Jackett reports a bad API key as a Torznab <error> document.
		if desc, ok := torznabError(body); ok {
			return fmt.Errorf("jackett error: %s", desc)
		}
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return fmt.Errorf("jackett rejected the API key (HTTP %d)", resp.StatusCode)
		}
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			return fmt.Errorf("unexpected response (HTTP %d)", resp.StatusCode)
		}
		return nil
	}
}

// torznabError extracts the description from a Torznab error document such as
// <error code="100" description="Invalid API Key"/>.
func torznabError(body []byte) (string, bool) {
	var doc struct {
		XMLName     xml.Name
		Code        string `xml:"code,attr"`
		Description string `xml:"description,attr"`
	}
	if err := xml.Unmarshal(body, &doc); err != nil || doc.XMLName.Local != "error" {
		return "", false
	}
	desc := doc.Description
	if desc == "" {
		desc = "code " + doc.Code
	}
	return desc, true
}

// withoutURL strips the request URL from transport errors, because the URL
// carries the API key.
func withoutURL(err error) error {
	var uerr *url.Error
	if errors.As(err, &uerr) {
		return uerr.Err
	}
	return err
}

package realimage_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// NetherNet exposes discovery over HTTP on server-port. Gameplay uses a
// separately negotiated WebRTC connection, exercised by the full client shard.
func (s *scenarioState) netherNetEventuallyResponds() error {
	// Keep discovery connections scoped to this step, including across retries.
	client := &http.Client{Transport: &http.Transport{}}
	defer client.CloseIdleConnections()
	return eventually("NetherNet discovery response from packaged Bedrock", realImageTimeout, func() error {
		if s.signalingURL == "" {
			return fmt.Errorf("candidate signaling endpoint is not available")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return discoverNetherNet(ctx, client, s.signalingURL)
	})
}

func discoverNetherNet(ctx context.Context, client *http.Client, address string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address+"/v1/join", nil)
	if err != nil {
		return err
	}
	request.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("NetherNet discovery returned %s", response.Status)
	}
	var status struct {
		Name     string `json:"name"`
		Protocol int    `json:"protocol"`
		Version  string `json:"version"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&status); err != nil {
		return fmt.Errorf("decode NetherNet discovery: %w", err)
	}
	if strings.TrimSpace(status.Name) == "" || status.Protocol <= 0 || strings.TrimSpace(status.Version) == "" {
		return fmt.Errorf("incomplete NetherNet discovery: %+v", status)
	}
	return nil
}

func TestNetherNetDiscovery(t *testing.T) {
	for _, test := range []struct {
		name      string
		code      int
		body      string
		wantError bool
	}{
		{"server status", http.StatusOK, `{"name":"Montainer","protocol":2225,"version":"1.26.60","players":0}`, false},
		{"other successful status", http.StatusCreated, `{"name":"Montainer","protocol":2225,"version":"1.26.60"}`, false},
		{"unavailable", http.StatusServiceUnavailable, `{}`, true},
		{"management health is not discovery", http.StatusOK, `{"status":"ok"}`, true},
		{"invalid JSON", http.StatusOK, `<html>proxy error</html>`, true},
		{"missing version", http.StatusOK, `{"name":"Montainer","protocol":2225}`, true},
		{"invalid protocol", http.StatusOK, `{"name":"Montainer","protocol":0,"version":"1.26.60"}`, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/join" {
					t.Errorf("unexpected discovery request: %s %s", r.Method, r.URL)
				}
				w.WriteHeader(test.code)
				_, _ = io.WriteString(w, test.body)
			}))
			defer server.Close()
			err := discoverNetherNet(context.Background(), server.Client(), server.URL)
			if (err != nil) != test.wantError {
				t.Fatalf("discovery error = %v, want error = %t", err, test.wantError)
			}
		})
	}
}

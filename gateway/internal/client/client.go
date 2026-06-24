package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"chillcheck-gateway/internal/reading"
)

type Client struct {
	baseURL string
	key     string
	hc      *http.Client
}

func New(baseURL, key string) *Client {
	return &Client{
		baseURL: baseURL,
		key:     key,
		hc:      &http.Client{Timeout: 15 * time.Second},
	}
}

type ingestResp struct {
	Accepted int `json:"accepted"`
	Ignored  int `json:"ignored"`
}

// Send posts a batch of readings. It returns an error on any network failure or
// non-2xx response, which signals the caller to spool the batch for retry.
func (c *Client) Send(ctx context.Context, rs []reading.Reading) (accepted, ignored int, err error) {
	if len(rs) == 0 {
		return 0, 0, nil
	}
	body, err := json.Marshal(map[string]any{"readings": rs})
	if err != nil {
		return 0, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/ingest/readings", bytes.NewReader(body))
	if err != nil {
		return 0, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Gateway-Key", c.key)

	resp, err := c.hc.Do(req)
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, 0, fmt.Errorf("ingest returned %s", resp.Status)
	}
	var out ingestResp
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return out.Accepted, out.Ignored, nil
}

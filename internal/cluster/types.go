package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type NodeInfo struct {
	ID   string `json:"id"`
	Addr string `json:"addr"`
}

type RegisterRequest struct {
	Node NodeInfo `json:"node"`
}

type BroadcastRequest struct {
	Path    string          `json:"path"`
	Payload json.RawMessage `json:"payload"`
}

var httpClient = &http.Client{Timeout: 5 * time.Second}

func PostJSON(ctx context.Context, url string, body any, out any) error {
	reqBody, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("http %s: %d", url, resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func GetJSON(ctx context.Context, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("http %s: %d", url, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

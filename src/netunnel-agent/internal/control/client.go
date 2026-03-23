package control

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

type Agent struct {
	ID            string `json:"id"`
	UserID        string `json:"user_id"`
	Name          string `json:"name"`
	MachineCode   string `json:"machine_code"`
	SecretKey     string `json:"secret_key"`
	Status        string `json:"status"`
	ClientVersion string `json:"client_version"`
	OSType        string `json:"os_type"`
}

type Tunnel struct {
	ID         string `json:"id"`
	UserID     string `json:"user_id"`
	AgentID    string `json:"agent_id"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	Status     string `json:"status"`
	Enabled    bool   `json:"enabled"`
	LocalHost  string `json:"local_host"`
	LocalPort  int    `json:"local_port"`
	RemotePort *int   `json:"remote_port"`
}

type DomainRoute struct {
	ID         string  `json:"id"`
	TunnelID   string  `json:"tunnel_id"`
	Domain     string  `json:"domain"`
	Scheme     string  `json:"scheme"`
	CertSource string  `json:"cert_source"`
	CertID     *string `json:"cert_id"`
}

type RegisterRequest struct {
	UserID        string `json:"user_id"`
	Name          string `json:"name"`
	MachineCode   string `json:"machine_code"`
	ClientVersion string `json:"client_version"`
	OSType        string `json:"os_type"`
}

type HeartbeatRequest struct {
	AgentID       string `json:"agent_id"`
	SecretKey     string `json:"secret_key"`
	Status        string `json:"status"`
	ClientVersion string `json:"client_version"`
	OSType        string `json:"os_type"`
}

type RegisterResponse struct {
	Agent   Agent `json:"agent"`
	Created bool  `json:"created"`
}

type ConfigResponse struct {
	Config struct {
		Agent        Agent                    `json:"agent"`
		Tunnels      []Tunnel                 `json:"tunnels"`
		DomainRoutes map[string][]DomainRoute `json:"domain_routes"`
	} `json:"config"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *Client) Register(ctx context.Context, req RegisterRequest) (*RegisterResponse, error) {
	var resp RegisterResponse
	if err := c.postJSON(ctx, "/api/v1/agents/register", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) LoadConfig(ctx context.Context, req HeartbeatRequest) (*ConfigResponse, error) {
	var resp ConfigResponse
	if err := c.postJSON(ctx, "/api/v1/agents/config", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) postJSON(ctx context.Context, path string, reqBody any, out any) error {
	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request %s: %w", path, err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response %s: %w", path, err)
	}

	if resp.StatusCode >= 400 {
		var apiErr errorResponse
		if json.Unmarshal(responseBody, &apiErr) == nil && apiErr.Error != "" {
			return fmt.Errorf("%s returned %d: %s", path, resp.StatusCode, apiErr.Error)
		}
		return fmt.Errorf("%s returned %d", path, resp.StatusCode)
	}

	if err := json.Unmarshal(responseBody, out); err != nil {
		return fmt.Errorf("decode response %s: %w", path, err)
	}
	return nil
}

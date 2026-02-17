package arango

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// Client is a lightweight ArangoDB HTTP client using stdlib only.
type Client struct {
	baseURL  string
	database string
	username string
	password string
	client   *http.Client
}

// NewClient creates a new ArangoDB client for the given database.
// Reads ARANGO_PASSWORD from env. Defaults to localhost:8529, root user.
func NewClient(database string) (*Client, error) {
	if database == "" {
		return nil, fmt.Errorf("database name is required")
	}

	baseURL := os.Getenv("ARANGO_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8529"
	}

	username := os.Getenv("ARANGO_USER")
	if username == "" {
		username = "root"
	}

	password := os.Getenv("ARANGO_PASSWORD")

	return &Client{
		baseURL:  baseURL,
		database: database,
		username: username,
		password: password,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}, nil
}

// cursorRequest is the body for /_api/cursor.
type cursorRequest struct {
	Query    string         `json:"query"`
	BindVars map[string]any `json:"bindVars,omitempty"`
}

// cursorResponse is the response from /_api/cursor.
type cursorResponse struct {
	Result  []json.RawMessage `json:"result"`
	HasMore bool              `json:"hasMore"`
	ID      string            `json:"id"`
	Error   bool              `json:"error"`
	Code    int               `json:"code"`
	Message string            `json:"errorMessage"`
}

// Query executes an AQL query and returns all results.
// Handles cursor pagination automatically.
func (c *Client) Query(aql string, bindVars map[string]any) ([]json.RawMessage, error) {
	body := cursorRequest{Query: aql, BindVars: bindVars}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal query: %w", err)
	}

	url := fmt.Sprintf("%s/_db/%s/_api/cursor", c.baseURL, c.database)
	var allResults []json.RawMessage

	// First request
	resp, err := c.doRequest("POST", url, data)
	if err != nil {
		return nil, err
	}

	var cursor cursorResponse
	if err := json.Unmarshal(resp, &cursor); err != nil {
		return nil, fmt.Errorf("unmarshal cursor response: %w", err)
	}
	if cursor.Error {
		return nil, fmt.Errorf("aql error %d: %s", cursor.Code, cursor.Message)
	}

	allResults = append(allResults, cursor.Result...)

	// Follow cursor pages
	for cursor.HasMore && cursor.ID != "" {
		nextURL := fmt.Sprintf("%s/_db/%s/_api/cursor/%s", c.baseURL, c.database, cursor.ID)
		resp, err = c.doRequest("PUT", nextURL, nil)
		if err != nil {
			return allResults, fmt.Errorf("cursor follow: %w", err)
		}

		cursor = cursorResponse{}
		if err := json.Unmarshal(resp, &cursor); err != nil {
			return allResults, fmt.Errorf("unmarshal cursor page: %w", err)
		}
		if cursor.Error {
			return allResults, fmt.Errorf("cursor page error %d: %s", cursor.Code, cursor.Message)
		}

		allResults = append(allResults, cursor.Result...)
	}

	return allResults, nil
}

// Ping tests connectivity to the database.
func (c *Client) Ping() error {
	url := fmt.Sprintf("%s/_db/%s/_api/version", c.baseURL, c.database)
	_, err := c.doRequest("GET", url, nil)
	return err
}

// Database returns the configured database name.
func (c *Client) Database() string {
	return c.database
}

// UpdateDocument performs a partial update (PATCH) on a document.
// Only the fields in the provided map are updated; existing fields are preserved.
func (c *Client) UpdateDocument(collection, key string, fields map[string]any) error {
	data, err := json.Marshal(fields)
	if err != nil {
		return fmt.Errorf("marshal update: %w", err)
	}

	url := fmt.Sprintf("%s/_db/%s/_api/document/%s/%s", c.baseURL, c.database, collection, key)
	_, err = c.doRequest("PATCH", url, data)
	return err
}

// doRequest executes an HTTP request with auth.
func (c *Client) doRequest(method, url string, body []byte) ([]byte, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.SetBasicAuth(c.username, c.password)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request %s %s: %w", method, url, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		// Try to extract ArangoDB error message
		var errResp struct {
			ErrorMessage string `json:"errorMessage"`
			Code         int    `json:"code"`
		}
		if json.Unmarshal(respBody, &errResp) == nil && errResp.ErrorMessage != "" {
			return nil, fmt.Errorf("arango error %d: %s", errResp.Code, errResp.ErrorMessage)
		}
		return nil, fmt.Errorf("http %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

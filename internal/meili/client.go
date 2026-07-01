package meili

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"msmgr/internal/config"
)

type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

type HTTPError struct {
	Method     string
	Path       string
	StatusCode int
	Status     string
	Body       string
}

func (e *HTTPError) Error() string {
	if strings.TrimSpace(e.Body) == "" {
		return fmt.Sprintf("%s %s: unexpected status %s", e.Method, e.Path, e.Status)
	}
	return fmt.Sprintf("%s %s: unexpected status %s: %s", e.Method, e.Path, e.Status, e.Body)
}

type Health struct {
	Status string `json:"status"`
}

type Index struct {
	UID        string `json:"uid"`
	PrimaryKey string `json:"primaryKey"`
	CreatedAt  string `json:"createdAt"`
	UpdatedAt  string `json:"updatedAt"`
}

type Document map[string]any

type indexListResponse struct {
	Results []Index `json:"results"`
}

type documentListResponse struct {
	Results []Document `json:"results"`
}

type SearchResult struct {
	Hits               []Document `json:"hits"`
	EstimatedTotalHits int        `json:"estimatedTotalHits"`
}

type Task struct {
	TaskUID  int    `json:"taskUid"`
	IndexUID string `json:"indexUid"`
	Status   string `json:"status"`
	Type     string `json:"type"`
	Error    any    `json:"error"`
}

type createIndexRequest struct {
	UID        string `json:"uid"`
	PrimaryKey string `json:"primaryKey,omitempty"`
}

type searchRequest struct {
	Query string `json:"q"`
	Limit int    `json:"limit"`
}

type addDocumentsRequest []Document

func NewClient(cfg config.Config, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &Client{
		baseURL:    strings.TrimRight(cfg.HTTPAddr, "/"),
		apiKey:     cfg.APIKey,
		httpClient: httpClient,
	}
}

func (c *Client) Health(ctx context.Context) (Health, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/health", nil)
	if err != nil {
		return Health{}, err
	}

	var health Health
	if err := c.do(req, &health); err != nil {
		return Health{}, err
	}

	return health, nil
}

func (c *Client) ListIndexes(ctx context.Context) ([]Index, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/indexes", nil)
	if err != nil {
		return nil, err
	}

	var response indexListResponse
	if err := c.do(req, &response); err != nil {
		return nil, err
	}

	return response.Results, nil
}

func (c *Client) GetIndex(ctx context.Context, uid string) (Index, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/indexes/"+url.PathEscape(uid), nil)
	if err != nil {
		return Index{}, err
	}

	var index Index
	if err := c.do(req, &index); err != nil {
		return Index{}, err
	}

	return index, nil
}

func (c *Client) ListDocuments(ctx context.Context, indexUID string, limit, offset int) ([]Document, error) {
	query := url.Values{}
	query.Set("limit", strconv.Itoa(limit))
	query.Set("offset", strconv.Itoa(offset))

	path := fmt.Sprintf("/indexes/%s/documents?%s", url.PathEscape(indexUID), query.Encode())

	req, err := c.newRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	var response documentListResponse
	if err := c.do(req, &response); err != nil {
		return nil, err
	}

	return response.Results, nil
}

func (c *Client) GetDocument(ctx context.Context, indexUID, documentID string) (Document, error) {
	path := fmt.Sprintf("/indexes/%s/documents/%s", url.PathEscape(indexUID), url.PathEscape(documentID))

	req, err := c.newRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	var document Document
	if err := c.do(req, &document); err != nil {
		return nil, err
	}

	return document, nil
}

func (c *Client) SearchIndex(ctx context.Context, indexUID, query string, limit int) (SearchResult, error) {
	payload, err := json.Marshal(searchRequest{
		Query: query,
		Limit: limit,
	})
	if err != nil {
		return SearchResult{}, fmt.Errorf("encode search request: %w", err)
	}

	path := fmt.Sprintf("/indexes/%s/search", url.PathEscape(indexUID))

	req, err := c.newRequest(ctx, http.MethodPost, path, bytes.NewReader(payload))
	if err != nil {
		return SearchResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	var result SearchResult
	if err := c.do(req, &result); err != nil {
		return SearchResult{}, err
	}

	return result, nil
}

func (c *Client) CreateIndex(ctx context.Context, uid, primaryKey string) (Task, error) {
	payload, err := json.Marshal(createIndexRequest{
		UID:        uid,
		PrimaryKey: primaryKey,
	})
	if err != nil {
		return Task{}, fmt.Errorf("encode create index request: %w", err)
	}

	req, err := c.newRequest(ctx, http.MethodPost, "/indexes", bytes.NewReader(payload))
	if err != nil {
		return Task{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	var task Task
	if err := c.do(req, &task); err != nil {
		return Task{}, err
	}

	return task, nil
}

func (c *Client) DeleteIndex(ctx context.Context, uid string) (Task, error) {
	req, err := c.newRequest(ctx, http.MethodDelete, "/indexes/"+url.PathEscape(uid), nil)
	if err != nil {
		return Task{}, err
	}

	var task Task
	if err := c.do(req, &task); err != nil {
		return Task{}, err
	}

	return task, nil
}

func (c *Client) DeleteDocument(ctx context.Context, indexUID, documentID string) (Task, error) {
	path := fmt.Sprintf("/indexes/%s/documents/%s", url.PathEscape(indexUID), url.PathEscape(documentID))

	req, err := c.newRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return Task{}, err
	}

	var task Task
	if err := c.do(req, &task); err != nil {
		return Task{}, err
	}

	return task, nil
}

func (c *Client) DeleteAllDocuments(ctx context.Context, indexUID string) (Task, error) {
	path := fmt.Sprintf("/indexes/%s/documents", url.PathEscape(indexUID))

	req, err := c.newRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return Task{}, err
	}

	var task Task
	if err := c.do(req, &task); err != nil {
		return Task{}, err
	}

	return task, nil
}

func (c *Client) AddDocuments(ctx context.Context, indexUID string, documents []Document) (Task, error) {
	payload, err := json.Marshal(addDocumentsRequest(documents))
	if err != nil {
		return Task{}, fmt.Errorf("encode add documents request: %w", err)
	}

	path := fmt.Sprintf("/indexes/%s/documents", url.PathEscape(indexUID))

	req, err := c.newRequest(ctx, http.MethodPost, path, bytes.NewReader(payload))
	if err != nil {
		return Task{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	var task Task
	if err := c.do(req, &task); err != nil {
		return Task{}, err
	}

	return task, nil
}

func (c *Client) GetTask(ctx context.Context, taskUID int) (Task, error) {
	req, err := c.newRequest(ctx, http.MethodGet, fmt.Sprintf("/tasks/%d", taskUID), nil)
	if err != nil {
		return Task{}, err
	}

	var task Task
	if err := c.do(req, &task); err != nil {
		return Task{}, err
	}

	return task, nil
}

func (c *Client) newRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	return req, nil
}

func (c *Client) do(req *http.Request, out any) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", req.Method, req.URL.Path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if readErr != nil {
			return fmt.Errorf("%s %s: unexpected status %s", req.Method, req.URL.Path, resp.Status)
		}

		return &HTTPError{
			Method:     req.Method,
			Path:       req.URL.Path,
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
			Body:       strings.TrimSpace(string(body)),
		}
	}

	if out == nil {
		return nil
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("%s %s: decode response: %w", req.Method, req.URL.Path, err)
	}

	return nil
}

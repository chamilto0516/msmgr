package meili

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"msmgr/internal/config"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestHealth(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		httpClient := &http.Client{
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.Method != http.MethodGet {
					t.Fatalf("unexpected method %s", r.Method)
				}

				if r.URL.Path != "/health" {
					t.Fatalf("unexpected path %s", r.URL.Path)
				}

				if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
					t.Fatalf("unexpected auth header %q", got)
				}

				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader(`{"status":"available"}`)),
					Request:    r,
				}, nil
			}),
		}

		client := NewClient(config.Config{
			HTTPAddr: "http://example.test",
			APIKey:   "test-key",
		}, httpClient)

		health, err := client.Health(context.Background())
		if err != nil {
			t.Fatalf("Health returned error: %v", err)
		}

		if health.Status != "available" {
			t.Fatalf("unexpected health status %q", health.Status)
		}
	})

	t.Run("unexpected status", func(t *testing.T) {
		httpClient := &http.Client{
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusBadGateway,
					Status:     "502 Bad Gateway",
					Body:       io.NopCloser(strings.NewReader("boom")),
					Request:    r,
				}, nil
			}),
		}

		client := NewClient(config.Config{
			HTTPAddr: "http://example.test",
		}, httpClient)

		_, err := client.Health(context.Background())
		if err == nil {
			t.Fatal("expected error")
		}

		if !strings.Contains(err.Error(), "unexpected status 502 Bad Gateway") {
			t.Fatalf("unexpected error %q", err)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		httpClient := &http.Client{
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Body:       io.NopCloser(strings.NewReader("not-json")),
					Request:    r,
				}, nil
			}),
		}

		client := NewClient(config.Config{
			HTTPAddr: "http://example.test",
		}, httpClient)

		_, err := client.Health(context.Background())
		if err == nil {
			t.Fatal("expected error")
		}

		if !strings.Contains(err.Error(), "decode response") {
			t.Fatalf("unexpected error %q", err)
		}
	})

	t.Run("transport error", func(t *testing.T) {
		httpClient := &http.Client{
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				return nil, errors.New("dial failed")
			}),
		}

		client := NewClient(config.Config{
			HTTPAddr: "http://example.test",
		}, httpClient)

		_, err := client.Health(context.Background())
		if err == nil {
			t.Fatal("expected error")
		}

		if !strings.Contains(err.Error(), "dial failed") {
			t.Fatalf("unexpected error %q", err)
		}
	})
}

func TestListIndexes(t *testing.T) {
	httpClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodGet {
				t.Fatalf("unexpected method %s", r.Method)
			}

			if r.URL.Path != "/indexes" {
				t.Fatalf("unexpected path %s", r.URL.Path)
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body: io.NopCloser(strings.NewReader(`{
					"results":[
						{"uid":"books","primaryKey":"id","createdAt":"2026-06-29T11:00:00Z"},
						{"uid":"notes","primaryKey":"","createdAt":"2026-06-29T11:05:00Z"}
					]
				}`)),
				Request: r,
			}, nil
		}),
	}

	client := NewClient(config.Config{HTTPAddr: "http://example.test"}, httpClient)
	indexes, err := client.ListIndexes(context.Background())
	if err != nil {
		t.Fatalf("ListIndexes returned error: %v", err)
	}

	if len(indexes) != 2 {
		t.Fatalf("unexpected index count %d", len(indexes))
	}

	if indexes[0].UID != "books" || indexes[1].UID != "notes" {
		t.Fatalf("unexpected indexes %#v", indexes)
	}
}

func TestGetIndex(t *testing.T) {
	httpClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodGet {
				t.Fatalf("unexpected method %s", r.Method)
			}

			if r.URL.Path != "/indexes/books" {
				t.Fatalf("unexpected path %s", r.URL.Path)
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body: io.NopCloser(strings.NewReader(`{
					"uid":"books",
					"primaryKey":"id",
					"createdAt":"2026-06-29T11:00:00Z",
					"updatedAt":"2026-06-29T11:05:00Z"
				}`)),
				Request: r,
			}, nil
		}),
	}

	client := NewClient(config.Config{HTTPAddr: "http://example.test"}, httpClient)
	index, err := client.GetIndex(context.Background(), "books")
	if err != nil {
		t.Fatalf("GetIndex returned error: %v", err)
	}

	if index.UID != "books" || index.PrimaryKey != "id" || index.UpdatedAt != "2026-06-29T11:05:00Z" {
		t.Fatalf("unexpected index %#v", index)
	}
}

func TestListDocuments(t *testing.T) {
	httpClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodGet {
				t.Fatalf("unexpected method %s", r.Method)
			}

			if r.URL.Path != "/indexes/books/documents" {
				t.Fatalf("unexpected path %s", r.URL.Path)
			}

			if got := r.URL.Query().Get("limit"); got != "20" {
				t.Fatalf("unexpected limit %q", got)
			}

			if got := r.URL.Query().Get("offset"); got != "0" {
				t.Fatalf("unexpected offset %q", got)
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body: io.NopCloser(strings.NewReader(`{
					"results":[
						{"id":1,"title":"Book One"},
						{"id":2,"title":"Book Two"}
					]
				}`)),
				Request: r,
			}, nil
		}),
	}

	client := NewClient(config.Config{HTTPAddr: "http://example.test"}, httpClient)
	documents, err := client.ListDocuments(context.Background(), "books", 20, 0)
	if err != nil {
		t.Fatalf("ListDocuments returned error: %v", err)
	}

	if len(documents) != 2 {
		t.Fatalf("unexpected document count %d", len(documents))
	}

	if documents[0]["title"] != "Book One" || documents[1]["title"] != "Book Two" {
		t.Fatalf("unexpected documents %#v", documents)
	}
}

func TestGetDocument(t *testing.T) {
	httpClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodGet {
				t.Fatalf("unexpected method %s", r.Method)
			}

			if r.URL.Path != "/indexes/books/documents/doc1" {
				t.Fatalf("unexpected path %s", r.URL.Path)
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body: io.NopCloser(strings.NewReader(`{
					"id":"doc1",
					"title":"Book One",
					"content":"hello"
				}`)),
				Request: r,
			}, nil
		}),
	}

	client := NewClient(config.Config{HTTPAddr: "http://example.test"}, httpClient)
	document, err := client.GetDocument(context.Background(), "books", "doc1")
	if err != nil {
		t.Fatalf("GetDocument returned error: %v", err)
	}

	if document["id"] != "doc1" || document["title"] != "Book One" {
		t.Fatalf("unexpected document %#v", document)
	}
}

func TestSearchIndex(t *testing.T) {
	httpClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected method %s", r.Method)
			}

			if r.URL.Path != "/indexes/books/search" {
				t.Fatalf("unexpected path %s", r.URL.Path)
			}

			if got := r.Header.Get("Content-Type"); got != "application/json" {
				t.Fatalf("unexpected content type %q", got)
			}

			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode request body: %v", err)
			}

			if payload["q"] != "history" {
				t.Fatalf("unexpected query %#v", payload["q"])
			}

			if payload["limit"] != float64(5) {
				t.Fatalf("unexpected limit %#v", payload["limit"])
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body: io.NopCloser(strings.NewReader(`{
					"estimatedTotalHits":2,
					"hits":[
						{"id":1,"title":"Book One","content":"abc"},
						{"id":2,"title":"Book Two","content":"def"}
					]
				}`)),
				Request: r,
			}, nil
		}),
	}

	client := NewClient(config.Config{HTTPAddr: "http://example.test"}, httpClient)
	result, err := client.SearchIndex(context.Background(), "books", "history", 5)
	if err != nil {
		t.Fatalf("SearchIndex returned error: %v", err)
	}

	if result.EstimatedTotalHits != 2 || len(result.Hits) != 2 {
		t.Fatalf("unexpected result %#v", result)
	}
}

func TestCreateIndex(t *testing.T) {
	httpClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected method %s", r.Method)
			}

			if r.URL.Path != "/indexes" {
				t.Fatalf("unexpected path %s", r.URL.Path)
			}

			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode request body: %v", err)
			}

			if payload["uid"] != "test" {
				t.Fatalf("unexpected uid %#v", payload["uid"])
			}

			if payload["primaryKey"] != "id" {
				t.Fatalf("unexpected primaryKey %#v", payload["primaryKey"])
			}

			return &http.Response{
				StatusCode: http.StatusCreated,
				Status:     "201 Created",
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body: io.NopCloser(strings.NewReader(`{
					"taskUid":42,
					"indexUid":"test",
					"status":"enqueued",
					"type":"indexCreation"
				}`)),
				Request: r,
			}, nil
		}),
	}

	client := NewClient(config.Config{HTTPAddr: "http://example.test"}, httpClient)
	task, err := client.CreateIndex(context.Background(), "test", "id")
	if err != nil {
		t.Fatalf("CreateIndex returned error: %v", err)
	}

	if task.TaskUID != 42 || task.IndexUID != "test" || task.Status != "enqueued" {
		t.Fatalf("unexpected task %#v", task)
	}
}

func TestDeleteIndex(t *testing.T) {
	httpClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodDelete {
				t.Fatalf("unexpected method %s", r.Method)
			}

			if r.URL.Path != "/indexes/test" {
				t.Fatalf("unexpected path %s", r.URL.Path)
			}

			return &http.Response{
				StatusCode: http.StatusAccepted,
				Status:     "202 Accepted",
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body: io.NopCloser(strings.NewReader(`{
					"taskUid":52,
					"indexUid":"test",
					"status":"enqueued",
					"type":"indexDeletion"
				}`)),
				Request: r,
			}, nil
		}),
	}

	client := NewClient(config.Config{HTTPAddr: "http://example.test"}, httpClient)
	task, err := client.DeleteIndex(context.Background(), "test")
	if err != nil {
		t.Fatalf("DeleteIndex returned error: %v", err)
	}

	if task.TaskUID != 52 || task.IndexUID != "test" || task.Type != "indexDeletion" {
		t.Fatalf("unexpected task %#v", task)
	}
}

func TestDeleteDocument(t *testing.T) {
	httpClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodDelete {
				t.Fatalf("unexpected method %s", r.Method)
			}

			if r.URL.Path != "/indexes/test/documents/doc1" {
				t.Fatalf("unexpected path %s", r.URL.Path)
			}

			return &http.Response{
				StatusCode: http.StatusAccepted,
				Status:     "202 Accepted",
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body: io.NopCloser(strings.NewReader(`{
					"taskUid":61,
					"indexUid":"test",
					"status":"enqueued",
					"type":"documentDeletion"
				}`)),
				Request: r,
			}, nil
		}),
	}

	client := NewClient(config.Config{HTTPAddr: "http://example.test"}, httpClient)
	task, err := client.DeleteDocument(context.Background(), "test", "doc1")
	if err != nil {
		t.Fatalf("DeleteDocument returned error: %v", err)
	}

	if task.TaskUID != 61 || task.IndexUID != "test" || task.Type != "documentDeletion" {
		t.Fatalf("unexpected task %#v", task)
	}
}

func TestDeleteAllDocuments(t *testing.T) {
	httpClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodDelete {
				t.Fatalf("unexpected method %s", r.Method)
			}

			if r.URL.Path != "/indexes/test/documents" {
				t.Fatalf("unexpected path %s", r.URL.Path)
			}

			return &http.Response{
				StatusCode: http.StatusAccepted,
				Status:     "202 Accepted",
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body: io.NopCloser(strings.NewReader(`{
					"taskUid":62,
					"indexUid":"test",
					"status":"enqueued",
					"type":"documentDeletion"
				}`)),
				Request: r,
			}, nil
		}),
	}

	client := NewClient(config.Config{HTTPAddr: "http://example.test"}, httpClient)
	task, err := client.DeleteAllDocuments(context.Background(), "test")
	if err != nil {
		t.Fatalf("DeleteAllDocuments returned error: %v", err)
	}

	if task.TaskUID != 62 || task.IndexUID != "test" || task.Type != "documentDeletion" {
		t.Fatalf("unexpected task %#v", task)
	}
}

func TestAddDocuments(t *testing.T) {
	httpClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected method %s", r.Method)
			}

			if r.URL.Path != "/indexes/test/documents" {
				t.Fatalf("unexpected path %s", r.URL.Path)
			}

			if got := r.Header.Get("Content-Type"); got != "application/json" {
				t.Fatalf("unexpected content type %q", got)
			}

			var payload []map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode request body: %v", err)
			}

			if len(payload) != 1 {
				t.Fatalf("unexpected payload length %d", len(payload))
			}

			if payload[0]["id"] != "doc1" || payload[0]["title"] != "Doc One" {
				t.Fatalf("unexpected payload %#v", payload)
			}

			return &http.Response{
				StatusCode: http.StatusAccepted,
				Status:     "202 Accepted",
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body: io.NopCloser(strings.NewReader(`{
					"taskUid":73,
					"indexUid":"test",
					"status":"enqueued",
					"type":"documentAdditionOrUpdate"
				}`)),
				Request: r,
			}, nil
		}),
	}

	client := NewClient(config.Config{HTTPAddr: "http://example.test"}, httpClient)
	task, err := client.AddDocuments(context.Background(), "test", []Document{
		{"id": "doc1", "title": "Doc One", "path": "test/doc1.md", "content": "hello"},
	})
	if err != nil {
		t.Fatalf("AddDocuments returned error: %v", err)
	}

	if task.TaskUID != 73 || task.IndexUID != "test" || task.Type != "documentAdditionOrUpdate" {
		t.Fatalf("unexpected task %#v", task)
	}
}

func TestGetTask(t *testing.T) {
	httpClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != http.MethodGet {
				t.Fatalf("unexpected method %s", r.Method)
			}

			if r.URL.Path != "/tasks/73" {
				t.Fatalf("unexpected path %s", r.URL.Path)
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body: io.NopCloser(strings.NewReader(`{
					"taskUid":73,
					"indexUid":"test",
					"status":"succeeded",
					"type":"documentAdditionOrUpdate",
					"error":null
				}`)),
				Request: r,
			}, nil
		}),
	}

	client := NewClient(config.Config{HTTPAddr: "http://example.test"}, httpClient)
	task, err := client.GetTask(context.Background(), 73)
	if err != nil {
		t.Fatalf("GetTask returned error: %v", err)
	}

	if task.TaskUID != 73 || task.Status != "succeeded" {
		t.Fatalf("unexpected task %#v", task)
	}
}

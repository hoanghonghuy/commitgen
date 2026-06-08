package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type echoReq struct {
	Name string `json:"name"`
}

type echoResp struct {
	Greeting string `json:"greeting"`
}

func newTestClient() *http.Client {
	return &http.Client{Timeout: 5 * time.Second}
}

func TestDoJSONRequest_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Test") != "yes" {
			t.Errorf("custom header not propagated")
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"greeting":"hello world"}`))
	}))
	defer srv.Close()

	var resp echoResp
	headers := map[string]string{"X-Test": "yes", "Content-Type": "application/json"}
	err := DoJSONRequest(context.Background(), newTestClient(), "POST", srv.URL, headers, echoReq{Name: "x"}, &resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Greeting != "hello world" {
		t.Errorf("got greeting %q", resp.Greeting)
	}
}

func TestDoJSONRequest_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid key"}`))
	}))
	defer srv.Close()

	var resp echoResp
	err := DoJSONRequest(context.Background(), newTestClient(), "POST", srv.URL, nil, echoReq{}, &resp)
	if err == nil {
		t.Fatal("expected error for non-200 status")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error should contain status code, got: %v", err)
	}
}

func TestDoJSONRequest_BadResponseJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	var resp echoResp
	err := DoJSONRequest(context.Background(), newTestClient(), "POST", srv.URL, nil, echoReq{}, &resp)
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Errorf("expected decode error, got: %v", err)
	}
}

func TestDoJSONRequest_UnmarshalableBody(t *testing.T) {
	var resp echoResp
	// channels cannot be marshaled to JSON
	err := DoJSONRequest(context.Background(), newTestClient(), "POST", "http://example.invalid", nil, make(chan int), &resp)
	if err == nil || !strings.Contains(err.Error(), "marshal request") {
		t.Errorf("expected marshal error, got: %v", err)
	}
}

func TestDoJSONRequest_RequestError(t *testing.T) {
	var resp echoResp
	// invalid URL scheme triggers client.Do error
	err := DoJSONRequest(context.Background(), newTestClient(), "POST", "http://127.0.0.1:0", nil, echoReq{}, &resp)
	if err == nil {
		t.Error("expected error for unreachable endpoint")
	}
}

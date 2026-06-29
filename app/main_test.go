package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestMatchPath(t *testing.T) {

	tests := []struct {
		pattern    string
		path       string
		wantOk     bool
		wantParams map[string]string
	}{
		{
			pattern:    "/echo/{str}",
			path:       "/echo/hello",
			wantOk:     true,
			wantParams: map[string]string{"str": "hello"},
		},
		{
			pattern:    "/echo/{str}",
			path:       "/echo/",
			wantOk:     true,
			wantParams: map[string]string{"str": ""},
		},
		{
			pattern:    "/echo/{str}",
			path:       "/echo/hello/world",
			wantOk:     false,
			wantParams: nil,
		},
		{
			pattern:    "/echo/{str}",
			path:       "/files/hello",
			wantOk:     false,
			wantParams: nil,
		},
		{
			pattern:    "/user-agent",
			path:       "/user-agent",
			wantOk:     true,
			wantParams: map[string]string{},
		},
		{
			pattern:    "/users/{id}/posts/{postId}",
			path:       "/users/42/posts/7",
			wantOk:     true,
			wantParams: map[string]string{"id": "42", "postId": "7"},
		},
		{
			pattern:    "/",
			path:       "/",
			wantOk:     true,
			wantParams: map[string]string{},
		},
	}

	for _, tt := range tests {
		params, ok := matchPath(tt.pattern, tt.path)

		if ok != tt.wantOk {
			t.Errorf("pattern=%s path=%s: expected ok=%v got ok=%v", tt.pattern, tt.path, tt.wantOk, ok)
		}

		if !reflect.DeepEqual(params, tt.wantParams) {
			t.Errorf("pattern=%s path=%s: expected params=%v got params=%v", tt.pattern, tt.path, tt.wantParams, params)
		}
	}
}

func TestRequestParse(t *testing.T) {
	raw := []byte("GET /echo/hello HTTP/1.1\r\nUser-Agent: curl/8.7.1\r\nHost: localhost\r\n\r\n")
	req := requestParse(raw)

	if req.Method != "GET" {
		t.Errorf("expected method=GET got %s", req.Method)
	}
	if req.Path != "/echo/hello" {
		t.Errorf("expected path=/echo/hello got %s", req.Path)
	}
	if req.Headers["User-Agent"] != "curl/8.7.1" {
		t.Errorf("expected User-Agent=curl/8.7.1 got %s", req.Headers["User-Agent"])
	}
}

func TestResponseBytes(t *testing.T) {
	res := TextResponse(200, "hello")
	bytes := res.Bytes()
	output := string(bytes)

	if !strings.Contains(output, "HTTP/1.1 200 OK") {
		t.Errorf("expected status line, got %s", output)
	}
	if !strings.Contains(output, "Content-Length: 5") {
		t.Errorf("expected Content-Length: 5, got %s", output)
	}
	if !strings.Contains(output, "hello") {
		t.Errorf("expected body=hello, got %s", output)
	}
}

func TestHandleEcho(t *testing.T) {
	ctx := &Context{
		Req:    Request{Method: "GET", Path: "/echo/hello"},
		Params: map[string]string{"str": "hello"},
	}
	res, err := handleEcho(ctx)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if res.StatusCode != 200 {
		t.Errorf("expected 200, got %d", res.StatusCode)
	}
	if string(res.Body) != "hello" {
		t.Errorf("expected body=hello, got %s", res.Body)
	}
}

func TestHandleRoot(t *testing.T) {
	ctx := &Context{Req: Request{Method: "GET", Path: "/"}}
	res, err := handleRoot(ctx)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if res.StatusCode != 200 {
		t.Errorf("expected 200, got %d", res.StatusCode)
	}
}

func TestHandleFileGetMissingFile(t *testing.T) {
	app := &App{FileDirectory: "/nonexistent/"}
	ctx := &Context{
		Req:    Request{Method: "GET", Path: "/files/test.txt"},
		Params: map[string]string{"filename": "test.txt"},
	}
	_, err := app.handleFileGet(ctx)

	if err == nil {
		t.Errorf("expected error for missing file, got nil")
	}
}

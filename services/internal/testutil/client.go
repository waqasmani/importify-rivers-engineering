package testutil

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
)

// Client is an HTTP client bound to a test server that remembers cookies and the CSRF token.
type Client struct {
	T       testing.TB
	Base    string
	HTTP    *http.Client
	CSRF    string
	Bearer  string
	Headers map[string]string
	// Validate, when set, checks every response (for example against the OpenAPI document).
	Validate func(req *http.Request, res *Response)
}

// NewClient creates a client for srv.
func NewClient(t testing.TB, srv *httptest.Server) *Client {
	jar, _ := cookiejar.New(nil)
	return &Client{T: t, Base: srv.URL, HTTP: &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}, Headers: map[string]string{}}
}

// Response is a decoded API response.
type Response struct {
	Status int
	Header http.Header
	Body   []byte
	JSON   map[string]any
}

// Do sends a JSON request. body may be nil, a string (sent raw) or any JSON-encodable value.
func (c *Client) Do(method, path string, body any) *Response {
	c.T.Helper()
	var rdr io.Reader
	if body != nil {
		switch b := body.(type) {
		case string:
			rdr = strings.NewReader(b)
		default:
			raw, err := json.Marshal(b)
			if err != nil {
				c.T.Fatalf("marshal: %v", err)
			}
			rdr = bytes.NewReader(raw)
		}
	}
	req, err := http.NewRequest(method, c.Base+path, rdr)
	if err != nil {
		c.T.Fatalf("request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.send(req)
}

// Upload sends a multipart form with one file field and extra fields.
func (c *Client) Upload(path, field, filename string, content []byte, fields map[string]string) *Response {
	c.T.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	fw, _ := mw.CreateFormFile(field, filename)
	_, _ = fw.Write(content)
	_ = mw.Close()
	req, _ := http.NewRequest(http.MethodPost, c.Base+path, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return c.send(req)
}

func (c *Client) send(req *http.Request) *Response {
	c.T.Helper()
	if c.CSRF != "" {
		req.Header.Set("X-CSRF-Token", c.CSRF)
	}
	if c.Bearer != "" {
		req.Header.Set("Authorization", "Bearer "+c.Bearer)
	}
	for k, v := range c.Headers {
		req.Header.Set(k, v)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		c.T.Fatalf("%s %s: %v", req.Method, req.URL.Path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	out := &Response{Status: resp.StatusCode, Header: resp.Header, Body: raw}
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
		_ = json.Unmarshal(raw, &out.JSON)
	}
	if c.Validate != nil {
		c.Validate(req, out)
	}
	return out
}

// Must asserts the status code and returns the response.
func (r *Response) Must(t testing.TB, status int) *Response {
	t.Helper()
	if r.Status != status {
		t.Fatalf("expected status %d, got %d: %s", status, r.Status, string(r.Body))
	}
	return r
}

// ErrorCode returns error.code from an error response.
func (r *Response) ErrorCode() string {
	if e, ok := r.JSON["error"].(map[string]any); ok {
		s, _ := e["code"].(string)
		return s
	}
	return ""
}

// Str returns a string field by dotted path.
func (r *Response) Str(path string) string {
	v := r.Get(path)
	s, _ := v.(string)
	return s
}

// Get returns a value by dotted path ("user.id", "items.0.name").
func (r *Response) Get(path string) any {
	var cur any = r.JSON
	for _, part := range strings.Split(path, ".") {
		switch node := cur.(type) {
		case map[string]any:
			cur = node[part]
		case []any:
			idx := 0
			for _, ch := range part {
				idx = idx*10 + int(ch-'0')
			}
			if idx >= len(node) {
				return nil
			}
			cur = node[idx]
		default:
			return nil
		}
	}
	return cur
}

// Items returns the "items" array.
func (r *Response) Items() []map[string]any {
	arr, _ := r.JSON["items"].([]any)
	out := make([]map[string]any, 0, len(arr))
	for _, it := range arr {
		if m, ok := it.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

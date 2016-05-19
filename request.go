package httpexpect

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Request provides methods to incrementally build http.Request object,
// send it, and receive response.
type Request struct {
	config Config
	chain  chain
	http   http.Request
	query  url.Values
}

// NewRequest returns a new Request object.
//
// method specifies the HTTP method (GET, POST, PUT, etc.).
// urlfmt and args are passed to fmt.Sprintf(), with url as format string.
//
// If Config.BaseURL is non-empty, it is prepended to final url,
// separated by slash.
//
// Example:
//  req := NewRequest(config, "PUT", "http://example.org/path")
func NewRequest(config Config, method, urlfmt string, args ...interface{}) *Request {
	chain := makeChain(config.Reporter)

	for _, a := range args {
		if a == nil {
			chain.fail(
				"\nunexpected nil argument for url format string:\n"+
					"  Request(\"%s\", %v...)", method, args)
		}
	}

	us := concatURLs(config.BaseURL, fmt.Sprintf(urlfmt, args...))

	u, err := url.Parse(us)
	if err != nil {
		chain.fail(err.Error())
	}

	req := Request{
		config: config,
		chain:  chain,
		http: http.Request{
			Method: method,
			URL:    u,
			Header: make(http.Header),
		},
	}

	return &req
}

func concatURLs(a, b string) string {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	if strings.HasSuffix(a, "/") {
		a = a[:len(a)-1]
	}
	if strings.HasPrefix(b, "/") {
		b = b[1:]
	}
	return a + "/" + b
}

// WithQuery adds query parameter to request URL.
//
// value is converted to string using fmt.Sprint() and urlencoded.
//
// Example:
//  req := NewRequest(config, "PUT", "http://example.org/path")
//  req.WithQuery("foo", 123)
//  req.WithQuery("bar", "baz")
//  // URL is now http://example.org/path?foo=123&bar=baz
func (r *Request) WithQuery(key string, value interface{}) *Request {
	if r.query == nil {
		r.query = r.http.URL.Query()
	}
	r.query.Add(key, fmt.Sprint(value))
	return r
}

// WithHeaders adds given headers to request.
//
// Example:
//  req := NewRequest(config, "PUT", "http://example.org/path")
//  req.WithHeaders(map[string]string{
//      "Content-Type": "application/json",
//  })
func (r *Request) WithHeaders(headers map[string]string) *Request {
	for k, v := range headers {
		r.http.Header.Add(k, v)
	}
	return r
}

// WithHeader adds given single header to request.
//
// Example:
//  req := NewRequest(config, "PUT", "http://example.org/path")
//  req.WithHeader("Content-Type": "application/json")
func (r *Request) WithHeader(k, v string) *Request {
	r.http.Header.Add(k, v)
	return r
}

// WithBody set given reader for request body.
//
// Expect() will read all available data from this reader.
//
// Example:
//  req := NewRequest(config, "PUT", "http://example.org/path")
//  req.WithHeader("Content-Type": "application/json")
//  req.WithBody(bytes.NewBufferString(`{"foo": 123}`))
func (r *Request) WithBody(reader io.Reader) *Request {
	if reader == nil {
		r.http.Body = nil
		r.http.ContentLength = 0
	} else {
		r.http.Body = readCloserAdapter{reader}
		r.http.ContentLength = -1
	}
	return r
}

// WithBytes is like WithBody, but gets body as a slice of bytes.
//
// Example:
//  req := NewRequest(config, "PUT", "http://example.org/path")
//  req.WithHeader("Content-Type": "application/json")
//  req.WithBytes([]byte(`{"foo": 123}`))
func (r *Request) WithBytes(b []byte) *Request {
	if b == nil {
		r.http.Body = nil
		r.http.ContentLength = 0
	} else {
		r.http.Body = readCloserAdapter{bytes.NewReader(b)}
		r.http.ContentLength = int64(len(b))
	}
	return r
}

// WithJSON sets Content-Type header to "application/json" and sets body to
// marshaled object.
//
// Example:
//  req := NewRequest(config, "PUT", "http://example.org/path")
//  req.WithJSON(map[string]interface{}{"foo": 123})
func (r *Request) WithJSON(object interface{}) *Request {
	b, err := json.Marshal(object)
	if err != nil {
		r.chain.fail(err.Error())
		return r
	}

	r.WithHeader("Content-Type", "application/json; charset=utf-8")
	r.WithBytes(b)

	return r
}

// Expect constructs http.Request, sends it, receives http.Response, and
// returns a new Response object to inspect received response.
//
// Request is sent using Config.Client interface.
//
// Example:
//  req := NewRequest(config, "PUT", "http://example.org/path")
//  req.WithJSON(map[string]interface{}{"foo": 123})
//  resp := req.Expect()
//  resp.Status(http.StatusOK)
func (r *Request) Expect() *Response {
	resp := r.sendRequest()
	return &Response{
		chain: r.chain,
		resp:  resp,
	}
}

func (r *Request) sendRequest() *http.Response {
	if r.chain.failed() {
		return nil
	}

	if r.query != nil {
		r.http.URL.RawQuery = r.query.Encode()
	}

	if r.config.Printer != nil {
		r.config.Printer.Request(&r.http)
	}

	resp, err := r.config.Client.Do(&r.http)
	if err != nil {
		r.chain.fail(err.Error())
		return nil
	}

	if r.config.Printer != nil {
		r.config.Printer.Response(resp)
	}

	return resp
}

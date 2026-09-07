package leetcode

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

// site holds the per-region endpoints.
type site struct {
	base   string
	origin string
}

var sites = map[string]site{
	"com": {base: "https://leetcode.com", origin: "https://leetcode.com"},
	"cn":  {base: "https://leetcode.cn", origin: "https://leetcode.cn"},
}

// DefaultUserAgent identifies lazyleet to LeetCode. A real browser-like UA
// avoids some bot filtering while still being honest about the tool.
const DefaultUserAgent = "lazyleet/0.x (+https://github.com/sven97/lazyleet)"

// Client talks to one LeetCode deployment. It is safe for concurrent use.
type Client struct {
	http         *http.Client
	site         site
	region       string
	creds        Credentials
	ua           string
	limiter      *rate.Limiter
	maxRetry     int
	retryBackoff time.Duration
}

// Option configures a Client.
type Option func(*Client)

// WithCredentials attaches auth cookies for authenticated requests.
func WithCredentials(c Credentials) Option { return func(cl *Client) { cl.creds = c } }

// WithHTTPClient overrides the underlying *http.Client (tests inject one that
// points at an httptest server).
func WithHTTPClient(h *http.Client) Option { return func(cl *Client) { cl.http = h } }

// WithUserAgent overrides the User-Agent header.
func WithUserAgent(ua string) Option { return func(cl *Client) { cl.ua = ua } }

// WithRateLimit overrides the default request rate (2/s, burst 1).
func WithRateLimit(r rate.Limit, burst int) Option {
	return func(cl *Client) { cl.limiter = rate.NewLimiter(r, burst) }
}

// WithBaseURL points the client at an arbitrary base (tests). It also sets the
// Origin/Referer host to match.
func WithBaseURL(base string) Option {
	return func(cl *Client) {
		cl.site = site{base: strings.TrimRight(base, "/"), origin: strings.TrimRight(base, "/")}
	}
}

// WithRetryBackoff sets the per-attempt backoff step (default 750ms). Tests set
// this low.
func WithRetryBackoff(d time.Duration) Option {
	return func(cl *Client) { cl.retryBackoff = d }
}

// New builds a Client for the given region ("com" or "cn"; anything else is
// treated as "com").
func New(region string, opts ...Option) *Client {
	s, ok := sites[region]
	if !ok {
		region, s = "com", sites["com"]
	}
	cl := &Client{
		http:         &http.Client{Timeout: 30 * time.Second},
		site:         s,
		region:       region,
		ua:           DefaultUserAgent,
		limiter:      rate.NewLimiter(2, 1),
		maxRetry:     3,
		retryBackoff: 750 * time.Millisecond,
	}
	for _, o := range opts {
		o(cl)
	}
	return cl
}

// Authenticated reports whether the client has credentials.
func (c *Client) Authenticated() bool { return !c.creds.Anonymous() }

// Region returns the configured region.
func (c *Client) Region() string { return c.region }

// --- GraphQL ---------------------------------------------------------------

type gqlRequest struct {
	OperationName string         `json:"operationName,omitempty"`
	Query         string         `json:"query"`
	Variables     map[string]any `json:"variables,omitempty"`
}

type gqlEnvelope struct {
	Data   json.RawMessage `json:"data"`
	Errors []gqlError      `json:"errors"`
}

type gqlError struct {
	Message string `json:"message"`
}

// APIError is returned when LeetCode responds with a non-2xx status or GraphQL
// errors. Status is 0 for GraphQL-level errors.
type APIError struct {
	Op      string
	Status  int
	Message string
}

func (e *APIError) Error() string {
	if e.Status != 0 {
		return fmt.Sprintf("leetcode %s: HTTP %d: %s", e.Op, e.Status, e.Message)
	}
	return fmt.Sprintf("leetcode %s: %s", e.Op, e.Message)
}

// graphql executes a query and unmarshals the "data" object into out.
func (c *Client) graphql(ctx context.Context, op, query string, vars map[string]any, out any) error {
	body, err := json.Marshal(gqlRequest{OperationName: op, Query: query, Variables: vars})
	if err != nil {
		return err
	}

	resp, raw, err := c.doWithRetry(ctx, op, c.site.base+"/graphql/", body, c.site.base+"/")
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{Op: op, Status: resp.StatusCode, Message: snippet(raw)}
	}

	var env gqlEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return &APIError{Op: op, Message: fmt.Sprintf("unreadable response: %v", err)}
	}
	if len(env.Errors) > 0 {
		msgs := make([]string, len(env.Errors))
		for i, e := range env.Errors {
			msgs[i] = e.Message
		}
		return &APIError{Op: op, Message: strings.Join(msgs, "; ")}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(env.Data, out); err != nil {
		return &APIError{Op: op, Message: fmt.Sprintf("decode data: %v", err)}
	}
	return nil
}

// postJSON executes a REST call (used by interpret/submit). referer must be the
// problem page URL — LeetCode rejects these calls otherwise.
func (c *Client) postJSON(ctx context.Context, op, path string, payload, out any, referer string) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	resp, raw, err := c.doWithRetry(ctx, op, c.site.base+path, body, referer)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{Op: op, Status: resp.StatusCode, Message: snippet(raw)}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return &APIError{Op: op, Message: fmt.Sprintf("decode: %v", err)}
	}
	return nil
}

// get executes a plain GET (used for polling submission results).
func (c *Client) get(ctx context.Context, op, path string, out any, referer string) error {
	resp, raw, err := c.doWithRetry(ctx, op, c.site.base+path, nil, referer)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{Op: op, Status: resp.StatusCode, Message: snippet(raw)}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return &APIError{Op: op, Message: fmt.Sprintf("decode: %v", err)}
	}
	return nil
}

// doWithRetry sends one request, retrying on 429 and 5xx with linear backoff.
// A nil body means GET; a non-nil body means POST JSON.
func (c *Client) doWithRetry(ctx context.Context, op, url string, body []byte, referer string) (*http.Response, []byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.maxRetry; attempt++ {
		if err := c.limiter.Wait(ctx); err != nil {
			return nil, nil, err
		}
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * c.retryBackoff):
			}
		}

		method := http.MethodGet
		var rdr io.Reader
		if body != nil {
			method, rdr = http.MethodPost, bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, url, rdr)
		if err != nil {
			return nil, nil, err
		}
		c.setHeaders(req, body != nil, referer)

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		raw, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			lastErr = &APIError{Op: op, Status: resp.StatusCode, Message: snippet(raw)}
			continue
		}
		return resp, raw, nil
	}
	return nil, nil, lastErr
}

func (c *Client) setHeaders(req *http.Request, isPost bool, referer string) {
	req.Header.Set("User-Agent", c.ua)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Origin", c.site.origin)
	if referer == "" {
		referer = c.site.base + "/"
	}
	req.Header.Set("Referer", referer)
	if isPost {
		req.Header.Set("Content-Type", "application/json")
	}
	if ck := c.creds.cookieHeader(); ck != "" {
		req.Header.Set("Cookie", ck)
		req.Header.Set("X-Csrftoken", c.creds.CSRFToken)
	}
}

func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 300 {
		s = s[:300] + "…"
	}
	if s == "" {
		s = "(empty body)"
	}
	return s
}

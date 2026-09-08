// Package browserlogin logs in to a site by launching an ordinary
// Chromium-family browser with a DevTools port open, letting the user sign in
// (Cloudflare/captcha included), and then reading the resulting cookies over
// CDP — no OS keychain, no cookie-file decryption.
//
// The browser is launched as a plain subprocess with NO automation flags and
// NO CDP client attached, so bot detection (Cloudflare Turnstile) sees a
// normal browser during the challenge. lazyleet connects only afterwards, just
// to read the cookies.
//
// It is written as a small reusable "browser layer": later features
// (Cloudflare-challenge fallback for API calls, editorial scraping, silent
// session refresh) can build on the same launched browser + CDP connection.
package browserlogin

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/storage"
	"github.com/chromedp/chromedp"
)

// Options configures Capture.
type Options struct {
	LoginURL     string        // page to open, e.g. https://leetcode.com/accounts/login/
	CookieDomain string        // return cookies whose domain contains this, e.g. "leetcode.com"
	WaitFor      []string      // cookie names that must all have non-empty values
	Timeout      time.Duration // max wait for login; 0 -> 5 minutes
	ProfileDir   string        // user-data-dir; must be set and not the browser's default
	ExecPath     string        // explicit browser binary; "" -> auto-discover
	Poll         time.Duration // cookie poll interval; 0 -> 1s
	OnStatus     func(string)  // optional progress callback
}

// Capture launches a browser at o.LoginURL and blocks until every cookie in
// o.WaitFor has a value for o.CookieDomain (success), the deadline passes, or
// the user closes the browser.
func Capture(ctx context.Context, o Options) (map[string]string, error) {
	if o.CookieDomain == "" || len(o.WaitFor) == 0 {
		return nil, fmt.Errorf("browserlogin: CookieDomain and WaitFor are required")
	}
	if o.ProfileDir == "" {
		return nil, fmt.Errorf("browserlogin: ProfileDir is required")
	}
	timeout := o.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	poll := o.Poll
	if poll <= 0 {
		poll = time.Second
	}
	status := o.OnStatus
	if status == nil {
		status = func(string) {}
	}

	bin, err := findBrowser(o.ExecPath)
	if err != nil {
		return nil, err
	}

	port, err := freePort()
	if err != nil {
		return nil, err
	}

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Plain launch. No --enable-automation, no CDP client yet.
	args := []string{
		"--remote-debugging-port=" + fmt.Sprint(port),
		// Chrome >=111 rejects CDP websocket connects from non-null origins
		// unless this is set. We only connect from localhost, after login.
		"--remote-allow-origins=*",
		"--user-data-dir=" + o.ProfileDir,
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-blink-features=AutomationControlled",
		"--new-window",
		o.LoginURL,
	}
	proc := exec.CommandContext(runCtx, bin, args...)
	if err := proc.Start(); err != nil {
		return nil, fmt.Errorf("could not start %s: %w", bin, err)
	}
	closed := make(chan struct{})
	go func() { _ = proc.Wait(); close(closed) }()
	defer func() {
		if proc.Process != nil {
			_ = proc.Process.Kill()
		}
	}()

	wsURL, err := waitForDevTools(runCtx, port)
	if err != nil {
		return nil, err
	}
	status("browser open — sign in to LeetCode, then return here")

	allocCtx, cancelAlloc := chromedp.NewRemoteAllocator(runCtx, wsURL)
	defer cancelAlloc()
	cdpCtx, cancelCDP := chromedp.NewContext(allocCtx)
	defer cancelCDP()

	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	for {
		if vals, err := readCookies(cdpCtx, o.CookieDomain, o.WaitFor); err == nil && HaveAll(vals, o.WaitFor) {
			status("captured session")
			return vals, nil
		}
		select {
		case <-closed:
			return nil, fmt.Errorf("browser was closed before sign-in completed")
		case <-runCtx.Done():
			return nil, fmt.Errorf("timed out after %s waiting for a LeetCode sign-in", timeout)
		case <-ticker.C:
		}
	}
}

func readCookies(ctx context.Context, domain string, want []string) (map[string]string, error) {
	var cookies []*network.Cookie
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var e error
		// Storage.getCookies returns every cookie in the browser, independent of
		// which tab (if any) this CDP context is attached to.
		cookies, e = storage.GetCookies().Do(ctx)
		return e
	}))
	if err != nil {
		return nil, err
	}
	wanted := make(map[string]bool, len(want))
	for _, n := range want {
		wanted[n] = true
	}
	out := map[string]string{}
	for _, c := range cookies {
		if !strings.Contains(c.Domain, domain) {
			continue
		}
		if len(wanted) > 0 && !wanted[c.Name] {
			continue
		}
		if c.Value != "" {
			out[c.Name] = c.Value
		}
	}
	return out, nil
}

// HaveAll reports whether vals has a non-empty entry for every name in want.
func HaveAll(vals map[string]string, want []string) bool {
	for _, n := range want {
		if vals[n] == "" {
			return false
		}
	}
	return len(want) > 0
}

// waitForDevTools polls the browser's HTTP debugging endpoint until it reports
// its websocket URL.
func waitForDevTools(ctx context.Context, port int) (string, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d/json/version", port)
	deadline := time.Now().Add(20 * time.Second)
	for {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			var body struct {
				WS string `json:"webSocketDebuggerUrl"`
			}
			dec := json.NewDecoder(resp.Body)
			_ = dec.Decode(&body)
			resp.Body.Close()
			if body.WS != "" {
				return body.WS, nil
			}
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("browser did not expose a DevTools endpoint on port %d", port)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// findBrowser locates a Chromium-family browser binary.
func findBrowser(explicit string) (string, error) {
	if explicit != "" {
		if _, err := exec.LookPath(explicit); err != nil {
			return "", fmt.Errorf("--browser-path %q: %w", explicit, err)
		}
		return explicit, nil
	}
	for _, c := range browserCandidates() {
		if p, err := exec.LookPath(c); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("no Chromium-family browser found (Chrome, Chromium, Edge, Brave) — " +
		"install one, pass --browser-path, or use `lazyleet auth` or `lazyleet auth --manual`")
}

func browserCandidates() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
			"/Applications/Arc.app/Contents/MacOS/Arc",
			"/Applications/Vivaldi.app/Contents/MacOS/Vivaldi",
			"google-chrome", "chromium",
		}
	case "windows":
		return []string{
			`C:\Program Files\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
			"chrome.exe", "msedge.exe",
		}
	default:
		return []string{
			"google-chrome", "google-chrome-stable", "chromium", "chromium-browser",
			"microsoft-edge", "microsoft-edge-stable", "brave-browser", "vivaldi-stable",
		}
	}
}

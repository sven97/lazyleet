// Package browserlogin drives a visible Chromium-family browser via the
// DevTools protocol (chromedp) so the user can log in to a site and lazyleet
// reads the resulting cookies straight from the running browser — no OS
// keychain, no cookie-file decryption.
//
// It is written as a small reusable "browser layer": later features
// (Cloudflare-challenge fallback, editorial scraping, silent session refresh)
// can build on Capture / the same chromedp context.
package browserlogin

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// Options configures Capture.
type Options struct {
	LoginURL     string        // page to open, e.g. https://leetcode.com/accounts/login/
	CookieDomain string        // return cookies whose domain contains this, e.g. "leetcode.com"
	WaitFor      []string      // cookie names that must all have non-empty values
	Timeout      time.Duration // max wait for login; 0 -> 5 minutes
	ProfileDir   string        // persistent user-data-dir; "" -> chromedp's temp dir
	ExecPath     string        // explicit browser binary; "" -> auto-discover
	Poll         time.Duration // cookie poll interval; 0 -> 1s
	OnStatus     func(string)  // optional progress callback
}

// Capture launches a browser at o.LoginURL and blocks until every cookie in
// o.WaitFor has a value for o.CookieDomain (success), the deadline passes, the
// context is cancelled, or the user closes the browser.
func Capture(ctx context.Context, o Options) (map[string]string, error) {
	if o.CookieDomain == "" || len(o.WaitFor) == 0 {
		return nil, fmt.Errorf("browserlogin: CookieDomain and WaitFor are required")
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

	allocOpts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	allocOpts = append(allocOpts,
		chromedp.Flag("headless", false),
		chromedp.Flag("no-first-run", true),
		chromedp.Flag("no-default-browser-check", true),
	)
	if o.ProfileDir != "" {
		allocOpts = append(allocOpts, chromedp.UserDataDir(o.ProfileDir))
	}
	if o.ExecPath != "" {
		allocOpts = append(allocOpts, chromedp.ExecPath(o.ExecPath))
	}

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(ctx, allocOpts...)
	defer cancelAlloc()
	browserCtx, cancelBrowser := chromedp.NewContext(allocCtx)
	defer cancelBrowser()

	deadlineCtx, cancelDeadline := context.WithTimeout(browserCtx, timeout)
	defer cancelDeadline()

	if err := chromedp.Run(deadlineCtx, chromedp.Navigate(o.LoginURL)); err != nil {
		return nil, launchError(err)
	}
	status("browser open — log in to LeetCode, then return here")

	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	for {
		vals, err := readCookies(deadlineCtx, o.CookieDomain, o.WaitFor)
		if err == nil && HaveAll(vals, o.WaitFor) {
			status("captured session")
			return vals, nil
		}
		select {
		case <-deadlineCtx.Done():
			if errIsClosed(browserCtx.Err()) {
				return nil, fmt.Errorf("browser was closed before login completed")
			}
			return nil, fmt.Errorf("timed out after %s waiting for a LeetCode login", timeout)
		case <-ticker.C:
		}
	}
}

func readCookies(ctx context.Context, domain string, want []string) (map[string]string, error) {
	var cookies []*network.Cookie
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var e error
		cookies, e = network.GetCookies().Do(ctx)
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

func errIsClosed(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "closed") || strings.Contains(s, "canceled") || strings.Contains(s, "cancelled")
}

func launchError(err error) error {
	if strings.Contains(err.Error(), "exec:") || strings.Contains(err.Error(), "executable file not found") {
		return fmt.Errorf("could not start a Chromium-family browser (Chrome/Chromium/Edge/Brave).\n"+
			"Install one, pass --browser-path, or use `lazyleet auth` / `lazyleet auth --manual`.\n(%v)", err)
	}
	return fmt.Errorf("browser launch failed: %w", err)
}

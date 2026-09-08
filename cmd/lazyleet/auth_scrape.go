package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/sven97/lazyleet/internal/browsercookies"
	"github.com/sven97/lazyleet/internal/leetcode"
)

var cookieNames = []string{"LEETCODE_SESSION", "csrftoken"}

func newAuthImportCmd(app *appContext) *cobra.Command {
	var browser string
	cmd := &cobra.Command{
		Use:   "import",
		Short: "Reuse the LeetCode cookies from a browser you're already signed in to",
		Long: "Reads LEETCODE_SESSION and csrftoken from a browser's cookie store.\n" +
			"Without --browser it tries keychain-free browsers (Firefox, Safari) first\n" +
			"and only falls back to Chrome-family browsers, which prompt for the OS\n" +
			"keychain on macOS. `lazyleet auth browsers` lists what was detected.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			creds, source, err := importFromBrowser(cmd, app, browser)
			if err != nil {
				return err
			}
			return finishAuth(cmd, app, creds, source)
		},
	}
	cmd.Flags().StringVar(&browser, "browser", "", "restrict to one browser (chrome, firefox, safari, edge, brave, arc, …)")
	return cmd
}

func newAuthPasteCmd(app *appContext) *cobra.Command {
	var fromStdin bool
	cmd := &cobra.Command{
		Use:   "paste",
		Short: "Enter LEETCODE_SESSION and csrftoken yourself",
		Long: "Prompts (hidden) for the two cookie values. Copy them from your browser's\n" +
			"DevTools → Application → Cookies. With --stdin, reads LEETCODE_SESSION on\n" +
			"line 1 and csrftoken on line 2 (for scripting).",
		RunE: func(cmd *cobra.Command, _ []string) error {
			var creds leetcode.Credentials
			var err error
			source := "manual entry"
			if fromStdin {
				creds, err = credsFromStdin(app)
				source = "stdin"
			} else {
				creds, err = credsFromPrompt(cmd, app)
			}
			if err != nil {
				return err
			}
			return finishAuth(cmd, app, creds, source)
		},
	}
	cmd.Flags().BoolVar(&fromStdin, "stdin", false, "read the two values from stdin instead of prompting")
	return cmd
}

// importFromBrowser reads the LeetCode cookies from a browser store. Without an
// explicit browser it tries keychain-free browsers first and only falls back to
// Chromium-family browsers — which prompt for the OS keychain on macOS.
func importFromBrowser(cmd *cobra.Command, app *appContext, browser string) (leetcode.Credentials, string, error) {
	ctx := cmd.Context()
	host := siteHost(app)

	if browser != "" {
		cookies := browsercookies.Read(ctx, host, browser, true, cookieNames...)
		if vals, source, ok := browsercookies.Pick(cookies, cookieNames...); ok {
			return credsFromMap(app, vals), source, nil
		}
		return leetcode.Credentials{}, "", noCookiesError(ctx, host, browser)
	}

	// pass 1: browsers that don't need the keychain
	cookies := browsercookies.Read(ctx, host, "", false, cookieNames...)
	if vals, source, ok := browsercookies.Pick(cookies, cookieNames...); ok {
		return credsFromMap(app, vals), source, nil
	}

	// pass 2: Chromium-family (may prompt)
	fmt.Fprintln(cmd.ErrOrStderr(),
		"no LeetCode session in Firefox/Safari — checking Chrome-family browsers.\n"+
			`macOS may ask for your login keychain password; choose "Always Allow".`)
	cookies = browsercookies.Read(ctx, host, "", true, cookieNames...)
	if vals, source, ok := browsercookies.Pick(cookies, cookieNames...); ok {
		return credsFromMap(app, vals), source, nil
	}
	return leetcode.Credentials{}, "", noCookiesError(ctx, host, "")
}

func noCookiesError(ctx context.Context, host, browser string) error {
	var b strings.Builder
	b.WriteString("no LeetCode session cookies found")
	if browser != "" {
		fmt.Fprintf(&b, " for browser %q", browser)
	}
	fmt.Fprintf(&b, " — sign in at https://%s/ in one of these browsers, then retry:\n", host)
	stores := browsercookies.Stores(ctx)
	if len(stores) == 0 {
		b.WriteString("  (no browser cookie stores detected on this machine)\n")
	}
	for _, s := range stores {
		b.WriteString("  " + s + "\n")
	}
	b.WriteString("or use `lazyleet auth` (browser sign-in) or `lazyleet auth paste`")
	return fmt.Errorf("%s", b.String())
}

func credsFromPrompt(cmd *cobra.Command, app *appContext) (leetcode.Credentials, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return leetcode.Credentials{}, fmt.Errorf("stdin is not a terminal; use `lazyleet auth paste --stdin`")
	}
	out := cmd.OutOrStdout()
	fmt.Fprint(out, "LEETCODE_SESSION: ")
	b1, err := term.ReadPassword(fd)
	fmt.Fprintln(out)
	if err != nil {
		return leetcode.Credentials{}, err
	}
	fmt.Fprint(out, "csrftoken: ")
	b2, err := term.ReadPassword(fd)
	fmt.Fprintln(out)
	if err != nil {
		return leetcode.Credentials{}, err
	}
	return buildCreds(app, string(b1), string(b2))
}

func credsFromStdin(app *appContext) (leetcode.Credentials, error) {
	sc := bufio.NewScanner(os.Stdin)
	var lines []string
	for sc.Scan() {
		lines = append(lines, strings.TrimSpace(sc.Text()))
	}
	if err := sc.Err(); err != nil {
		return leetcode.Credentials{}, err
	}
	if len(lines) < 2 {
		return leetcode.Credentials{}, fmt.Errorf("expected two lines on stdin: LEETCODE_SESSION then csrftoken")
	}
	return buildCreds(app, lines[0], lines[1])
}

func buildCreds(app *appContext, session, csrf string) (leetcode.Credentials, error) {
	session, csrf = strings.TrimSpace(session), strings.TrimSpace(csrf)
	if session == "" || csrf == "" {
		return leetcode.Credentials{}, fmt.Errorf("both LEETCODE_SESSION and csrftoken are required")
	}
	return leetcode.Credentials{Session: session, CSRFToken: csrf, Region: string(app.cfg.Region)}, nil
}

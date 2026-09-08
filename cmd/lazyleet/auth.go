package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/sven97/lazyleet/internal/browsercookies"
	"github.com/sven97/lazyleet/internal/browserlogin"
	"github.com/sven97/lazyleet/internal/config"
	"github.com/sven97/lazyleet/internal/leetcode"
)

func newAuthCmd(app *appContext) *cobra.Command {
	var (
		browser   string
		manual    bool
		fromStdin bool
	)

	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Authenticate by importing your LeetCode session from a browser",
		Long: "By default lazyleet reads the LEETCODE_SESSION and csrftoken cookies\n" +
			"straight from a browser where you are logged in to LeetCode (Chrome,\n" +
			"Firefox, Safari, Edge, Brave, Arc, Vivaldi, Opera, …). Nothing is sent\n" +
			"anywhere — the two values are written to auth.json (0600) on this\n" +
			"machine. `lazyleet auth logout` removes them.\n\n" +
			"  lazyleet auth            read cookies from an already-logged-in browser\n" +
			"  lazyleet auth login      open a browser window to log in (no keychain prompt)\n" +
			"  lazyleet auth --browser  restrict the read to one browser\n" +
			"  lazyleet auth browsers   list detected browser cookie stores\n" +
			"  lazyleet auth --manual   paste the two cookies yourself",
		RunE: func(cmd *cobra.Command, _ []string) error {
			var creds leetcode.Credentials
			var source string
			var err error

			switch {
			case fromStdin:
				creds, err = credsFromStdin(app)
				source = "stdin"
			case manual:
				creds, err = credsFromPrompt(cmd, app)
				source = "manual entry"
			default:
				creds, source, err = importFromBrowser(cmd, app, browser)
			}
			if err != nil {
				return err
			}

			if err := creds.Save(app.paths.AuthFile); err != nil {
				return err
			}

			user, verr := verifyAuth(cmd.Context(), app)
			if verr != nil {
				fmt.Fprintf(cmd.ErrOrStderr(),
					"saved cookies from %s to %s, but LeetCode did not accept them: %v\n"+
						"Log in again at https://%s/ and retry.\n",
					source, app.paths.AuthFile, verr, siteHost(app))
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "authenticated as %s (from %s)\n", user, source)
			return nil
		},
	}
	cmd.Flags().StringVar(&browser, "browser", "", "restrict the import to one browser (chrome, firefox, safari, edge, brave, arc, …)")
	cmd.Flags().BoolVar(&manual, "manual", false, "paste LEETCODE_SESSION and csrftoken yourself instead of reading a browser")
	cmd.Flags().BoolVar(&fromStdin, "stdin", false, "read LEETCODE_SESSION on line 1 and csrftoken on line 2 from stdin")

	var browserPath string
	var fresh bool
	loginCmd := &cobra.Command{
		Use:   "login",
		Short: "Log in via a browser window lazyleet controls (no keychain prompt)",
		Long: "Opens a visible Chrome/Chromium/Edge/Brave window at the LeetCode login\n" +
			"page. Log in there (Cloudflare/captcha included) and lazyleet reads the\n" +
			"session straight from the running browser over the DevTools protocol —\n" +
			"nothing to decrypt, no OS keychain prompt.\n\n" +
			"Uses a dedicated profile at <data-dir>/browser so later logins are quick.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			host := siteHost(app)
			profile := filepath.Join(app.paths.DataDir, "browser")
			if fresh {
				_ = os.RemoveAll(profile)
			}
			if err := app.paths.EnsureDirs(); err != nil {
				return err
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), 6*time.Minute)
			defer cancel()

			fmt.Fprintln(cmd.ErrOrStderr(), "opening a browser… log in to LeetCode, then come back here.")
			vals, err := browserlogin.Capture(ctx, browserlogin.Options{
				LoginURL:     "https://" + host + "/accounts/login/",
				CookieDomain: host,
				WaitFor:      []string{"LEETCODE_SESSION", "csrftoken"},
				ProfileDir:   profile,
				ExecPath:     browserPath,
				OnStatus:     func(s string) { fmt.Fprintln(cmd.ErrOrStderr(), s) },
			})
			if err != nil {
				return err
			}

			creds := leetcode.Credentials{
				Session:   vals["LEETCODE_SESSION"],
				CSRFToken: vals["csrftoken"],
				Region:    string(app.cfg.Region),
			}
			if err := creds.Save(app.paths.AuthFile); err != nil {
				return err
			}
			user, verr := verifyAuth(cmd.Context(), app)
			if verr != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "saved cookies but LeetCode did not accept them: %v\n", verr)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "authenticated as %s (via browser login)\n", user)
			return nil
		},
	}
	loginCmd.Flags().StringVar(&browserPath, "browser-path", "", "path to a Chromium-family browser binary (default: auto-discover)")
	loginCmd.Flags().BoolVar(&fresh, "fresh", false, "wipe the lazyleet browser profile and start a clean login")
	cmd.AddCommand(loginCmd)

	cmd.AddCommand(&cobra.Command{
		Use:   "browsers",
		Short: "List the browser cookie stores lazyleet can read",
		RunE: func(cmd *cobra.Command, _ []string) error {
			stores := browsercookies.Stores(cmd.Context())
			if len(stores) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no browser cookie stores found")
				return nil
			}
			for _, s := range stores {
				fmt.Fprintln(cmd.OutOrStdout(), "  "+s)
			}
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show whether credentials are stored and valid",
		RunE: func(cmd *cobra.Command, _ []string) error {
			creds, err := app.loadCredentials()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if creds.Anonymous() {
				fmt.Fprintln(out, "not authenticated (anonymous access only) — run `lazyleet auth`")
				return nil
			}
			fmt.Fprintf(out, "auth file : %s\n", app.paths.AuthFile)
			fmt.Fprintf(out, "region    : %s\n", firstNonEmpty(creds.Region, string(app.cfg.Region)))
			if creds.SavedAt > 0 {
				fmt.Fprintf(out, "saved     : %s\n", time.Unix(creds.SavedAt, 0).Format(time.RFC3339))
			}
			user, err := verifyAuth(cmd.Context(), app)
			if err != nil {
				fmt.Fprintf(out, "status    : INVALID (%v) — run `lazyleet auth`\n", err)
				return nil
			}
			fmt.Fprintf(out, "status    : ok, logged in as %s\n", user)
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "logout",
		Short: "Delete the stored credentials",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := leetcode.DeleteCredentials(app.paths.AuthFile); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "removed", app.paths.AuthFile)
			return nil
		},
	})

	return cmd
}

func siteHost(app *appContext) string {
	if app.cfg.Region == config.RegionCN {
		return "leetcode.cn"
	}
	return "leetcode.com"
}

// importFromBrowser reads the LeetCode cookies from a browser store. Without an
// explicit --browser it tries keychain-free browsers (Firefox, Safari) first
// and only falls back to Chromium-family browsers — which prompt for the OS
// keychain on macOS — if nothing else has the session.
func importFromBrowser(cmd *cobra.Command, app *appContext, browser string) (leetcode.Credentials, string, error) {
	ctx := cmd.Context()
	host := siteHost(app)
	names := []string{"LEETCODE_SESSION", "csrftoken"}

	if browser != "" {
		cookies := browsercookies.Read(ctx, host, browser, true, names...)
		if vals, source, ok := browsercookies.Pick(cookies, names...); ok {
			return credsFrom(app, vals, source)
		}
		return leetcode.Credentials{}, "", noCookiesError(ctx, host, browser)
	}

	// pass 1: browsers that don't need the keychain
	cookies := browsercookies.Read(ctx, host, "", false, names...)
	if vals, source, ok := browsercookies.Pick(cookies, names...); ok {
		return credsFrom(app, vals, source)
	}

	// pass 2: Chromium-family (may prompt)
	fmt.Fprintln(cmd.ErrOrStderr(),
		"no LeetCode session in Firefox/Safari — checking Chrome-family browsers.\n"+
			"macOS may ask for your login keychain password; choose \"Always Allow\" so it doesn't ask again.")
	cookies = browsercookies.Read(ctx, host, "", true, names...)
	if vals, source, ok := browsercookies.Pick(cookies, names...); ok {
		return credsFrom(app, vals, source)
	}
	return leetcode.Credentials{}, "", noCookiesError(ctx, host, "")
}

func credsFrom(app *appContext, vals map[string]string, source string) (leetcode.Credentials, string, error) {
	return leetcode.Credentials{
		Session:   vals["LEETCODE_SESSION"],
		CSRFToken: vals["csrftoken"],
		Region:    string(app.cfg.Region),
	}, source, nil
}

func noCookiesError(ctx context.Context, host, browser string) error {
	var b strings.Builder
	b.WriteString("no LeetCode session cookies found")
	if browser != "" {
		fmt.Fprintf(&b, " for browser %q", browser)
	}
	fmt.Fprintf(&b, ".\nLog in at https://%s/ in one of these browsers, then run `lazyleet auth` again:\n", host)
	stores := browsercookies.Stores(ctx)
	if len(stores) == 0 {
		b.WriteString("  (no browser cookie stores detected on this machine)\n")
	}
	for _, s := range stores {
		b.WriteString("  " + s + "\n")
	}
	b.WriteString("Or paste the cookies manually with `lazyleet auth --manual`.")
	return fmt.Errorf("%s", b.String())
}

func credsFromPrompt(cmd *cobra.Command, app *appContext) (leetcode.Credentials, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return leetcode.Credentials{}, fmt.Errorf("stdin is not a terminal; use `lazyleet auth --stdin`")
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

// verifyAuth runs a tiny authenticated query and returns the signed-in username.
func verifyAuth(ctx context.Context, app *appContext) (string, error) {
	client, err := app.newClient()
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	user, err := client.WhoAmI(ctx)
	if err != nil {
		return "", err
	}
	if user == "" {
		return "", fmt.Errorf("LeetCode did not recognise the session (expired?)")
	}
	return user, nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

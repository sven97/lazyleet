package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/sven97/lazyleet/internal/browserlogin"
)

func newAuthLoginCmd(app *appContext) *cobra.Command {
	var browserPath string
	var fresh bool

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Sign in via a browser window lazyleet opens (same as bare `auth`)",
		Long: "Opens a plain Chrome/Chromium/Edge/Brave window at the LeetCode sign-in\n" +
			"page. Sign in there (Cloudflare/captcha included) and lazyleet reads the\n" +
			"session over the DevTools protocol once it appears — nothing to decrypt,\n" +
			"no keychain prompt. A dedicated profile at <data-dir>/browser keeps later\n" +
			"sign-ins quick.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runBrowserLogin(cmd, app, browserPath, fresh)
		},
	}
	cmd.Flags().StringVar(&browserPath, "browser-path", "", "path to a Chromium-family browser binary (default: auto-discover)")
	cmd.Flags().BoolVar(&fresh, "fresh", false, "wipe the lazyleet browser profile and start clean")
	return cmd
}

func runBrowserLogin(cmd *cobra.Command, app *appContext, browserPath string, fresh bool) error {
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

	fmt.Fprintln(cmd.ErrOrStderr(), "opening a browser — sign in to LeetCode, then come back here")
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
	return finishAuth(cmd, app, credsFromMap(app, vals), "browser login")
}

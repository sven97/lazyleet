package main

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/sven97/lazyleet/internal/browsercookies"
	"github.com/sven97/lazyleet/internal/config"
	"github.com/sven97/lazyleet/internal/leetcode"
)

// newAuthCmd builds the `auth` command tree. Bare `lazyleet auth` runs the
// browser-login flow (the recommended path); `import` / `paste` are fallbacks.
func newAuthCmd(app *appContext) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Authenticate with LeetCode (opens a browser to sign in)",
		Long: "`lazyleet auth` opens a browser window at the LeetCode sign-in page and\n" +
			"reads your session from it — no cookie files, no OS keychain prompt.\n" +
			"The LEETCODE_SESSION and csrftoken values are written to auth.json (0600)\n" +
			"on this machine and nothing is sent anywhere.\n\n" +
			"  lazyleet auth            sign in via a browser window (recommended)\n" +
			"  lazyleet auth import     reuse cookies from an already-signed-in browser\n" +
			"  lazyleet auth paste      enter LEETCODE_SESSION + csrftoken yourself\n" +
			"  lazyleet auth browsers   list detected browser cookie stores\n" +
			"  lazyleet auth status     check the stored credentials\n" +
			"  lazyleet auth logout     delete the stored credentials",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runBrowserLogin(cmd, app, "", false)
		},
	}

	cmd.AddCommand(
		newAuthLoginCmd(app),
		newAuthImportCmd(app),
		newAuthPasteCmd(app),
		newAuthBrowsersCmd(app),
		newAuthStatusCmd(app),
		newAuthLogoutCmd(app),
	)
	return cmd
}

func newAuthBrowsersCmd(_ *appContext) *cobra.Command {
	return &cobra.Command{
		Use:   "browsers",
		Short: "List the browser cookie stores `auth import` can read",
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
	}
}

func newAuthStatusCmd(app *appContext) *cobra.Command {
	return &cobra.Command{
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
			fmt.Fprintf(out, "status    : ok, signed in as %s\n", user)
			return nil
		},
	}
}

func newAuthLogoutCmd(app *appContext) *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Delete the stored credentials",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := leetcode.DeleteCredentials(app.paths.AuthFile); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "removed", app.paths.AuthFile)
			return nil
		},
	}
}

// --- shared helpers -------------------------------------------------------

func siteHost(app *appContext) string {
	if app.cfg.Region == config.RegionCN {
		return "leetcode.cn"
	}
	return "leetcode.com"
}

func credsFromMap(app *appContext, vals map[string]string) leetcode.Credentials {
	return leetcode.Credentials{
		Session:   vals["LEETCODE_SESSION"],
		CSRFToken: vals["csrftoken"],
		Region:    string(app.cfg.Region),
	}
}

// finishAuth saves credentials, verifies them against LeetCode, and reports the
// outcome. A rejected session is a warning, not an error (the file is still
// written so the user can inspect it).
func finishAuth(cmd *cobra.Command, app *appContext, creds leetcode.Credentials, source string) error {
	if err := creds.Save(app.paths.AuthFile); err != nil {
		return err
	}
	user, err := verifyAuth(cmd.Context(), app)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(),
			"saved credentials from %s, but LeetCode did not accept them: %v\n"+
				"Sign in again at https://%s/ and retry.\n", source, err, siteHost(app))
		return nil
	}
	fmt.Fprintf(cmd.OutOrStdout(), "authenticated as %s (%s)\n", user, source)
	fmt.Fprintln(cmd.OutOrStdout(), "refreshing solve progress…")
	ctx, cancel := context.WithTimeout(cmd.Context(), 90*time.Second)
	defer cancel()
	client, err := app.newClient()
	if err == nil {
		var solved int
		solved, err = refreshAuthProgress(ctx, app, client)
		if err == nil {
			fmt.Fprintf(cmd.OutOrStdout(), "progress synced · %d solved\n", solved)
		}
	}
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Signed in, but progress refresh failed: %v\nRetry with `lazyleet sync --progress`.\n", err)
	}
	fmt.Fprintln(cmd.OutOrStdout(), "In an open workspace, retry R/s; in browse, press s to refresh your account and progress.")
	return nil
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

// refreshAuthProgress also seeds an empty catalog on first sign-in so status
// updates have rows to attach to.
func refreshAuthProgress(ctx context.Context, app *appContext, client *leetcode.Client) (int, error) {
	db, err := app.openStore()
	if err != nil {
		return 0, err
	}
	defer db.Close()
	n, err := db.ProblemCount(ctx)
	if err != nil {
		return 0, err
	}
	if n == 0 {
		if _, err := fetchAndCacheProblems(ctx, client, db, nil); err != nil {
			return 0, err
		}
	}
	return fetchAndCacheProgress(ctx, client, db)
}

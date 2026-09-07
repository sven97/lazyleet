package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/sven97/lazyleet/internal/leetcode"
)

func newAuthCmd(app *appContext) *cobra.Command {
	var fromStdin bool

	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Store your LeetCode session cookies locally",
		Long: "Copy two cookies from a browser where you are logged in to LeetCode\n" +
			"(DevTools → Application → Cookies): LEETCODE_SESSION and csrftoken.\n\n" +
			"They are written to auth.json with 0600 permissions and never leave\n" +
			"this machine. Run `lazyleet auth logout` to remove them.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			var session, csrf string
			var err error

			if fromStdin {
				session, csrf, err = readCredsFromStdin()
			} else {
				session, csrf, err = promptCreds(cmd)
			}
			if err != nil {
				return err
			}
			session, csrf = strings.TrimSpace(session), strings.TrimSpace(csrf)
			if session == "" || csrf == "" {
				return fmt.Errorf("both LEETCODE_SESSION and csrftoken are required")
			}

			creds := leetcode.Credentials{
				Session:   session,
				CSRFToken: csrf,
				Region:    string(app.cfg.Region),
			}
			if err := creds.Save(app.paths.AuthFile); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "saved to %s\n", app.paths.AuthFile)

			// Best-effort verification.
			if err := verifyAuth(cmd, app); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not verify credentials: %v\n", err)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&fromStdin, "stdin", false, "read SESSION on line 1 and csrftoken on line 2 from stdin")

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
				fmt.Fprintln(out, "not authenticated (anonymous access only)")
				return nil
			}
			fmt.Fprintf(out, "auth file : %s\n", app.paths.AuthFile)
			fmt.Fprintf(out, "region    : %s\n", firstNonEmpty(creds.Region, string(app.cfg.Region)))
			if creds.SavedAt > 0 {
				fmt.Fprintf(out, "saved     : %s\n", time.Unix(creds.SavedAt, 0).Format(time.RFC3339))
			}
			if err := verifyAuth(cmd, app); err != nil {
				fmt.Fprintf(out, "status    : INVALID (%v)\n", err)
				return nil
			}
			fmt.Fprintln(out, "status    : ok")
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

func promptCreds(cmd *cobra.Command) (session, csrf string, err error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return "", "", fmt.Errorf("stdin is not a terminal; use `lazyleet auth --stdin`")
	}
	out := cmd.OutOrStdout()
	fmt.Fprint(out, "LEETCODE_SESSION: ")
	b1, err := term.ReadPassword(fd)
	fmt.Fprintln(out)
	if err != nil {
		return "", "", err
	}
	fmt.Fprint(out, "csrftoken: ")
	b2, err := term.ReadPassword(fd)
	fmt.Fprintln(out)
	if err != nil {
		return "", "", err
	}
	return string(b1), string(b2), nil
}

func readCredsFromStdin() (session, csrf string, err error) {
	sc := bufio.NewScanner(os.Stdin)
	var lines []string
	for sc.Scan() {
		lines = append(lines, strings.TrimSpace(sc.Text()))
	}
	if err := sc.Err(); err != nil {
		return "", "", err
	}
	if len(lines) < 2 {
		return "", "", fmt.Errorf("expected two lines on stdin: SESSION then csrftoken")
	}
	return lines[0], lines[1], nil
}

// verifyAuth runs a tiny authenticated query and reports whether it came back
// authenticated.
func verifyAuth(cmd *cobra.Command, app *appContext) error {
	client, err := app.newClient()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), 15*time.Second)
	defer cancel()

	user, err := client.WhoAmI(ctx)
	if err != nil {
		return err
	}
	if user == "" {
		return fmt.Errorf("LeetCode did not recognise the session (expired?)")
	}
	fmt.Fprintf(cmd.OutOrStdout(), "logged in as: %s\n", user)
	return nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

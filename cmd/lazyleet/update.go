package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/sven97/lazyleet/internal/selfupdate"
	"github.com/sven97/lazyleet/internal/tui"
)

// noUpdateCheckEnv turns the launch-time check off without touching config
// (also honoured: CI, where nobody reads the banner).
const noUpdateCheckEnv = "LAZYLEET_NO_UPDATE_CHECK"

// updateService adapts internal/selfupdate to tui.Updater and backs the
// `lazyleet update` command.
type updateService struct {
	install selfupdate.Install
	checker *selfupdate.Checker
	updater *selfupdate.Updater
	enabled bool

	mu     sync.Mutex
	latest selfupdate.Release // set by Check, used by Apply
}

func newUpdateService(app *appContext) *updateService {
	in := selfupdate.Detect(version)
	selfupdate.CleanupOld(in.Exe)
	return &updateService{
		install: in,
		checker: selfupdate.NewChecker(filepath.Join(app.paths.DataDir, "update-check.json")),
		updater: selfupdate.NewUpdater(),
		enabled: app.cfg.UpdateCheck && os.Getenv(noUpdateCheckEnv) == "" && os.Getenv("CI") == "",
	}
}

// Check implements tui.Updater: the cached daily check, skipped for source
// builds (nothing to compare against) and when disabled.
func (s *updateService) Check(ctx context.Context) (tui.UpdateInfo, error) {
	if !s.enabled || s.install.Method == selfupdate.MethodSource {
		return tui.UpdateInfo{}, nil
	}
	return s.check(ctx, false)
}

func (s *updateService) check(ctx context.Context, force bool) (tui.UpdateInfo, error) {
	rel, err := s.checker.Latest(ctx, force)
	if err != nil {
		return tui.UpdateInfo{}, err
	}
	if !selfupdate.Newer(rel.Tag, s.install.Version) {
		return tui.UpdateInfo{}, nil
	}
	s.mu.Lock()
	s.latest = rel
	s.mu.Unlock()
	ok, why := s.updater.CanApply(s.install)
	manual := s.install.ManualCommand()
	if !ok && s.install.Method == selfupdate.MethodBinary {
		manual = why + "; download it from " + rel.URL
	}
	return tui.UpdateInfo{
		Latest:   rel.Tag,
		Method:   s.install.Method.String(),
		CanApply: ok,
		Manual:   manual,
	}, nil
}

// Apply implements tui.Updater.
func (s *updateService) Apply(ctx context.Context) error {
	s.mu.Lock()
	rel := s.latest
	s.mu.Unlock()
	if rel.Tag == "" {
		return errors.New("no update pending")
	}
	return s.updater.Apply(ctx, s.install, rel)
}

func newUpdateCmd(app *appContext) *cobra.Command {
	var checkOnly bool
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update lazyleet to the latest release",
		Long: "Check GitHub for a newer lazyleet release and install it the same way\n" +
			"the current one was installed (Homebrew, go install, or release binary).",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			s := newUpdateService(app)
			in := s.install
			fmt.Fprintf(out, "installed  %s (%s)\n", versionString(), in.Method)
			if in.Method == selfupdate.MethodSource {
				fmt.Fprintf(out, "this is a source build; update it with: %s\n", in.ManualCommand())
				return nil
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Minute)
			defer cancel()
			info, err := s.check(ctx, true)
			if err != nil {
				return err
			}
			if info.Latest == "" {
				fmt.Fprintln(out, "lazyleet is up to date")
				return nil
			}
			fmt.Fprintf(out, "latest     %s\n", info.Latest)
			if !info.CanApply {
				return fmt.Errorf("cannot update automatically: %s", info.Manual)
			}
			if checkOnly {
				fmt.Fprintln(out, "run `lazyleet update` to install it")
				return nil
			}
			fmt.Fprintf(out, "updating via %s…\n", in.Method)
			if err := s.Apply(ctx); err != nil {
				return err
			}
			fmt.Fprintf(out, "updated to %s\n", info.Latest)
			return nil
		},
	}
	cmd.Flags().BoolVar(&checkOnly, "check", false, "only report whether an update is available")
	return cmd
}

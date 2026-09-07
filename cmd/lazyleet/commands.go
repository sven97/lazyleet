package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func newAuthCmd(_ *appContext) *cobra.Command {
	return &cobra.Command{
		Use:   "auth",
		Short: "Store your LeetCode session cookies (Phase 1)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return fmt.Errorf("not implemented yet: `auth` lands in Phase 1")
		},
	}
}

func newSyncCmd(_ *appContext) *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Refresh the local problem/study-plan cache from LeetCode (Phase 1)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return fmt.Errorf("not implemented yet: `sync` lands in Phase 1")
		},
	}
}

func newDebugCmd(app *appContext) *cobra.Command {
	debug := &cobra.Command{
		Use:    "debug",
		Short:  "Introspection helpers for development",
		Hidden: true,
	}

	debug.AddCommand(&cobra.Command{
		Use:   "paths",
		Short: "Print every resolved filesystem location",
		RunE: func(cmd *cobra.Command, _ []string) error {
			p := app.paths
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "config dir     %s\n", p.ConfigDir)
			fmt.Fprintf(out, "config file    %s\n", p.ConfigFile)
			fmt.Fprintf(out, "data dir       %s\n", p.DataDir)
			fmt.Fprintf(out, "database       %s\n", p.DatabaseFile)
			fmt.Fprintf(out, "auth file      %s\n", p.AuthFile)
			fmt.Fprintf(out, "workspace root %s\n", p.WorkspaceRoot)
			return nil
		},
	})

	debug.AddCommand(&cobra.Command{
		Use:   "config",
		Short: "Print the effective configuration as YAML",
		RunE: func(cmd *cobra.Command, _ []string) error {
			out, err := yaml.Marshal(app.cfg)
			if err != nil {
				return err
			}
			cmd.OutOrStdout().Write(out)
			return nil
		},
	})

	return debug
}

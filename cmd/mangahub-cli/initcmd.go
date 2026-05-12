package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Create or reset CLI configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := saveConfigFile(cfgFile, cfg.clone()); err != nil {
				return err
			}
			fmt.Printf("Configuration saved to %s\n", cfgFile)
			fmt.Printf("API base URL: %s\n", cfg.APIBaseURL)
			return nil
		},
	}
}

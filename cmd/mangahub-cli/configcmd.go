package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect or update local CLI configuration",
	}
	cmd.AddCommand(newConfigShowCmd())
	cmd.AddCommand(newConfigSetCmd())
	return cmd
}

func newConfigShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Print the current CLI config",
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, err := json.MarshalIndent(cfg.clone(), "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(raw))
			fmt.Printf("Config file: %s\n", cfgFile)
			return nil
		},
	}
}

func newConfigSetCmd() *cobra.Command {
	var key, value string
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Set a config value",
		RunE: func(cmd *cobra.Command, args []string) error {
			switch strings.ToLower(strings.TrimSpace(key)) {
			case "api", "api_base_url", "server", "server_url":
				cfg.APIBaseURL = value
			case "token":
				cfg.Token = value
			default:
				return fmt.Errorf("unknown config key: %s", key)
			}
			if err := saveConfigFile(cfgFile, cfg); err != nil {
				return err
			}
			fmt.Printf("Saved %s\n", key)
			return nil
		},
	}
	cmd.Flags().StringVar(&key, "key", "", "config key")
	cmd.Flags().StringVar(&value, "value", "", "config value")
	_ = cmd.MarkFlagRequired("key")
	_ = cmd.MarkFlagRequired("value")
	return cmd
}

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var (
	cfg     = newAppConfig()
	rootCmd = &cobra.Command{Use: "mangahub", Short: "MangaHub CLI"}
	cfgFile string
	apiBase string
)

func Execute() error { return rootCmd.Execute() }

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "path to CLI config file")
	rootCmd.PersistentFlags().StringVar(&apiBase, "api", "", "override API base URL (e.g. http://localhost:8080)")

	rootCmd.AddCommand(newInitCmd())
	rootCmd.AddCommand(newAuthCmd())
	rootCmd.AddCommand(newMangaCmd())
	rootCmd.AddCommand(newLibraryCmd())
	rootCmd.AddCommand(newProgressCmd())
	rootCmd.AddCommand(newSyncCmd())
	rootCmd.AddCommand(newNotifyCmd())
	rootCmd.AddCommand(newServerCmd())
	rootCmd.AddCommand(newConfigCmd())
}

func initConfig() {
	if cfgFile == "" {
		cfgFile = defaultConfigPath()
	}
	loaded, err := loadConfig(cfgFile)
	if err == nil {
		cfg = loaded
	}
	if apiBase != "" {
		cfg.APIBaseURL = apiBase
	}
}

func defaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".mangahub-cli.json"
	}
	return filepath.Join(home, ".mangahub", "cli.json")
}

func fatalf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}

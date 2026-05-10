package main

import (
	"fmt"
	"strings"

	"github.com/plbth/mangahub/pkg/models"
	"github.com/spf13/cobra"
)

func newProgressCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "progress",
		Short: "Reading progress commands",
	}
	cmd.AddCommand(newProgressUpdateCmd())
	cmd.AddCommand(newProgressHistoryCmd())
	cmd.AddCommand(newProgressSyncCmd())
	cmd.AddCommand(newProgressSyncStatusCmd())
	return cmd
}

func newProgressUpdateCmd() *cobra.Command {
	var mangaID, notes string
	var chapter, volume, rating int

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update current reading progress",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(mangaID) == "" {
				return fmt.Errorf("--manga-id is required")
			}
			if chapter <= 0 {
				return fmt.Errorf("--chapter must be greater than 0")
			}

			payload := map[string]any{
				"manga_id": mangaID,
				"chapter":  chapter,
				"volume":   volume,
				"rating":   rating,
				"notes":    notes,
			}

			client := newHTTPClient()
			var resp map[string]any
			if err := client.request("PUT", "/users/progress", payload, &resp, true); err != nil {
				return err
			}
			fmt.Println(resp["message"])
			return nil
		},
	}
	cmd.Flags().StringVar(&mangaID, "manga-id", "", "manga id")
	cmd.Flags().IntVar(&chapter, "chapter", 0, "chapter number")
	cmd.Flags().IntVar(&volume, "volume", 0, "volume number")
	cmd.Flags().IntVar(&rating, "rating", 0, "rating")
	cmd.Flags().StringVar(&notes, "notes", "", "notes")
	_ = cmd.MarkFlagRequired("manga-id")
	_ = cmd.MarkFlagRequired("chapter")
	return cmd
}

func newProgressHistoryCmd() *cobra.Command {
	var mangaID string
	cmd := &cobra.Command{
		Use:   "history",
		Short: "Show current tracked progress from the library",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newHTTPClient()
			var resp libraryResponse
			if err := client.request("GET", "/users/library", nil, &resp, true); err != nil {
				return err
			}
			if mangaID != "" {
				filtered := make([]models.LibraryView, 0, len(resp.Library))
				for _, item := range resp.Library {
					if strings.EqualFold(item.Manga.ID, mangaID) {
						filtered = append(filtered, item)
					}
				}
				resp.Library = filtered
				resp.Count = len(filtered)
			}
			fmt.Printf("Tracked entries: %d\n", resp.Count)
			printLibraryTable(resp.Library)
			return nil
		},
	}
	cmd.Flags().StringVar(&mangaID, "manga-id", "", "optional manga id filter")
	return cmd
}

func newProgressSyncCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Trigger a manual sync placeholder",
		Run: func(cmd *cobra.Command, args []string) {
			warnUnsupported("progress sync")
		},
	}
}

func newProgressSyncStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sync-status",
		Short: "Show sync status placeholder",
		Run: func(cmd *cobra.Command, args []string) {
			warnUnsupported("progress sync-status")
		},
	}
}

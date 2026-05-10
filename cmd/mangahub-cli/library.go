package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/plbth/mangahub/pkg/models"
	"github.com/spf13/cobra"
)

type libraryResponse struct {
	Count   int                  `json:"count"`
	Library []models.LibraryView `json:"library"`
}

func newLibraryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "library",
		Short: "Library management commands",
	}
	cmd.AddCommand(newLibraryAddCmd())
	cmd.AddCommand(newLibraryListCmd())
	cmd.AddCommand(newLibraryRemoveCmd())
	cmd.AddCommand(newLibraryUpdateCmd())
	return cmd
}

func newLibraryAddCmd() *cobra.Command {
	var mangaID, status, notes string
	var currentChapter, volume, rating int

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a manga to your library",
		RunE: func(cmd *cobra.Command, args []string) error {
			if mangaID == "" {
				return fmt.Errorf("--manga-id is required")
			}
			if status == "" {
				status = "reading"
			}

			payload := map[string]any{
				"manga_id":        mangaID,
				"status":          status,
				"current_chapter": currentChapter,
				"volume":          volume,
				"rating":          rating,
				"notes":           notes,
			}

			client := newHTTPClient()
			var resp map[string]any
			if err := client.request("POST", "/users/library", payload, &resp, true); err != nil {
				return err
			}
			fmt.Println(resp["message"])
			return nil
		},
	}
	cmd.Flags().StringVar(&mangaID, "manga-id", "", "manga id")
	cmd.Flags().StringVar(&status, "status", "reading", "library status")
	cmd.Flags().StringVar(&notes, "notes", "", "notes")
	cmd.Flags().IntVar(&currentChapter, "chapter", 0, "current chapter")
	cmd.Flags().IntVar(&volume, "volume", 0, "current volume")
	cmd.Flags().IntVar(&rating, "rating", 0, "rating")
	_ = cmd.MarkFlagRequired("manga-id")
	return cmd
}

func newLibraryListCmd() *cobra.Command {
	var status string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Show your library",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newHTTPClient()
			endpoint := "/users/library"
			if status != "" {
				endpoint += "?status=" + status
			}
			var resp libraryResponse
			if err := client.request("GET", endpoint, nil, &resp, true); err != nil {
				return err
			}
			fmt.Printf("Library items: %d\n", resp.Count)
			printLibraryTable(resp.Library)
			return nil
		},
	}
	cmd.Flags().StringVar(&status, "status", "", "filter by status")
	return cmd
}

func newLibraryRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove",
		Short: "Remove a manga from your library",
		Run: func(cmd *cobra.Command, args []string) {
			warnUnsupported("library remove")
		},
	}
}

func newLibraryUpdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "Update a library entry",
		Run: func(cmd *cobra.Command, args []string) {
			warnUnsupported("library update")
		},
	}
}

func printLibraryTable(items []models.LibraryView) {
	if len(items) == 0 {
		fmt.Println("No library items found.")
		return
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "MANGA ID\tTITLE\tSTATUS\tCHAPTER\tRATING\tUPDATED")
	for _, item := range items {
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%d\t%s\n",
			item.Manga.ID,
			item.Manga.Title,
			item.Progress.Status,
			item.Progress.CurrentChapter,
			item.Progress.Rating,
			item.Progress.UpdatedAt.Format("2006-01-02 15:04"),
		)
	}
	_ = w.Flush()
}

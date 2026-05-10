package main

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/plbth/mangahub/pkg/models"
	"github.com/spf13/cobra"
)

type mangaListResponse struct {
	Count int            `json:"count"`
	Manga []models.Manga `json:"manga"`
}

func newMangaCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "manga",
		Short: "Manga discovery commands",
	}
	cmd.AddCommand(newMangaSearchCmd())
	cmd.AddCommand(newMangaInfoCmd())
	cmd.AddCommand(newMangaListCmd())
	return cmd
}

func newMangaSearchCmd() *cobra.Command {
	var query, genre, status string
	var limit, page int

	cmd := &cobra.Command{
		Use:   "search",
		Short: "Search manga by title, author, genre, or status",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newHTTPClient()
			params := []string{}
			if query != "" {
				params = append(params, "query="+query)
			}
			if genre != "" {
				params = append(params, "genre="+genre)
			}
			if status != "" {
				params = append(params, "status="+status)
			}
			if limit > 0 {
				params = append(params, fmt.Sprintf("limit=%d", limit))
			}
			if page > 0 {
				params = append(params, fmt.Sprintf("page=%d", page))
			}

			endpoint := "/manga"
			if len(params) > 0 {
				endpoint += "?" + strings.Join(params, "&")
			}

			var resp mangaListResponse
			if err := client.request("GET", endpoint, nil, &resp, false); err != nil {
				return err
			}

			fmt.Printf("Found %d result(s)\n", resp.Count)
			printMangaTable(resp.Manga)
			return nil
		},
	}
	cmd.Flags().StringVar(&query, "query", "", "search text")
	cmd.Flags().StringVar(&genre, "genre", "", "genre filter")
	cmd.Flags().StringVar(&status, "status", "", "status filter")
	cmd.Flags().IntVar(&limit, "limit", 0, "page size")
	cmd.Flags().IntVar(&page, "page", 1, "page number")
	return cmd
}

func newMangaInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info <manga-id>",
		Short: "View one manga in detail",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newHTTPClient()
			var manga models.Manga
			if err := client.request("GET", "/manga/"+args[0], nil, &manga, false); err != nil {
				return err
			}
			printMangaDetail(manga)
			return nil
		},
	}
}

func newMangaListCmd() *cobra.Command {
	var genre, status string
	var limit, page int

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List public manga",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newHTTPClient()
			params := []string{}
			if genre != "" {
				params = append(params, "genre="+genre)
			}
			if status != "" {
				params = append(params, "status="+status)
			}
			if limit > 0 {
				params = append(params, fmt.Sprintf("limit=%d", limit))
			}
			if page > 0 {
				params = append(params, fmt.Sprintf("page=%d", page))
			}

			endpoint := "/manga"
			if len(params) > 0 {
				endpoint += "?" + strings.Join(params, "&")
			}

			var resp mangaListResponse
			if err := client.request("GET", endpoint, nil, &resp, false); err != nil {
				return err
			}
			fmt.Printf("Total results: %d\n", resp.Count)
			printMangaTable(resp.Manga)
			return nil
		},
	}
	cmd.Flags().StringVar(&genre, "genre", "", "genre filter")
	cmd.Flags().StringVar(&status, "status", "", "status filter")
	cmd.Flags().IntVar(&limit, "limit", 20, "page size")
	cmd.Flags().IntVar(&page, "page", 1, "page number")
	return cmd
}

func printMangaTable(items []models.Manga) {
	if len(items) == 0 {
		fmt.Println("No manga found.")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tTITLE\tAUTHOR\tSTATUS\tCHAPTERS")
	for _, m := range items {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\n", m.ID, m.Title, m.Author, m.Status, m.TotalChapters)
	}
	_ = w.Flush()
}

func printMangaDetail(m models.Manga) {
	fmt.Println("--------------------------------------------------")
	fmt.Printf("ID:            %s\n", m.ID)
	fmt.Printf("Title:         %s\n", m.Title)
	fmt.Printf("Author:        %s\n", m.Author)
	fmt.Printf("Genres:        %s\n", strings.Join(m.Genres, ", "))
	fmt.Printf("Status:        %s\n", m.Status)
	fmt.Printf("TotalChapters: %d\n", m.TotalChapters)
	fmt.Printf("CoverURL:      %s\n", m.CoverURL)
	fmt.Println("Description:")
	fmt.Printf("  %s\n", m.Description)
	fmt.Println("--------------------------------------------------")
}

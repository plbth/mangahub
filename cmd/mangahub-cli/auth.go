package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/plbth/mangahub/pkg/models"
	"github.com/spf13/cobra"
)

func newAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Authentication commands",
	}

	cmd.AddCommand(newAuthRegisterCmd())
	cmd.AddCommand(newAuthLoginCmd())
	cmd.AddCommand(newAuthLogoutCmd())
	cmd.AddCommand(newAuthStatusCmd())
	cmd.AddCommand(newAuthClearCmd())
	return cmd
}

func newAuthRegisterCmd() *cobra.Command {
	var username, email, password string
	cmd := &cobra.Command{
		Use:   "register",
		Short: "Register a new account",
		RunE: func(cmd *cobra.Command, args []string) error {
			if username == "" || email == "" {
				return fmt.Errorf("--username and --email are required")
			}
			if password == "" {
				password = promptPassword("Password: ")
				confirm := promptPassword("Confirm password: ")
				if password != confirm {
					return fmt.Errorf("password confirmation does not match")
				}
			}

			client := newHTTPClient()
			var resp models.AuthResponse
			if err := client.request("POST", "/auth/register", map[string]string{
				"username": username,
				"email":    email,
				"password": password,
			}, &resp, false); err != nil {
				return err
			}

			if err := saveToken(resp.Token); err != nil {
				return err
			}
			client.setToken(resp.Token)

			fmt.Println("Account created successfully.")
			showUser(resp.User)
			fmt.Println("Token saved to config.")
			return nil
		},
	}
	cmd.Flags().StringVar(&username, "username", "", "username")
	cmd.Flags().StringVar(&email, "email", "", "email")
	cmd.Flags().StringVar(&password, "password", "", "password")
	_ = cmd.MarkFlagRequired("username")
	_ = cmd.MarkFlagRequired("email")
	return cmd
}

func newAuthLoginCmd() *cobra.Command {
	var username, email, password string
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Login to get an authentication token",
		RunE: func(cmd *cobra.Command, args []string) error {
			if password == "" {
				password = promptPassword("Password: ")
			}
			body := map[string]string{"password": password}
			if strings.TrimSpace(username) != "" {
				body["username"] = username
			} else if strings.TrimSpace(email) != "" {
				body["email"] = email
			} else {
				return fmt.Errorf("either --username or --email is required")
			}

			client := newHTTPClient()
			var resp models.AuthResponse
			if err := client.request("POST", "/auth/login", body, &resp, false); err != nil {
				return err
			}

			if err := saveToken(resp.Token); err != nil {
				return err
			}
			fmt.Println("Login successful.")
			showUser(resp.User)
			fmt.Println("Token saved to config.")
			return nil
		},
	}
	cmd.Flags().StringVar(&username, "username", "", "username")
	cmd.Flags().StringVar(&email, "email", "", "email")
	cmd.Flags().StringVar(&password, "password", "", "password")
	return cmd
}

func newAuthLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Clear the stored authentication token",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := clearToken(); err != nil {
				return err
			}
			fmt.Println("Token cleared.")
			return nil
		},
	}
}

func newAuthStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show authentication status",
		Run: func(cmd *cobra.Command, args []string) {
			if strings.TrimSpace(cfg.Token) == "" {
				fmt.Println("Status: not logged in")
				fmt.Printf("Config file: %s\n", cfgFile)
				return
			}
			fmt.Println("Status: logged in")
			fmt.Printf("Config file: %s\n", cfgFile)
			printTokenClaims(cfg.Token)
		},
	}
}

func newAuthClearCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clear",
		Short: "Remove local authentication data",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := clearToken(); err != nil {
				return err
			}
			fmt.Println("Authentication token removed.")
			return nil
		},
	}
}

func promptPassword(label string) string {
	fmt.Print(label)
	reader := bufio.NewReader(os.Stdin)
	text, _ := reader.ReadString('\n')
	return strings.TrimSpace(text)
}

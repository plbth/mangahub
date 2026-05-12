package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/plbth/mangahub/pkg/models"
)

func saveToken(token string) error {
	return SaveConfig(token)
}

func clearToken() error {
	cfgCopy := cfg.clone()
	cfgCopy.Token = ""
	cfg = cfgCopy
	return saveConfigFile(cfgFile, cfgCopy)
}

func showUser(user models.User) {
	fmt.Printf("User ID: %s\n", user.ID)
	fmt.Printf("Username: %s\n", user.Username)
	fmt.Printf("Email: %s\n", user.Email)
	fmt.Printf("Created At: %s\n", user.CreatedAt.Format("2006-01-02 15:04:05 MST"))
}

func printTokenClaims(token string) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		fmt.Println("Token: (invalid JWT format)")
		return
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		fmt.Println("Token: (unable to decode payload)")
		return
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		fmt.Println("Token: (unable to parse payload)")
		return
	}
	if sub, ok := claims["sub"].(string); ok {
		fmt.Printf("Subject: %s\n", sub)
	}
	if userID, ok := claims["userID"].(string); ok {
		fmt.Printf("User ID: %s\n", userID)
	}
	if username, ok := claims["username"].(string); ok {
		fmt.Printf("Username: %s\n", username)
	}
	if exp, ok := claims["exp"].(float64); ok {
		fmt.Printf("Expires At (unix): %.0f\n", exp)
	}
}

func warnUnsupported(feature string) {
	fmt.Fprintf(os.Stdout, "%s is not implemented by the current HTTP API yet.\n", feature)
	fmt.Fprintln(os.Stdout, "This command is kept as a CLI placeholder for the final project scope.")
}

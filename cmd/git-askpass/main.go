package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	token := os.Getenv("GIT_TOKEN")
	if token == "" {
		fmt.Fprintf(os.Stderr, "GIT_TOKEN environment variable is required\n")
		os.Exit(1)
	}
	prompt := ""
	if len(os.Args) > 1 {
		prompt = os.Args[1]
	}
	// OAuth2-/Bearer-Tokens muessen bei git-over-https als Passwort mit dem
	// festen Benutzernamen "oauth2" verwendet werden.
	if os.Getenv("GIT_TOKEN_TYPE") == "Bearer" && strings.Contains(strings.ToLower(prompt), "username") {
		fmt.Print("oauth2")
		return
	}
	fmt.Print(token)
}

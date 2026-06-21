package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	gmailsvc "google.golang.org/api/gmail/v1"
)

// NewHTTPClient returns an authenticated *http.Client using the OAuth2 flow.
// On first run it prints a URL, reads an authorization code from stdin, saves
// token.json (chmod 600), and then returns the client. On subsequent runs it
// loads and refreshes token.json automatically.
func NewHTTPClient(ctx context.Context, credentialsFile, tokenFile string) (*http.Client, error) {
	b, err := os.ReadFile(credentialsFile)
	if err != nil {
		return nil, fmt.Errorf("reading credentials file %q: %w", credentialsFile, err)
	}

	cfg, err := google.ConfigFromJSON(b, gmailsvc.GmailModifyScope)
	if err != nil {
		return nil, fmt.Errorf("parsing credentials: %w", err)
	}

	tok, err := loadToken(tokenFile)
	if err != nil {
		// First run: interactive OAuth consent.
		tok, err = obtainToken(ctx, cfg, tokenFile)
		if err != nil {
			return nil, err
		}
	}

	return cfg.Client(ctx, tok), nil
}

// loadToken reads a cached token from disk.
func loadToken(path string) (*oauth2.Token, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var tok oauth2.Token
	if err := json.NewDecoder(f).Decode(&tok); err != nil {
		return nil, fmt.Errorf("decoding token: %w", err)
	}
	return &tok, nil
}

// obtainToken runs the interactive OAuth2 consent flow and persists the token.
func obtainToken(ctx context.Context, cfg *oauth2.Config, tokenFile string) (*oauth2.Token, error) {
	cfg.RedirectURL = "urn:ietf:wg:oauth:2.0:oob"
	authURL := cfg.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Open the following URL in your browser and authorize the application:\n\n%s\n\nEnter the authorization code: ", authURL)

	var code string
	if _, err := fmt.Scan(&code); err != nil {
		return nil, fmt.Errorf("reading authorization code: %w", err)
	}

	tok, err := cfg.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("exchanging auth code: %w", err)
	}

	if err := saveToken(tokenFile, tok); err != nil {
		return nil, err
	}
	return tok, nil
}

// saveToken writes the token to disk with 0600 permissions.
func saveToken(path string, tok *oauth2.Token) error {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("saving token to %q: %w", path, err)
	}
	defer f.Close()

	if err := json.NewEncoder(f).Encode(tok); err != nil {
		return fmt.Errorf("encoding token: %w", err)
	}
	return nil
}

package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/catalinlongevai/canvacli/internal/auth"
	"github.com/catalinlongevai/canvacli/internal/config"
	"github.com/spf13/cobra"
	"golang.org/x/oauth2"
)

const (
	canvaAuthURL  = "https://www.canva.com/api/oauth/authorize"
	canvaTokenURL = "https://api.canva.com/rest/v1/oauth/token"
)

var canvaScopes = []string{
	"design:meta:read",
	"design:content:read",
	"design:content:write",
	"brandtemplate:meta:read",
	"brandtemplate:content:read",
	"folder:read",
	"profile:read",
	"comment:read",
	"comment:write",
	"asset:read",
	"asset:write",
}

func NewLogin() *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "Authenticate with Canva via OAuth 2.0 PKCE",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cid := clientID()
			csec := clientSecret()
			if cid == "" || csec == "" {
				return errors.New("missing Canva developer credentials — this binary was built without embedded credentials. For local dev, set CANVA_CLIENT_ID and CANVA_CLIENT_SECRET env vars; for the official release, download the binary from https://github.com/catalinlongevai/canvacli/releases")
			}

			state := auth.NewState()
			cb, err := auth.StartCallbackServer(state, []int{8765, 8766, 8767})
			if err != nil {
				return err
			}
			defer cb.Close()

			conf := &oauth2.Config{
				ClientID:     cid,
				ClientSecret: csec,
				Endpoint: oauth2.Endpoint{
					AuthURL:  canvaAuthURL,
					TokenURL: canvaTokenURL,
				},
				RedirectURL: cb.RedirectURI(),
				Scopes:      canvaScopes,
			}

			verifier := oauth2.GenerateVerifier()
			authURL := conf.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier))

			fmt.Fprintln(os.Stderr, "Opening browser to authorize canvacli...")
			fmt.Fprintln(os.Stderr, "If it doesn't open, visit:", authURL)
			_ = auth.OpenBrowser(authURL)

			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Minute)
			defer cancel()
			code, err := cb.Wait(ctx)
			if err != nil {
				return fmt.Errorf("oauth callback: %w", err)
			}

			tok, err := conf.Exchange(ctx, code, oauth2.VerifierOption(verifier))
			if err != nil {
				return fmt.Errorf("token exchange: %w", err)
			}
			path, err := config.TokenPath()
			if err != nil {
				return err
			}
			if err := auth.SaveToken(path, tok); err != nil {
				return err
			}
			fmt.Fprintln(os.Stderr, "Logged in. Token stored at", path)
			return nil
		},
	}
}

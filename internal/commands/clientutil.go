package commands

import (
	"context"
	"errors"
	"net/http"
	"os"

	"github.com/catalinlongevai/canvacli/internal/api"
	"github.com/catalinlongevai/canvacli/internal/auth"
	"github.com/catalinlongevai/canvacli/internal/cache"
	"github.com/catalinlongevai/canvacli/internal/config"
	"golang.org/x/oauth2"
)

func loadClient(ctx context.Context) (*api.Client, error) {
	tokPath, err := config.TokenPath()
	if err != nil {
		return nil, err
	}
	tok, err := auth.LoadToken(tokPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.New("auth_required: run `canva login` first")
		}
		return nil, err
	}
	clientID := os.Getenv("CANVA_CLIENT_ID")
	clientSecret := os.Getenv("CANVA_CLIENT_SECRET")
	conf := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     oauth2.Endpoint{TokenURL: canvaTokenURL},
	}
	src := oauth2.ReuseTokenSource(tok, conf.TokenSource(ctx, tok))
	persisting := auth.NewPersistingSource(src, tokPath)

	httpClient := &http.Client{
		Transport: &auth.RefreshOn401Transport{
			Base:   &oauth2.Transport{Source: persisting, Base: http.DefaultTransport},
			Source: persisting,
		},
	}

	return api.NewClient(tok.AccessToken, api.WithHTTPClient(httpClient)), nil
}

func loadCache() (*cache.Cache, error) {
	p, err := config.CacheDBPath()
	if err != nil {
		return nil, err
	}
	return cache.Open(p)
}

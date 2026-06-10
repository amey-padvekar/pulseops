package main

import (
	"context"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/certainelf/pulseops/backend/internal/agentbuilder"
)

// newGoogleADCTokenProvider returns an agentbuilder.TokenProvider backed by
// Application Default Credentials with the cloud-platform scope. It works with the
// Cloud Run service account in production and `gcloud auth application-default
// login` locally. Tokens are cached and refreshed automatically.
func newGoogleADCTokenProvider(ctx context.Context) (agentbuilder.TokenProvider, error) {
	src, err := google.DefaultTokenSource(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return nil, err
	}
	reuse := oauth2.ReuseTokenSource(nil, src)
	return func(context.Context) (string, error) {
		tok, err := reuse.Token()
		if err != nil {
			return "", err
		}
		return tok.AccessToken, nil
	}, nil
}

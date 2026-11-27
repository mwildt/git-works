package gitlab

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"gtihub.com/mwildt/git-works/pkg/auth"
)

const tokenRefreshSkew = 60 * time.Second

type TokenProvider struct {
	manager *auth.Manager
	oidc    *auth.OIDCClient
}

func NewTokenProvider(manager *auth.Manager, oidc *auth.OIDCClient) *TokenProvider {
	return &TokenProvider{manager: manager, oidc: oidc}
}

// TokenFor liefert den g├╝ltigen GitLab-Token der Session. PAT-Sessions liefern
// ihren gespeicherten Token direkt; OIDC-Sessions refreshen den Access-Token
// serverseitig, sobald er ablaufen w├╝rde.
func (p *TokenProvider) TokenFor(ctx context.Context) (Token, error) {
	session, ok := auth.ReadSession(ctx)
	if !ok {
		return Token{}, errors.New("no session in context")
	}
	if token, ok := session.Value(TokenKey).(Token); ok && token.Value != "" {
		return token, nil
	}
	if !p.oidc.Enabled() {
		return Token{}, errors.New("oidc not configured and no token in session")
	}
	tokens, err := p.manager.RefreshOIDC(session.Id, func(current auth.OIDCTokens) (auth.OIDCTokens, error) {
		if time.Now().Add(tokenRefreshSkew).Before(current.Expiry) {
			slog.Default().Debug("oidc access token still valid", "username", session.Username, "expires_at", current.Expiry)
			return current, nil
		}
		slog.Default().Info("oidc access token expired, refreshing", "username", session.Username, "expired_at", current.Expiry)
		resp, err := p.oidc.Refresh(ctx, current.RefreshToken)
		if err != nil {
			slog.Default().Error("oidc token refresh failed", "username", session.Username, "error", err)
			return current, fmt.Errorf("refresh token: %w", err)
		}
		refreshToken := resp.RefreshToken
		if refreshToken == "" {
			refreshToken = current.RefreshToken
		}
		expiry := time.Now().Add(time.Duration(resp.ExpiresIn) * time.Second)
		if resp.ExpiresIn <= 0 {
			expiry = time.Now().Add(2 * time.Hour)
		}
		slog.Default().Info("oidc token refreshed", "username", session.Username, "expires_in", resp.ExpiresIn, "refresh_token_rotated", refreshToken != current.RefreshToken, "new_expiry", expiry)
		return auth.OIDCTokens{
			AccessToken:  resp.AccessToken,
			RefreshToken: refreshToken,
			Expiry:       expiry,
		}, nil
	})
	if err != nil {
		return Token{}, err
	}
	return Token{Value: tokens.AccessToken, TokenType: TokenTypeBearer}, nil
}

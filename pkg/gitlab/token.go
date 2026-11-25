package gitlab

import (
	"context"
	"errors"
	"fmt"
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
			return current, nil
		}
		resp, err := p.oidc.Refresh(ctx, current.RefreshToken)
		if err != nil {
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

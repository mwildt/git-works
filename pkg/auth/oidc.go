package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const DefaultOIDCScopes = "openid profile email api offline_access"

type OIDCConfig struct {
	ClientID      string
	ClientSecret  string
	Scopes        string
	RedirectURI   string
	PublicBaseURL string
}

func (c OIDCConfig) Enabled() bool {
	return c.ClientID != "" && c.ClientSecret != "" && c.RedirectURI != ""
}

type Discovery struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
	RevocationEndpoint    string `json:"revocation_endpoint"`
	JWKSURI               string `json:"jwks_uri"`
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	IDToken      string `json:"id_token"`
	Scope        string `json:"scope"`
}

type Userinfo struct {
	Sub               string `json:"sub"`
	Name              string `json:"name"`
	PreferredUsername string `json:"preferred_username"`
	Nickname          string `json:"nickname"`
	Email             string `json:"email"`
}

func (u Userinfo) Username() string {
	if u.PreferredUsername != "" {
		return u.PreferredUsername
	}
	if u.Nickname != "" {
		return u.Nickname
	}
	return u.Sub
}

type OIDCClient struct {
	issuer     string
	config     OIDCConfig
	httpClient *http.Client

	mu         sync.Mutex
	discovered *Discovery
}

func NewOIDCClient(issuer string, config OIDCConfig) *OIDCClient {
	if config.Scopes == "" {
		config.Scopes = DefaultOIDCScopes
	}
	return &OIDCClient{
		issuer:     strings.TrimSuffix(issuer, "/"),
		config:     config,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *OIDCClient) Enabled() bool {
	return c != nil && c.config.Enabled()
}

func (c *OIDCClient) discoveryDoc(ctx context.Context) (*Discovery, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.discovered != nil {
		return c.discovered, nil
	}
	docURL := c.issuer + "/.well-known/openid-configuration"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, docURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("oidc discovery: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("oidc discovery: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var doc Discovery
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return nil, fmt.Errorf("oidc discovery decode: %w", err)
	}
	c.discovered = &doc
	return c.discovered, nil
}

func NewPKCE() (verifier string, challenge string, err error) {
	b := make([]byte, 48)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	verifier = base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

func (c *OIDCClient) AuthorizeURL(ctx context.Context, state string, codeChallenge string) (string, error) {
	doc, err := c.discoveryDoc(ctx)
	if err != nil {
		return "", err
	}
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", c.config.ClientID)
	q.Set("redirect_uri", c.config.RedirectURI)
	q.Set("scope", c.config.Scopes)
	q.Set("state", state)
	q.Set("code_challenge", codeChallenge)
	q.Set("code_challenge_method", "S256")
	return doc.AuthorizationEndpoint + "?" + q.Encode(), nil
}

func (c *OIDCClient) postToken(ctx context.Context, form url.Values) (*TokenResponse, error) {
	doc, err := c.discoveryDoc(ctx)
	if err != nil {
		return nil, err
	}
	form.Set("client_id", c.config.ClientID)
	form.Set("client_secret", c.config.ClientSecret)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, doc.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("oidc token request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("oidc token endpoint: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("oidc token decode: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("oidc token endpoint returned no access_token")
	}
	return &tokenResp, nil
}

func (c *OIDCClient) ExchangeCode(ctx context.Context, code string, codeVerifier string) (*TokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", c.config.RedirectURI)
	form.Set("code_verifier", codeVerifier)
	return c.postToken(ctx, form)
}

func (c *OIDCClient) Refresh(ctx context.Context, refreshToken string) (*TokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	return c.postToken(ctx, form)
}

func (c *OIDCClient) Userinfo(ctx context.Context, accessToken string) (*Userinfo, error) {
	doc, err := c.discoveryDoc(ctx)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, doc.UserinfoEndpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("oidc userinfo request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("oidc userinfo: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var info Userinfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("oidc userinfo decode: %w", err)
	}
	return &info, nil
}

func (c *OIDCClient) Revoke(ctx context.Context, token string, tokenTypeHint string) error {
	doc, err := c.discoveryDoc(ctx)
	if err != nil {
		return err
	}
	if doc.RevocationEndpoint == "" {
		return nil
	}
	form := url.Values{}
	form.Set("token", token)
	if tokenTypeHint != "" {
		form.Set("token_type_hint", tokenTypeHint)
	}
	form.Set("client_id", c.config.ClientID)
	form.Set("client_secret", c.config.ClientSecret)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, doc.RevocationEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("oidc revoke request: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return nil
}

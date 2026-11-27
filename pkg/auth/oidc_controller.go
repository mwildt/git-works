package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"
	"time"

	"gtihub.com/mwildt/git-works/pkg/misc"
)

const (
	oidcStateCookieName    = "gw_oidc_state"
	oidcVerifierCookieName = "gw_oidc_verifier"
	oidcCookieMaxAge       = 600
)

func (ctrl *Controller) setOIDCFlowCookie(w http.ResponseWriter, name string, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   ctrl.secureCookies,
		MaxAge:   maxAge,
	})
}

func fingerprint(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:8]
}

func (ctrl *Controller) setUpOIDC(router *http.ServeMux) {

	router.HandleFunc("GET /api/auth/methods", func(w http.ResponseWriter, r *http.Request) {
		misc.SendJson(w, http.StatusOK, map[string]any{
			"token": true,
			"oidc":  ctrl.oidc.Enabled(),
		})
	})

	router.HandleFunc("GET /api/auth/oidc/login", func(w http.ResponseWriter, r *http.Request) {
		if !ctrl.oidc.Enabled() {
			slog.Default().Debug("oidc login requested but oidc is not configured")
			http.Error(w, "oidc not configured", http.StatusNotFound)
			return
		}
		slog.Default().Info("oidc login started", "remote", r.RemoteAddr)
		state, err := RandomString(32)
		if err != nil {
			slog.Default().Error("unable to create state", "error", err)
			http.Error(w, "unable to create state", http.StatusInternalServerError)
			return
		}
		verifier, challenge, err := NewPKCE()
		if err != nil {
			slog.Default().Error("unable to create pkce", "error", err)
			http.Error(w, "unable to create pkce", http.StatusInternalServerError)
			return
		}
		authorizeURL, err := ctrl.oidc.AuthorizeURL(r.Context(), state, challenge)
		if err != nil {
			slog.Default().Error("unable to build authorize url", "error", err)
			http.Error(w, "unable to build authorize url", http.StatusBadGateway)
			return
		}
		ctrl.setOIDCFlowCookie(w, oidcStateCookieName, state, oidcCookieMaxAge)
		ctrl.setOIDCFlowCookie(w, oidcVerifierCookieName, verifier, oidcCookieMaxAge)
		slog.Default().Info("oidc login redirect", "state", fingerprint(state), "redirect_uri", ctrl.oidc.RedirectURI(), "remote", r.RemoteAddr)
		http.Redirect(w, r, authorizeURL, http.StatusFound)
	})

	router.HandleFunc("GET /api/auth/oidc/callback", func(w http.ResponseWriter, r *http.Request) {
		if !ctrl.oidc.Enabled() {
			slog.Default().Debug("oidc callback received but oidc is not configured")
			http.Error(w, "oidc not configured", http.StatusNotFound)
			return
		}
		queryState := r.URL.Query().Get("state")
		slog.Default().Info("oidc callback received", "remote", r.RemoteAddr, "state", fingerprint(queryState), "has_code", r.URL.Query().Get("code") != "")
		if errParam := r.URL.Query().Get("error"); errParam != "" {
			slog.Default().Warn("oidc callback error", "error", errParam, "description", r.URL.Query().Get("error_description"), "state", fingerprint(queryState))
			http.Error(w, "oidc provider returned an error: "+errParam, http.StatusBadRequest)
			return
		}
		stateCookie, err := r.Cookie(oidcStateCookieName)
		cookieState := ""
		if stateCookie != nil {
			cookieState = stateCookie.Value
		}
		if err != nil || cookieState == "" || cookieState != queryState {
			slog.Default().Warn("oidc state mismatch",
				"cookie_present", cookieState != "",
				"query_state", fingerprint(queryState),
				"cookie_state", fingerprint(cookieState),
				"remote", r.RemoteAddr)
			http.Error(w, "invalid state", http.StatusBadRequest)
			return
		}
		slog.Default().Debug("oidc state valid", "state", fingerprint(queryState))
		verifierCookie, err := r.Cookie(oidcVerifierCookieName)
		if err != nil || verifierCookie.Value == "" {
			slog.Default().Warn("oidc pkce verifier cookie missing", "state", fingerprint(queryState), "remote", r.RemoteAddr)
			http.Error(w, "missing pkce verifier", http.StatusBadRequest)
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			slog.Default().Warn("oidc callback missing code", "state", fingerprint(queryState))
			http.Error(w, "missing code", http.StatusBadRequest)
			return
		}

		ctrl.setOIDCFlowCookie(w, oidcStateCookieName, "", -1)
		ctrl.setOIDCFlowCookie(w, oidcVerifierCookieName, "", -1)
		slog.Default().Debug("oidc flow cookies cleared", "state", fingerprint(queryState))

		tokenResp, err := ctrl.oidc.ExchangeCode(r.Context(), code, verifierCookie.Value)
		if err != nil {
			slog.Default().Error("oidc code exchange failed", "error", err, "state", fingerprint(queryState))
			http.Error(w, "unable to exchange code", http.StatusBadGateway)
			return
		}
		slog.Default().Info("oidc code exchanged", "state", fingerprint(queryState), "token_type", tokenResp.TokenType, "expires_in", tokenResp.ExpiresIn, "scope", tokenResp.Scope, "has_refresh_token", tokenResp.RefreshToken != "")
		userinfo, err := ctrl.oidc.Userinfo(r.Context(), tokenResp.AccessToken)
		if err != nil {
			slog.Default().Error("oidc userinfo failed", "error", err, "state", fingerprint(queryState))
			http.Error(w, "unable to load userinfo", http.StatusBadGateway)
			return
		}
		username := userinfo.Username()
		if username == "" {
			slog.Default().Error("oidc userinfo has no username", "sub", userinfo.Sub, "state", fingerprint(queryState))
			http.Error(w, "unable to determine username", http.StatusBadGateway)
			return
		}
		slog.Default().Info("oidc userinfo received", "username", username, "sub", userinfo.Sub, "state", fingerprint(queryState))

		session, err := ctrl.manager.CreateSession(username, AuthMethodOIDC)
		if err != nil {
			slog.Default().Error("unable to create session", "error", err, "username", username)
			http.Error(w, "unable to create session", http.StatusInternalServerError)
			return
		}
		expiry := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
		if tokenResp.ExpiresIn <= 0 {
			expiry = time.Now().Add(2 * time.Hour)
		}
		ctrl.manager.SetOIDC(session.Id, OIDCTokens{
			AccessToken:  tokenResp.AccessToken,
			RefreshToken: tokenResp.RefreshToken,
			Expiry:       expiry,
		})

		slog.Default().Info("oidc login success", "username", session.Username, "auth_method", AuthMethodOIDC, "state", fingerprint(queryState), "token_expiry", expiry)
		http.SetCookie(w, &http.Cookie{
			Name:     SessionCookieName,
			Value:    session.Id,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			Secure:   ctrl.secureCookies,
			MaxAge:   int(sessionTTL.Seconds()),
		})
		http.Redirect(w, r, "/", http.StatusFound)
	})
}

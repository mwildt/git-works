package auth

import (
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

func (ctrl *Controller) setUpOIDC(router *http.ServeMux) {

	router.HandleFunc("GET /api/auth/methods", func(w http.ResponseWriter, r *http.Request) {
		misc.SendJson(w, http.StatusOK, map[string]any{
			"token": true,
			"oidc":  ctrl.oidc.Enabled(),
		})
	})

	router.HandleFunc("GET /api/auth/oidc/login", func(w http.ResponseWriter, r *http.Request) {
		if !ctrl.oidc.Enabled() {
			http.Error(w, "oidc not configured", http.StatusNotFound)
			return
		}
		state, err := RandomString(32)
		if err != nil {
			http.Error(w, "unable to create state", http.StatusInternalServerError)
			return
		}
		verifier, challenge, err := NewPKCE()
		if err != nil {
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
		slog.Default().Info("oidc login redirect", "url", authorizeURL)
		http.Redirect(w, r, authorizeURL, http.StatusFound)
	})

	router.HandleFunc("GET /api/auth/oidc/callback", func(w http.ResponseWriter, r *http.Request) {
		if !ctrl.oidc.Enabled() {
			http.Error(w, "oidc not configured", http.StatusNotFound)
			return
		}
		if errParam := r.URL.Query().Get("error"); errParam != "" {
			slog.Default().Warn("oidc callback error", "error", errParam, "description", r.URL.Query().Get("error_description"))
			http.Error(w, "oidc provider returned an error: "+errParam, http.StatusBadRequest)
			return
		}
		stateCookie, err := r.Cookie(oidcStateCookieName)
		if err != nil || stateCookie.Value == "" || stateCookie.Value != r.URL.Query().Get("state") {
			http.Error(w, "invalid state", http.StatusBadRequest)
			return
		}
		verifierCookie, err := r.Cookie(oidcVerifierCookieName)
		if err != nil || verifierCookie.Value == "" {
			http.Error(w, "missing pkce verifier", http.StatusBadRequest)
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "missing code", http.StatusBadRequest)
			return
		}

		ctrl.setOIDCFlowCookie(w, oidcStateCookieName, "", -1)
		ctrl.setOIDCFlowCookie(w, oidcVerifierCookieName, "", -1)

		tokenResp, err := ctrl.oidc.ExchangeCode(r.Context(), code, verifierCookie.Value)
		if err != nil {
			slog.Default().Error("oidc code exchange failed", "error", err)
			http.Error(w, "unable to exchange code", http.StatusBadGateway)
			return
		}
		userinfo, err := ctrl.oidc.Userinfo(r.Context(), tokenResp.AccessToken)
		if err != nil {
			slog.Default().Error("oidc userinfo failed", "error", err)
			http.Error(w, "unable to load userinfo", http.StatusBadGateway)
			return
		}
		username := userinfo.Username()
		if username == "" {
			http.Error(w, "unable to determine username", http.StatusBadGateway)
			return
		}

		session, err := ctrl.manager.CreateSession(username, AuthMethodOIDC)
		if err != nil {
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

		slog.Default().Info("oidc login success", "username", session.Username)
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

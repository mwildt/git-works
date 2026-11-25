package auth

import (
	"log/slog"
	"net/http"

	"gtihub.com/mwildt/git-works/pkg/misc"
)

const SessionCookieName = "gw_session"

type Controller struct {
	manager       *Manager
	oidc          *OIDCClient
	secureCookies bool
}

func NewController(manager *Manager, oidc *OIDCClient, secureCookies bool) *Controller {
	return &Controller{manager: manager, oidc: oidc, secureCookies: secureCookies}
}

func (ctrl *Controller) SetUp(router *http.ServeMux) {

	ctrl.setUpOIDC(router)

	router.HandleFunc("GET /api/auth/logout", ctrl.manager.Authenticated(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, ok := ReadSession(r.Context())
		if !ok {
			http.Error(w, "invalid JSON", http.StatusUnauthorized)
			return
		}

		if session.AuthMethod == AuthMethodOIDC && ctrl.oidc.Enabled() {
			if tokens, ok := ctrl.manager.OIDCTokens(session.Id); ok {
				if err := ctrl.oidc.Revoke(r.Context(), tokens.AccessToken, "access_token"); err != nil {
					slog.Default().Warn("oidc access token revoke failed", "error", err)
				}
				if tokens.RefreshToken != "" {
					if err := ctrl.oidc.Revoke(r.Context(), tokens.RefreshToken, "refresh_token"); err != nil {
						slog.Default().Warn("oidc refresh token revoke failed", "error", err)
					}
				}
			}
		}

		ctrl.manager.Delete(session.Id)

		http.SetCookie(w, &http.Cookie{
			Name:     SessionCookieName,
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			Secure:   ctrl.secureCookies,
			MaxAge:   -1,
		})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)

	})))

	router.HandleFunc("GET /api/user", ctrl.manager.Authenticated(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, ok := ReadSession(r.Context())
		if !ok {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		body := struct {
			Username string `json:"username"`
		}{
			Username: session.Username,
		}
		misc.SendJson(w, http.StatusOK, body)
	})))

}

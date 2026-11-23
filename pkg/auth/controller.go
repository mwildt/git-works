package auth

import (
	"net/http"

	"gtihub.com/mwildt/git-works/pkg/misc"
)

const SessionCookieName = "gw_session"

type Controller struct {
	manager *Manager
}

func NewController(manager *Manager) *Controller {
	return &Controller{manager}
}

func (ctrl *Controller) SetUp(router *http.ServeMux) {

	router.HandleFunc("GET /api/auth/logout", ctrl.manager.Authenticated(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, ok := ReadSession(r.Context())
		if !ok {
			http.Error(w, "invalid JSON", http.StatusUnauthorized)
			return
		}

		ctrl.manager.Delete(session.Id)

		http.SetCookie(w, &http.Cookie{
			Name:     SessionCookieName,
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
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

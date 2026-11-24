package gitlab

import (
	"log/slog"
	"net/http"
	"strconv"

	"gtihub.com/mwildt/git-works/pkg/auth"
	"gtihub.com/mwildt/git-works/pkg/misc"
)

type Controller struct {
	sessionManager *auth.Manager
	gitlabClient   *Client
	tokenProvider  *TokenProvider
}

func NewController(gitlabClient *Client, sessionManager *auth.Manager, tokenProvider *TokenProvider) *Controller {
	return &Controller{
		sessionManager: sessionManager,
		gitlabClient:   gitlabClient,
		tokenProvider:  tokenProvider,
	}
}

func (ctrl *Controller) SetUp(router *http.ServeMux) {

	router.HandleFunc("GET /api/gitlab/project", ctrl.sessionManager.Authenticated(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		token, err := ctrl.tokenProvider.TokenFor(r.Context())
		if err != nil {
			slog.Default().Warn("unable to resolve token", "error", err)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		search := r.URL.Query().Get("search")
		page := 1
		if raw := r.URL.Query().Get("page"); raw != "" {
			if parsed, err := strconv.Atoi(raw); err == nil {
				page = parsed
			}
		}

		result, err := ctrl.gitlabClient.SearchProjects(token, search, page)
		if err != nil {
			slog.Default().Warn("gitlab projects not found")
			http.Error(w, "gitlab projects not found", http.StatusBadGateway)
			return
		}

		misc.SendJson(w, http.StatusOK, result)
	})))

	router.HandleFunc("GET /api/gitlab/project/{projectid}/branches", ctrl.sessionManager.Authenticated(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, err := ctrl.tokenProvider.TokenFor(r.Context())
		if err != nil {
			slog.Default().Warn("unable to resolve token", "error", err)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		projectid := r.PathValue("projectid")

		branches, err := ctrl.gitlabClient.ListBranches(token, projectid)
		if err != nil {
			slog.Default().Error("unable to load branches", "err", err)
			http.Error(w, "unable to load branches", http.StatusBadGateway)
			return
		}

		misc.SendJson(w, http.StatusOK, branches)
	})))

}

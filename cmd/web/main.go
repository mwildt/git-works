package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path"
	"strconv"
	"time"
	"uuid"

	"gtihub.com/mwildt/git-works/pkg/actions"
	"gtihub.com/mwildt/git-works/pkg/auth"
	"gtihub.com/mwildt/git-works/pkg/config"
	"gtihub.com/mwildt/git-works/pkg/git"
	"gtihub.com/mwildt/git-works/pkg/gitlab"
	workspaces "gtihub.com/mwildt/git-works/pkg/workspace"
)

type configuration struct {
	Host string
}

func main() {

	appConfig := &configuration{
		Host: config.GetEnv("SERVER_ADDRESS", "0.0.0.0:8080"),
	}

	opencodeActions, err := actions.NewHttpActions(config.GetEnv("GITHUB_ACTIONS_FILE", "gitlab-http-actions.yml"))
	if err != nil {
		log.Fatal(err)
	}

	gitlabClient := gitlab.NewClient(config.GetEnv("GITLAB_BASE_URL", "https://gitlab.com"))
	sessionManager := auth.NewSessionManager(gitlabClient)
	workspaceManager := workspaces.NewManager()

	gitClient := git.NewGit()

	handler := http.NewServeMux()

	opencode := http.NewServeMux()

	opencode.HandleFunc("PATCH /.git-works/session-mode", func(writer http.ResponseWriter, request *http.Request) {
		cookie, err := request.Cookie("gw_session")
		if err != nil {
			slog.Default().Warn("cookie not found")
			http.Error(writer, "invalid cookie", http.StatusUnauthorized)
			return
		}
		if err := sessionManager.ActivateWorkspace(cookie.Value, uuid.Nil()); err != nil {
			slog.Default().Warn("cookie not found")
			http.Error(writer, "invalid cookie", http.StatusBadRequest)
			return
		}
		writer.WriteHeader(http.StatusAccepted)
	})

	opencode.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {

		cookie, err := request.Cookie("gw_session")
		if err != nil {
			slog.Default().Warn("cookie not found")
			http.Error(writer, "invalid cookie", http.StatusUnauthorized)
			return
		}
		session, _ := sessionManager.Validate(cookie.Value)

		workspace, ok := workspaceManager.ByIdAndOwner(session.ActiveWorkspace, session.Username)
		if !ok {
			slog.Default().Warn("workspace not found")
			http.Error(writer, "workspace not found", http.StatusNotFound)
			return
		}
		request.Header.Set("Authorization", fmt.Sprintf("Basic %s", workspace.Opencode.BasicAuth()))
		target, _ := url.Parse(workspace.Opencode.TargetUrl())
		proxy := httputil.NewSingleHostReverseProxy(target)
		proxy.ServeHTTP(writer, request)
		return
	})

	opencode.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, path.Join("./static/opencode", "index.html"))
	})

	opencode.Handle("GET /.gw-mixin/", http.FileServer(http.Dir("./static/opencode/")))

	handler.HandleFunc("PATCH /.git-works/session-mode", func(writer http.ResponseWriter, request *http.Request) {
		wsid, err := uuid.Parse(request.URL.Query().Get("wsid"))
		if err != nil {
			http.Error(writer, "invalid wsid", http.StatusBadRequest)
			return
		}
		cookie, err := request.Cookie("gw_session")
		if err != nil {
			slog.Default().Warn("cookie not found")
			http.Error(writer, "invalid cookie", http.StatusUnauthorized)
			return
		}
		if err := sessionManager.ActivateWorkspace(cookie.Value, wsid); err != nil {
			slog.Default().Warn("cookie not found")
			http.Error(writer, "invalid cookie", http.StatusBadRequest)
			return
		}
		writer.WriteHeader(http.StatusAccepted)
	})

	srv := &http.Server{
		Addr: appConfig.Host,
		Handler: withTraceLog(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if cookie, err := r.Cookie("gw_session"); err == http.ErrNoCookie {
				handler.ServeHTTP(w, r)
			} else if session, err := sessionManager.Validate(cookie.Value); err != nil {
				handler.ServeHTTP(w, r)
			} else if session.ActiveWorkspace != uuid.Nil() {
				// hier noch einen healthcheck ausführen ??
				opencode.ServeHTTP(w, r)
			} else {
				handler.ServeHTTP(w, r)
			}
		})),
		DisableGeneralOptionsHandler: false,
		TLSConfig:                    nil,
		ReadTimeout:                  30 * time.Second,
		ReadHeaderTimeout:            10 * time.Second,
		WriteTimeout:                 30 * time.Second,
		IdleTimeout:                  120 * time.Second,
	}

	authenticated := func(handler http.Handler) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {

			cookie, err := r.Cookie("gw_session")
			if err != nil {
				slog.Default().Warn("cookie not found")
				http.Error(w, "invalid cookie", http.StatusUnauthorized)
				return
			}
			_, err = sessionManager.Validate(cookie.Value)
			if err != nil {
				slog.Default().Warn("session is invalid")
				http.Error(w, "invalid cookie", http.StatusUnauthorized)
				return
			}
			handler.ServeHTTP(w, r)
		}
	}

	api := http.NewServeMux()

	type WorkspaceResponse struct {
		Id       uuid.UUID            `json:"id"`
		Owner    string               `json:"owner"`
		Name     string               `json:"name"`
		Created  string               `json:"created"`
		Projects []workspaces.Project `json:"projects"`
	}

	api.HandleFunc("POST /api/workspace", func(w http.ResponseWriter, r *http.Request) {
		cookie, _ := r.Cookie("gw_session")
		session, _ := sessionManager.Validate(cookie.Value)

		workspace := workspaceManager.Create(session.Username)
		response := WorkspaceResponse{
			Id:      workspace.Id,
			Name:    workspace.Name,
			Owner:   workspace.Owner,
			Created: workspace.Created.Format(time.RFC3339),
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)

		if err := json.NewEncoder(w).Encode(response); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	api.HandleFunc("GET /api/workspace", func(w http.ResponseWriter, r *http.Request) {
		cookie, _ := r.Cookie("gw_session")
		session, _ := sessionManager.Validate(cookie.Value)

		response := []WorkspaceResponse{}
		for _, w := range workspaceManager.AllByOwner(session.Username) {
			response = append(response, WorkspaceResponse{
				Id:      w.Id,
				Name:    w.Name,
				Owner:   w.Owner,
				Created: w.Created.Format(time.RFC3339),
			})
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if err := json.NewEncoder(w).Encode(response); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	api.HandleFunc("GET /api/workspace/{wsId}", func(w http.ResponseWriter, r *http.Request) {
		wsId, err := uuid.Parse(r.PathValue("wsId"))
		if err != nil {
			slog.Default().Warn("invalid ws id")
			http.Error(w, "invalid ws id", http.StatusBadRequest)
			return
		}
		cookie, _ := r.Cookie("gw_session")
		session, _ := sessionManager.Validate(cookie.Value)

		workspace, ok := workspaceManager.ByIdAndOwner(wsId, session.Username)
		if !ok {
			slog.Default().Warn("workspace not found")
			http.Error(w, "workspace not found", http.StatusNotFound)
			return
		}
		response := WorkspaceResponse{
			Id:       workspace.Id,
			Name:     workspace.Name,
			Owner:    workspace.Owner,
			Created:  workspace.Created.Format(time.RFC3339),
			Projects: workspace.Projects,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if err := json.NewEncoder(w).Encode(response); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	handler.HandleFunc("POST /api/workspace/{wsId}/activate", func(writer http.ResponseWriter, request *http.Request) {
		wsid, err := uuid.Parse(request.PathValue("wsId"))
		if err != nil {
			http.Error(writer, "invalid wsid", http.StatusBadRequest)
			return
		}
		cookie, err := request.Cookie("gw_session")
		if err != nil {
			slog.Default().Warn("cookie not found")
			http.Error(writer, "invalid cookie", http.StatusUnauthorized)
			return
		}
		if err := sessionManager.ActivateWorkspace(cookie.Value, wsid); err != nil {
			slog.Default().Warn("unable to activate workspace", "wsid", wsid.String(), "err", err)
			http.Error(writer, "invalid cookie", http.StatusInternalServerError)
			return
		}
		writer.WriteHeader(http.StatusAccepted)
	})

	api.HandleFunc("POST /api/workspace/{wsId}/oc-up", func(w http.ResponseWriter, r *http.Request) {
		wsId, err := uuid.Parse(r.PathValue("wsId"))
		if err != nil {
			slog.Default().Warn("invalid ws id")
			http.Error(w, "invalid ws id", http.StatusBadRequest)
			return
		}
		cookie, _ := r.Cookie("gw_session")
		session, _ := sessionManager.Validate(cookie.Value)

		workspace, ok := workspaceManager.ByIdAndOwner(wsId, session.Username)
		if !ok {
			slog.Default().Warn("workspace not found")
			http.Error(w, "workspace not found", http.StatusNotFound)
			return
		}

		err = workspace.Start(opencodeActions)
		if err != nil {
			slog.Default().Warn("workspace not found")
			http.Error(w, "workspace not found", http.StatusBadGateway)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)

	})

	api.HandleFunc("GET /api/workspace/{wsId}/status", func(w http.ResponseWriter, r *http.Request) {
		wsId, err := uuid.Parse(r.PathValue("wsId"))
		if err != nil {
			slog.Default().Warn("invalid ws id")
			http.Error(w, "invalid ws id", http.StatusBadRequest)
			return
		}
		cookie, _ := r.Cookie("gw_session")
		session, _ := sessionManager.Validate(cookie.Value)

		workspace, ok := workspaceManager.ByIdAndOwner(wsId, session.Username)
		if !ok {
			slog.Default().Warn("workspace not found")
			http.Error(w, "workspace not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		response := struct {
			WorkspaceId uuid.UUID `json:"workspaceId"`
			Status      string    `json:"status"`
		}{
			WorkspaceId: workspace.Id,
			Status: func() string {
				res, err := opencodeActions.Send(context.Background(), "OPENCODE_WORKSPACE_HEALTH", map[string]string{
					"port":             strconv.Itoa(workspace.Opencode.Port),
					"oc_authorization": fmt.Sprintf("Basic %s", workspace.Opencode.BasicAuth()),
				})
				if err != nil {
					slog.Default().Warn("unable to run OPENCODE_WORKSPACE_HEALTH", "error", err.Error())
					return "down"
				} else if res.StatusCode != 200 {
					body, _ := io.ReadAll(res.Body)
					slog.Default().Warn("status != 200 when running OPENCODE_WORKSPACE_HEALTH", "status", res.StatusCode, "code", res.StatusCode, "body", string(body))
					return "down"
				}
				return "up"
			}(),
		}

		if err := json.NewEncoder(w).Encode(response); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	api.HandleFunc("DELETE /api/workspace/{wsId}", func(w http.ResponseWriter, r *http.Request) {
		wsId, err := uuid.Parse(r.PathValue("wsId"))
		if err != nil {
			slog.Default().Warn("invalid ws id")
			http.Error(w, "invalid ws id", http.StatusBadRequest)
			return
		}
		cookie, _ := r.Cookie("gw_session")
		session, _ := sessionManager.Validate(cookie.Value)

		ok := workspaceManager.DeleteByIdAndOwner(wsId, session.Username)
		if !ok {
			slog.Default().Warn("workspace not found")
			http.Error(w, "workspace not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
	})

	api.HandleFunc("POST /api/workspace/{wsId}/init", func(w http.ResponseWriter, r *http.Request) {
		wsId, err := uuid.Parse(r.PathValue("wsId"))
		if err != nil {
			slog.Default().Warn("invalid ws id")
			http.Error(w, "invalid ws id", http.StatusBadRequest)
			return
		}
		cookie, _ := r.Cookie("gw_session")
		session, _ := sessionManager.Validate(cookie.Value)

		ws, ok := workspaceManager.ByIdAndOwner(wsId, session.Username)
		if !ok {
			slog.Default().Warn("workspace not found")
			http.Error(w, "workspace not found", http.StatusNotFound)
			return
		}

		go func() {
			err := ws.Init(session.Token, gitClient)
			if err != nil {
				slog.Default().Warn("workspace init err", "error", err)
			}
		}()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
	})

	api.HandleFunc("POST /api/workspace/{wsId}/project", func(writer http.ResponseWriter, request *http.Request) {
		var project struct {
			Path    string `json:"path"`
			Project struct {
				GitUrl string `json:"http_url_to_repo"`
			} `json:"project"`
			Branch struct {
				Name string `json:"name"`
			} `json:"branch"`
		}

		err := json.NewDecoder(request.Body).Decode(&project)
		if err != nil {
			http.Error(writer, "invalid JSON", http.StatusBadRequest)
			return
		}
		wsId, err := uuid.Parse(request.PathValue("wsId"))
		if err != nil {
			slog.Default().Warn("invalid ws id")
			http.Error(writer, "invalid ws id", http.StatusBadRequest)
			return
		}

		cookie, _ := request.Cookie("gw_session")
		session, _ := sessionManager.Validate(cookie.Value)

		workspace, ok := workspaceManager.ByIdAndOwner(wsId, session.Username)
		if !ok {
			slog.Default().Warn("workspace not found")
			http.Error(writer, "workspace not found", http.StatusNotFound)
			return
		}

		workspace.AddProject(project.Path, project.Project.GitUrl, project.Branch.Name)
		writer.WriteHeader(http.StatusCreated)
	})

	api.HandleFunc("DELETE /api/workspace/{wsId}/project/{projectId}", func(writer http.ResponseWriter, request *http.Request) {
		wsId, err := uuid.Parse(request.PathValue("wsId"))
		if err != nil {
			slog.Default().Warn("invalid ws id")
			http.Error(writer, "invalid ws id", http.StatusBadRequest)
			return
		}
		projectId, err := uuid.Parse(request.PathValue("projectId"))
		if err != nil {
			slog.Default().Warn("invalid projectId")
			http.Error(writer, "invalid projectId", http.StatusBadRequest)
			return
		}

		cookie, _ := request.Cookie("gw_session")
		session, _ := sessionManager.Validate(cookie.Value)

		workspace, ok := workspaceManager.ByIdAndOwner(wsId, session.Username)
		if !ok {
			slog.Default().Warn("workspace not found")
			http.Error(writer, "workspace not found", http.StatusNotFound)
			return
		}
		workspace.DeleteProjectById(projectId)
	})

	api.HandleFunc("GET /api/workspace/{wsId}/project/{projectId}/status", func(writer http.ResponseWriter, request *http.Request) {
		wsId, err := uuid.Parse(request.PathValue("wsId"))
		if err != nil {
			slog.Default().Warn("invalid ws id")
			http.Error(writer, "invalid ws id", http.StatusBadRequest)
			return
		}
		projectId, err := uuid.Parse(request.PathValue("projectId"))
		if err != nil {
			slog.Default().Warn("invalid projectId")
			http.Error(writer, "invalid projectId", http.StatusBadRequest)
			return
		}

		cookie, _ := request.Cookie("gw_session")
		session, _ := sessionManager.Validate(cookie.Value)

		status, err := workspaceManager.GetProjectStatus(session.Username, wsId, projectId)
		if err != nil {
			slog.Default().Warn("cannot get project state")
			http.Error(writer, "cannot get project state", http.StatusBadRequest)
			return
		}

		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(writer).Encode(status); err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
		}

	})

	api.HandleFunc("GET /api/gitlab/project", func(w http.ResponseWriter, r *http.Request) {
		cookie, _ := r.Cookie("gw_session")
		session, _ := sessionManager.Validate(cookie.Value)

		projects, err := gitlabClient.ListProjects(session.Token)
		if err != nil {
			slog.Default().Warn("gitlab projects not found")
			http.Error(w, "gitlab projects not found", http.StatusBadGateway)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(projects); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	api.HandleFunc("GET /api/gitlab/project/{projectid}/branches", func(w http.ResponseWriter, r *http.Request) {
		projectid := r.PathValue("projectid")

		cookie, _ := r.Cookie("gw_session")
		session, _ := sessionManager.Validate(cookie.Value)

		branches, err := gitlabClient.ListBranches(session.Token, projectid)
		if err != nil {
			slog.Default().Error("unable to load branches", "err", err)
			http.Error(w, "unable to load branches", http.StatusBadGateway)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(branches); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	api.HandleFunc("GET /api/user", func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("gw_session")
		if err != nil {
			http.Error(w, "invalid cookie", http.StatusUnauthorized)
			return
		}
		session, err := sessionManager.Validate(cookie.Value)
		if err != nil {
			http.Error(w, "invalid cookie", http.StatusUnauthorized)
			return
		}
		body := struct {
			Username string `json:"username"`
		}{
			Username: session.Username,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(body); err != nil {
			slog.Default().Error("json encode failed", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	handler.Handle("/api/", authenticated(api))

	handler.HandleFunc("POST /api/auth/token", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Token string `json:"token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		session, err := sessionManager.CreateSession(gitlab.Token{
			Value:     body.Token,
			TokenType: "PRIVATE-TOKEN",
		})
		if err != nil {
			http.Error(w, "invalid JSON", http.StatusUnauthorized)
			return
		}
		slog.Default().Info("token introspection success", "username", session.Username)
		http.SetCookie(w, &http.Cookie{
			Name:     "gw_session",
			Value:    session.Id,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   int(12 * time.Hour.Seconds()),
		})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
	})

	handler.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	handler.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static/git-works"))))

	handler.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, path.Join("./static/git-works", "index.html"))
	})

	slog.Default().Info("starting server", slog.String("host", appConfig.Host))
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Default().Error("server error", "error", err)
		os.Exit(1)
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func withTraceLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		slog.Default().Info("http request", "method", r.Method, "path", r.URL.Path, "status", recorder.status, "duration", time.Since(start).String(), "remote", r.RemoteAddr)
	})
}

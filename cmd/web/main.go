package main

import (
	"bytes"
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
	"strings"
	"time"

	"gtihub.com/mwildt/git-works/pkg/actions"
	"gtihub.com/mwildt/git-works/pkg/auth"
	"gtihub.com/mwildt/git-works/pkg/config"
	"gtihub.com/mwildt/git-works/pkg/git"
	"gtihub.com/mwildt/git-works/pkg/gitlab"
	"gtihub.com/mwildt/git-works/pkg/misc"
	workspaces "gtihub.com/mwildt/git-works/pkg/workspace"
)

type configuration struct {
	Host string
}

func main() {

	appConfig := &configuration{
		Host: config.GetEnv("SERVER_ADDRESS", "0.0.0.0:8080"),
	}

	opencodeActions, err := actions.NewHttpActions(config.GetEnv("GITLAB_ACTIONS_FILE", "gitlab-http-actions.yml"))

	slog.Info("startup-info",
		"SERVER_ADDRESS", os.Getenv("SERVER_ADDRESS"),
		"DATA_DIR", os.Getenv("DATA_DIR"),
		"GITLAB_ACTIONS_FILE", os.Getenv("GITLAB_ACTIONS_FILE"),
		"GITLAB_BASE_URL", os.Getenv("GITLAB_BASE_URL"),
		"SSH_ASKPASS", os.Getenv("SSH_ASKPASS"),
		"OPENCODE_REMOTE_HOST", os.Getenv("OPENCODE_REMOTE_HOST"),
	)

	if err != nil {
		log.Fatal(err)
	}

	gitlabClient := gitlab.NewClient(config.GetEnv("GITLAB_BASE_URL", "https://gitlab.com"))
	sessionManager := auth.NewSessionManager(gitlabClient)
	//workspaceManager, err := workspaces.NewManager()
	if err != nil {
		log.Fatal(err)
	}

	gitClient := git.NewGit()

	handler := http.NewServeMux()

	opencode := http.NewServeMux()

	workspaceRepo, err := workspaces.NewRepository()
	if err != nil {
		log.Fatal(err)
	}
	workspaceService := workspaces.NewService(workspaceRepo, gitClient, opencodeActions)
	workspaceController, err := workspaces.NewController(workspaceRepo, workspaceService, sessionManager)
	api := http.NewServeMux()

	workspaceController.SetUpApi(api)

	authenticated := func(handler http.Handler) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {

			cookie, err := r.Cookie("gw_session")
			if err != nil {
				slog.Default().Warn("cookie not found")
				http.Error(w, "invalid cookie", http.StatusUnauthorized)
				return
			}
			session, err := sessionManager.Validate(cookie.Value)
			if err != nil {
				slog.Default().Warn("session is invalid")
				http.Error(w, "invalid cookie", http.StatusUnauthorized)
				return
			}
			r = r.WithContext(auth.WithSession(r.Context(), session))
			handler.ServeHTTP(w, r)
		}
	}

	opencode.HandleFunc("PATCH /.git-works/session-mode", func(writer http.ResponseWriter, request *http.Request) {
		cookie, err := request.Cookie("gw_session")
		if err != nil {
			slog.Default().Warn("cookie not found")
			http.Error(writer, "invalid cookie", http.StatusUnauthorized)
			return
		}

		var location struct {
			Location string `json:"location"`
		}

		session, ok := auth.ReadSession(request.Context())
		if ok {
			location.Location = session.ActiveWorkload.Location
		}

		if err := sessionManager.ActivateWorkspace(cookie.Value, auth.WorkloadReference{}); err != nil {
			slog.Default().Warn("cookie not found")
			http.Error(writer, "invalid cookie", http.StatusBadRequest)
			return
		}

		misc.SendJson(writer, http.StatusAccepted, location)

	})

	opencode.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {

		session, ok := auth.ReadSession(request.Context())

		if !ok {
			http.Error(writer, "not found", http.StatusForbidden)
			return
		}

		_, workload, err := workspaceRepo.WorkloadById(request.Context(), session.ActiveWorkload.WorkspaceId, session.ActiveWorkload.WorkloadId)
		if err != nil {
			slog.Default().Warn("workspace not found")
			http.Error(writer, "not found", http.StatusBadRequest)
			return
		}
		request.Header.Set("Authorization", fmt.Sprintf("Basic %s", workload.Data.BasicAuth()))
		target, err := url.Parse(workload.TargetUrl())
		if err != nil {
			slog.Default().Warn("invalid workload target url", "error", err)
			http.Error(writer, "invalid workload target url", http.StatusBadGateway)
			return
		}
		proxy := httputil.NewSingleHostReverseProxy(target)
		proxy.FlushInterval = -1

		proxy.ModifyResponse = func(resp *http.Response) error {
			contentType := resp.Header.Get("Content-Type")
			if !strings.Contains(contentType, "text/html") {
				return nil
			}
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()

			const injectTag = "<script type=\"module\" crossorigin src=\"/.gw-mixin/index.js\" defer></script>"
			lower := bytes.ToLower(body)
			idx := bytes.LastIndex(lower, []byte("</body>"))
			if idx == -1 {
				resp.Body = io.NopCloser(bytes.NewReader(body))
				resp.ContentLength = int64(len(body))
				resp.Header.Set("Content-Length", fmt.Sprint(len(body)))
				return nil
			}
			var out bytes.Buffer
			out.Write(body[:idx])
			out.WriteString(injectTag)
			out.Write(body[idx:])

			resp.Body = io.NopCloser(&out)
			resp.ContentLength = int64(out.Len())
			resp.Header.Set("Content-Length", fmt.Sprint(out.Len()))
			return nil
		}

		proxy.ServeHTTP(writer, request)
		return
	})

	opencode.Handle("GET /.gw-mixin/", http.FileServer(http.Dir("./static/opencode/")))

	api.HandleFunc("GET /api/gitlab/project", func(w http.ResponseWriter, r *http.Request) {
		cookie, _ := r.Cookie("gw_session")
		session, _ := sessionManager.Validate(cookie.Value)

		projects, err := gitlabClient.ListProjects(session.Token)
		if err != nil {
			slog.Default().Warn("gitlab projects not found")
			http.Error(w, "gitlab projects not found", http.StatusBadGateway)
			return
		}

		misc.SendJson(w, http.StatusOK, projects)
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

		misc.SendJson(w, http.StatusOK, branches)
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
		misc.SendJson(w, http.StatusOK, body)
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

	srv := &http.Server{
		Addr: appConfig.Host,
		Handler: withTraceLog(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if cookie, err := r.Cookie("gw_session"); err == http.ErrNoCookie {
				handler.ServeHTTP(w, r)
			} else if session, err := sessionManager.Validate(cookie.Value); err != nil {
				handler.ServeHTTP(w, r)
			} else if session.ActiveWorkload != auth.NilWorkloadReference() {
				// hier noch einen healthcheck ausführen ??
				authenticated(opencode).ServeHTTP(w, r)
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

func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func withTraceLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		slog.Default().Info("http request", "method", r.Method, "path", r.URL.Path, "status", recorder.status, "duration", time.Since(start).String(), "remote", r.RemoteAddr)
	})
}

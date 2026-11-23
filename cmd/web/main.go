package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
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
	"gtihub.com/mwildt/git-works/pkg/events"
	"gtihub.com/mwildt/git-works/pkg/git"
	"gtihub.com/mwildt/git-works/pkg/gitlab"
	"gtihub.com/mwildt/git-works/pkg/misc"
	workspaces "gtihub.com/mwildt/git-works/pkg/workspace"
)

type configuration struct {
	Host    string
	TLSCert string
	TLSKey  string
	UseTLS  bool
}

func main() {
	slog.SetLogLoggerLevel(slog.LevelDebug)

	appConfig := &configuration{
		Host: config.GetEnv("SERVER_ADDRESS", "0.0.0.0:8080"),
	}
	appConfig.TLSCert = os.Getenv("TLS_CERT_FILE")
	appConfig.TLSKey = os.Getenv("TLS_KEY_FILE")
	appConfig.UseTLS = appConfig.TLSCert != "" && appConfig.TLSKey != ""

	opencodeActions, err := actions.NewHttpActions(config.GetEnv("GITLAB_ACTIONS_FILE", "gitlab-http-actions.yml"))
	if err != nil {
		log.Fatal(err)
	}

	slog.Info("startup-info",
		"SERVER_ADDRESS", os.Getenv("SERVER_ADDRESS"),
		"DATA_DIR", os.Getenv("DATA_DIR"),
		"GITLAB_ACTIONS_FILE", os.Getenv("GITLAB_ACTIONS_FILE"),
		"GITLAB_BASE_URL", os.Getenv("GITLAB_BASE_URL"),
		"SSH_ASKPASS", os.Getenv("SSH_ASKPASS"),
		"OPENCODE_REMOTE_HOST", os.Getenv("OPENCODE_REMOTE_HOST"),
	)

	sessionManager := auth.NewSessionManager()

	rootHandler := http.NewServeMux()
	opencodeHandler := http.NewServeMux()

	dispatcher := events.NewDispatcher()
	gitClient := git.NewGit()

	portmanager := workspaces.NewPortManager(config.GetEnvInt("WORKSPACE_START_PORT", 9000))
	dispatcher.RegisterFunc(portmanager.Handle)

	auth.NewController(sessionManager).SetUp(rootHandler)

	// **********************************************************+
	// Gitlab Init
	// **********************************************************+
	gitlabClient := gitlab.NewClient(config.GetEnv("GITLAB_BASE_URL", "https://gitlab.com"))
	gitlab.NewController(gitlabClient, sessionManager).SetUp(rootHandler)

	// **********************************************************+
	// Workspace Init
	// **********************************************************+
	workspaceRepo := func() *workspaces.Repository {
		logFile := config.GetEnv("WORKSPACE_LOG", "./data/workspace.v2.log")

		workspaceRepo, err := workspaces.NewRepository(logFile, dispatcher)
		if err != nil {
			log.Fatal(err)
		}

		workspaceView := workspaces.NewUserWorkspacesView()
		dispatcher.RegisterFunc(workspaceView.Handle)

		workspaceInit := workspaces.NewWorkloadInitializer(workspaceRepo, portmanager)
		dispatcher.RegisterFunc(workspaceInit.Handle)

		workspaceService := workspaces.NewService(workspaceRepo, gitClient, opencodeActions, portmanager)
		if workspaceController, err := workspaces.NewController(workspaceRepo, workspaceService, sessionManager, workspaceView); err != nil {
			log.Fatal(err)
		} else {
			workspaceController.SetUp(rootHandler)
		}

		return workspaceRepo
	}()

	// **********************************************************+
	// und jetzt das Replay ALLER Events starten
	// **********************************************************+
	if err = workspaceRepo.Replay(); err != nil {
		log.Fatal(err)
	}

	opencodeHandler.HandleFunc("PATCH /.git-works/session-mode", func(writer http.ResponseWriter, request *http.Request) {
		cookie, err := request.Cookie(auth.SessionCookieName)
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

	staticFS := http.Dir("./static/opencode/")
	staticServer := http.FileServer(staticFS)

	opencodeHandler.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {

		session, ok := auth.ReadSession(request.Context())

		slog.Debug("proxy request", "PATH", request.URL.Path)

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

		config, ok := workload.Data.(workspaces.OpencodeWorkloadConfiguration)
		if !ok {
			http.Error(writer, "not found", http.StatusBadRequest)
		}

		// Zuerst lokale Dateien prüfen
		path := request.URL.Path

		if path != "/" && request.Method == "GET" {

			file, err := staticFS.Open(path)
			if err == nil {

				info, statErr := file.Stat()
				file.Close()

				if statErr == nil && !info.IsDir() {
					slog.Default().Debug("serve file from static ressource", "path", path)
					writer.Header().Set("Cache-Control", "no-store")
					staticServer.ServeHTTP(writer, request)
					return
				}
			}
		}

		target, err := url.Parse(config.TargetUrl())
		if err != nil {
			slog.Default().Warn("invalid workload target url", "error", err)
			http.Error(writer, "invalid workload target url", http.StatusBadGateway)
			return
		}
		proxy := &httputil.ReverseProxy{
			Rewrite: func(proxyRequest *httputil.ProxyRequest) {
				proxyRequest.SetURL(target)
				proxyRequest.Out.SetBasicAuth(config.AuthUsername, config.AuthPassword)
				proxyRequest.Out.Header.Set("Accept-Encoding", "identity")
				slog.Default().Debug("pass reqeuest to downstream host", "path", path, "url", target.String())
			},
			FlushInterval: -1,
			ModifyResponse: func(resp *http.Response) error {
				contentType := resp.Header.Get("Content-Type")
				resp.Header.Set("Cache-Control", "no-store")

				if !strings.Contains(contentType, "text/html") {
					return nil
				}

				body, err := io.ReadAll(resp.Body)
				if err != nil {
					resp.Body.Close()
					return fmt.Errorf("read upstream body: %w", err)
				}
				resp.Body.Close()

				lower := bytes.ToLower(body)
				idx := bytes.LastIndex(lower, []byte("</body>"))
				if idx == -1 {
					resp.Body = io.NopCloser(bytes.NewReader(body))
					resp.ContentLength = int64(len(body))
					resp.Header.Set("Content-Length", fmt.Sprint(len(body)))
					return nil
				}
				const injectTag = "<script type=\"module\" crossorigin src=\"/.gw-mixin/index.js\" defer></script>"
				var out bytes.Buffer
				out.Write(body[:idx])
				out.WriteString(injectTag)
				out.Write(body[idx:])

				resp.Body = io.NopCloser(&out)
				resp.ContentLength = int64(out.Len())
				resp.Header.Set("Content-Length", fmt.Sprint(out.Len()))
				return nil
			},
			ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
				slog.Default().Error("proxy error", "method", r.Method, "path", r.URL.Path, "error", err)

				if _, ok := errors.AsType[*net.OpError](err); ok {
					sessionManager.ActivateWorkspace(session.Id, auth.WorkloadReference{})
					http.RedirectHandler("/", http.StatusTemporaryRedirect)
				} else {
					http.Error(w, "workload unreachable", http.StatusBadGateway)
				}
			},
		}

		proxy.ServeHTTP(writer, request)
		return
	})

	rootHandler.HandleFunc("POST /api/auth/token", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Token string `json:"token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		token := gitlab.Token{
			Value:     body.Token,
			TokenType: "PRIVATE-TOKEN",
		}
		user, err := gitlabClient.GetUser(token)
		if err != nil {
			slog.Default().Error("unable to get user", "error", err)
			http.Error(w, "unknown", http.StatusUnauthorized)
			return
		}
		session, err := sessionManager.CreateSession(user.Username)
		if err != nil {
			http.Error(w, "invalid JSON", http.StatusUnauthorized)
			return
		}
		sessionManager.PutValue(session.Id, gitlab.TokenKey, token)

		slog.Default().Info("token introspection success", "username", session.Username)
		http.SetCookie(w, &http.Cookie{
			Name:     auth.SessionCookieName,
			Value:    session.Id,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   int(12 * time.Hour.Seconds()),
		})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
	})

	rootHandler.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	rootHandler.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static/git-works"))))

	rootHandler.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, path.Join("./static/git-works", "index.html"))
	})

	srv := &http.Server{
		Addr: appConfig.Host,
		Handler: withTraceLog(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			slog.Default().Debug("request", "path", r.URL.Path)
			if cookie, err := r.Cookie(auth.SessionCookieName); err == http.ErrNoCookie {
				rootHandler.ServeHTTP(w, r)
			} else if session, err := sessionManager.Validate(cookie.Value); err != nil {
				rootHandler.ServeHTTP(w, r)
			} else if session.ActiveWorkload != auth.NilWorkloadReference() {
				// hier noch einen healthcheck ausführen ??
				sessionManager.Authenticated(opencodeHandler).ServeHTTP(w, r)
			} else {
				rootHandler.ServeHTTP(w, r)
			}
		})),
		DisableGeneralOptionsHandler: false,
		TLSConfig:                    nil,
		ReadTimeout:                  30 * time.Second,
		ReadHeaderTimeout:            10 * time.Second,
		IdleTimeout:                  120 * time.Second,
	}

	slog.Default().Info("starting server", slog.String("host", appConfig.Host), slog.Bool("tls", appConfig.UseTLS))
	var serverErr error
	if appConfig.UseTLS {
		serverErr = srv.ListenAndServeTLS(appConfig.TLSCert, appConfig.TLSKey)
	} else {
		serverErr = srv.ListenAndServe()
	}
	if serverErr != nil && serverErr != http.ErrServerClosed {
		slog.Default().Error("server error", "error", serverErr)
		os.Exit(1)
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := r.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	return h.Hijack()
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

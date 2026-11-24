package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"gtihub.com/mwildt/git-works/pkg/config"
)

type (
	PipelineRequest struct {
		Ref       string            `json:"ref"`
		Variables map[string]string `json:"variables,omitempty"`
	}

	PipelineResponse struct {
		ID     int    `json:"id"`
		Ref    string `json:"ref"`
		Status string `json:"status"`
		WebURL string `json:"web_url"`
	}
)

func main() {

	handler := http.NewServeMux()

	pipelineID := 0

	handler.HandleFunc("POST /api/v4/pipelines", func(writer http.ResponseWriter, request *http.Request) {
		var header bytes.Buffer
		request.Header.Write(&header)

		slog.Default().Info("Received pipeline",
			"uri", request.RequestURI,
			"header", header.String(),
		)
		var body PipelineRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil || body.Ref == "" {
			slog.Default().Error("fehler beim parsen", "error", err.Error())
			http.Error(writer, `{"error": "invalid payload, 'ref' is required"}`, http.StatusBadRequest)
			return
		}

		pipelineID++

		slog.Default().Info("run new pipeline ",
			"pipelineID", pipelineID,
			"variables", body.Variables,
			"ref", body.Ref,
			"job", body.Variables["NAME"])

		workspaceId := body.Variables["WORKSPACE_ID"]
		workloadId := body.Variables["WORKLOAD_ID"]

		opencodeFile := fmt.Sprintf("opencode.%s.json", workloadId)

		if body.Variables["JOB"] == "OC_UP" {
			args := []string{
				"run", "-d", "--replace", "--rm",
				"--name", body.Variables["NAME"],
				"--user", "0:0",
				"-e", "HOME=/opencode/home",
				"-e", "OPENCODE_CONFIG=/opencode/opencode.json",
				"-e", "OPENCODE_SERVER_USERNAME=" + body.Variables["OPENCODE_SERVER_USERNAME"],
				"-e", "OPENCODE_SERVER_PASSWORD=" + body.Variables["OPENCODE_SERVER_PASSWORD"],
				"-e", "GIT_AUTHOR_NAME=Klaus",
				"-e", "GIT_AUTHOR_EMAIL=klaus@kalle.com",
				"-e", "GIT_COMMITTER_NAME=Klaus",
				"-e", "GIT_COMMITTER_EMAIL=klaus@kalle.com",
				"-p", fmt.Sprintf("%s:4096", body.Variables["PORT"]),
				"-v", fmt.Sprintf("./data/%s/workspace:/workspace", workspaceId),
				"-v", fmt.Sprintf("./data/%s/%s:/opencode/opencode.json:ro", workspaceId, opencodeFile),
				"localhost/opencode-web:latest",
			}

			slog.Default().Info("run podman: ", "args", strings.Join(args, " "))

			cmd := exec.Command("podman", args...)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr

			if err := cmd.Run(); err != nil {
				fmt.Fprintln(os.Stderr, "podman run failed:", err)
			}
		}

		if body.Variables["JOB"] == "VSCODE_UP" {
			args := []string{
				"run", "-d", "--replace", "--rm",
				"--name", body.Variables["NAME"],
				"--user", "0:0",
				"-p", fmt.Sprintf("%s:8080", body.Variables["PORT"]),
				"-v", fmt.Sprintf("./data/%s/workspace:/workspace", workspaceId),
				"-v", "./vscode.yml:/vscode/config.yaml:ro",
				"-v", "./vscode.settings.json:/root/.local/share/code-server/User/settings.json:ro",
				"-e", "EXTENSIONS_GALLERY={}",
				"git-works-vscode", "--config", "/vscode/config.yaml", "/workspace",
			}

			slog.Default().Info("run podman: ", "args", strings.Join(args, " "))

			cmd := exec.Command("podman", args...)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr

			if err := cmd.Run(); err != nil {
				fmt.Fprintln(os.Stderr, "podman run failed:", err)
			}
		}

		if body.Variables["JOB"] == "OC_STOP" {
			args := []string{"stop", body.Variables["NAME"]}

			slog.Default().Info("run podman: ", "args", strings.Join(args, " "))

			cmd := exec.Command("podman", args...)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr

			if err := cmd.Run(); err != nil {
				fmt.Fprintln(os.Stderr, "podman run failed:", err)
			}
		}

		if body.Variables["JOB"] == "VSCODE_STOP" {
			args := []string{"stop", body.Variables["NAME"]}

			slog.Default().Info("run podman: ", "args", strings.Join(args, " "))

			cmd := exec.Command("podman", args...)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr

			if err := cmd.Run(); err != nil {
				fmt.Fprintln(os.Stderr, "podman run failed:", err)
			}
		}

		if body.Variables["JOB"] == "OC_RM" {
			args := []string{"rm", "-f", body.Variables["NAME"]}

			slog.Default().Info("run podman: ", "args", strings.Join(args, " "))

			cmd := exec.Command("podman", args...)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr

			if err := cmd.Run(); err != nil {
				fmt.Fprintln(os.Stderr, "podman run failed:", err)
			}
		}

		if body.Variables["JOB"] == "VSCODE_RM" {
			args := []string{"rm", "-f", body.Variables["NAME"]}

			slog.Default().Info("run podman: ", "args", strings.Join(args, " "))

			cmd := exec.Command("podman", args...)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr

			if err := cmd.Run(); err != nil {
				fmt.Fprintln(os.Stderr, "podman run failed:", err)
			}
		}

		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusCreated)
		json.NewEncoder(writer).Encode(PipelineResponse{
			ID:     pipelineID,
			Ref:    body.Ref,
			Status: "pending",
			WebURL: "https://ci.example.com/pipelines/" + strconv.Itoa(pipelineID),
		})

	})

	// ---------- GitLab API (Token-Login) ----------

	handler.HandleFunc("GET /api/v4/projects", func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("PRIVATE-TOKEN") == "" && request.Header.Get("Authorization") == "" {
			http.Error(writer, `{"message":"401 Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("X-Next-Page", "")
		json.NewEncoder(writer).Encode([]any{})
	})

	handler.HandleFunc("GET /api/v4/user", func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("PRIVATE-TOKEN") == "" && request.Header.Get("Authorization") == "" {
			http.Error(writer, `{"message":"401 Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		json.NewEncoder(writer).Encode(map[string]any{
			"id":       1,
			"username": "mockuser",
			"name":     "Mock User",
			"email":    "mock@example.com",
			"state":    "active",
			"web_url":  "https://gitlab.example.com/mockuser",
		})
	})

	// ---------- OIDC-Provider (fuer git-works als Client) ----------

	mockBase := func(request *http.Request) string {
		scheme := "http"
		if request.TLS != nil {
			scheme = "https"
		}
		return scheme + "://" + request.Host
	}

	handler.HandleFunc("GET /.well-known/openid-configuration", func(writer http.ResponseWriter, request *http.Request) {
		base := mockBase(request)
		writer.Header().Set("Content-Type", "application/json")
		json.NewEncoder(writer).Encode(map[string]any{
			"issuer":                           base,
			"authorization_endpoint":           base + "/oauth/authorize",
			"token_endpoint":                   base + "/oauth/token",
			"userinfo_endpoint":                base + "/oauth/userinfo",
			"revocation_endpoint":              base + "/oauth/revoke",
			"jwks_uri":                         base + "/oauth/jwks",
			"response_types_supported":         []string{"code"},
			"grant_types_supported":            []string{"authorization_code", "refresh_token"},
			"code_challenge_methods_supported": []string{"S256"},
			"scopes_supported":                 []string{"openid", "profile", "email", "api", "offline_access"},
		})
	})

	handler.HandleFunc("GET /oauth/authorize", func(writer http.ResponseWriter, request *http.Request) {
		q := request.URL.Query()
		redirectURI := q.Get("redirect_uri")
		if redirectURI == "" {
			http.Error(writer, "missing redirect_uri", http.StatusBadRequest)
			return
		}
		sep := "?"
		if strings.Contains(redirectURI, "?") {
			sep = "&"
		}
		http.Redirect(writer, request, redirectURI+sep+"code=mock-auth-code&state="+q.Get("state"), http.StatusFound)
	})

	handler.HandleFunc("POST /oauth/token", func(writer http.ResponseWriter, request *http.Request) {
		if err := request.ParseForm(); err != nil {
			http.Error(writer, "invalid form", http.StatusBadRequest)
			return
		}
		slog.Default().Info("oauth token request", "grant_type", request.Form.Get("grant_type"))
		expiresIn := 7200
		if raw := os.Getenv("MOCK_TOKEN_EXPIRES_IN"); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil {
				expiresIn = n
			}
		}
		writer.Header().Set("Content-Type", "application/json")
		json.NewEncoder(writer).Encode(map[string]any{
			"access_token":  "mock-access-token-" + request.Form.Get("grant_type"),
			"refresh_token": "mock-refresh-token",
			"token_type":    "Bearer",
			"expires_in":    expiresIn,
			"scope":         "openid profile email api offline_access",
		})
	})

	handler.HandleFunc("GET /oauth/userinfo", func(writer http.ResponseWriter, request *http.Request) {
		if !strings.HasPrefix(request.Header.Get("Authorization"), "Bearer ") {
			http.Error(writer, `{"error":"invalid_token"}`, http.StatusUnauthorized)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		json.NewEncoder(writer).Encode(map[string]any{
			"sub":                "1",
			"name":               "Mock User",
			"preferred_username": "mockuser",
			"email":              "mock@example.com",
		})
	})

	handler.HandleFunc("POST /oauth/revoke", func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
	})

	if err := http.ListenAndServe(config.GetEnv("SERVICE_ADRESS", ":8999"), handler); err != nil {
		slog.Default().Error(err.Error())
	}
}

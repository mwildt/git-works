package main

import (
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
		slog.Default().Info("Received pipeline", "uri", request.RequestURI)
		var body PipelineRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil || body.Ref == "" {
			slog.Default().Error("fehler beim parsen", "error", err.Error())
			http.Error(writer, `{"error": "invalid payload, 'ref' is required"}`, http.StatusBadRequest)
			return
		}

		pipelineID++

		slog.Default().Info("run new pipeline ", "pipelineID", pipelineID, "variables", body.Variables, "ref", body.Ref)

		if body.Variables["JOB"] == "OC_UP" {
			args := []string{
				"run", "-d", "--replace", "--rm",
				"--name", body.Variables["NAME"],
				"-e", "OPENCODE_SERVER_USERNAME=" + body.Variables["OPENCODE_SERVER_USERNAME"],
				"-e", "OPENCODE_SERVER_PASSWORD=" + body.Variables["OPENCODE_SERVER_PASSWORD"],
				"-p", fmt.Sprintf("%s:4096", body.Variables["PORT"]),
				"-v", fmt.Sprintf("./data/%s/workspace:/workspace", body.Variables["WORKSPACE_ID"]),
				"opencode-web",
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

		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusCreated)
		json.NewEncoder(writer).Encode(PipelineResponse{
			ID:     pipelineID,
			Ref:    body.Ref,
			Status: "pending",
			WebURL: "https://ci.example.com/pipelines/" + strconv.Itoa(pipelineID),
		})

	})

	if err := http.ListenAndServe(config.GetEnv("SERVICE_ADRESS", ":8999"), handler); err != nil {
		slog.Default().Error(err.Error())
	}
}

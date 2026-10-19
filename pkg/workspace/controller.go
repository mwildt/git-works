package workspace

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"uuid"

	"gtihub.com/mwildt/git-works/pkg/auth"
)

type Controller struct {
	repo           *Repository
	service        *Service
	sessionManager *auth.Manager
}

type CommitRequest struct {
	Message string `json:"message"`
	Branch  string `json:"branch"`
}

func NewController(repo *Repository, service *Service, sessionManager *auth.Manager) (*Controller, error) {
	return &Controller{
		repo:           repo,
		service:        service,
		sessionManager: sessionManager,
	}, nil
}

func (ctrl *Controller) SetUpApi(api *http.ServeMux) error {

	service := ctrl.service

	api.HandleFunc("GET /api/workspace", func(w http.ResponseWriter, r *http.Request) {
		workspaces, err := ctrl.repo.All(r.Context())
		if err != nil {
			sendError(w, err)
			return
		}
		var response []WorkspaceResponse
		for _, w := range workspaces {
			response = append(response, ToWorkspaceResponse(w))
		}
		sendJson(w, http.StatusOK, response)
	})

	api.HandleFunc("POST /api/workspace", func(w http.ResponseWriter, r *http.Request) {
		workspace, err := service.Create(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		sendJson(w, http.StatusCreated, ToWorkspaceResponse(workspace))
	})

	api.HandleFunc("GET /api/workspace/{workspaceId}", func(w http.ResponseWriter, r *http.Request) {
		workspaceId, err := uuid.Parse(r.PathValue("workspaceId"))
		if err != nil {
			slog.Default().Warn("invalid ws id")
			sendErrorMessage(w, http.StatusBadRequest, "invalid workspace id")
			return
		}

		workspace, err := ctrl.repo.ById(r.Context(), workspaceId)
		if err != nil {
			slog.Default().Warn("workspace not found")
			sendError(w, err)
			return
		}
		sendJson(w, http.StatusOK, ToWorkspaceResponse(&workspace))
	})

	api.HandleFunc("DELETE /api/workspace/{workspaceId}", func(w http.ResponseWriter, r *http.Request) {
		workspaceId, err := uuid.Parse(r.PathValue("workspaceId"))
		if err != nil {
			slog.Default().Warn("invalid ws id")
			sendErrorMessage(w, http.StatusBadRequest, "invalid workspace id")
			return
		}
		err = service.Delete(r.Context(), workspaceId)
		if err != nil {
			sendError(w, err)
			return
		}

		w.WriteHeader(http.StatusAccepted)
	})

	api.HandleFunc("POST /api/workspace/{workspaceId}/init", func(writer http.ResponseWriter, request *http.Request) {
		workspaceId, err := uuid.Parse(request.PathValue("workspaceId"))
		if err != nil {
			http.Error(writer, "invalid ws id", http.StatusBadRequest)
			return
		}

		workspace, err := ctrl.repo.ById(request.Context(), workspaceId)
		if err != nil {
			sendError(writer, err)
			return
		}

		if err := service.WorkspaceInit(request.Context(), workspace); err != nil {
			sendError(writer, err)
			return
		}
		writer.WriteHeader(http.StatusAccepted)
	})

	api.HandleFunc("POST /api/workspace/{workspaceId}/project", func(writer http.ResponseWriter, request *http.Request) {
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
		workspaceId, err := uuid.Parse(request.PathValue("workspaceId"))
		if err != nil {
			slog.Default().Warn("invalid ws id")
			http.Error(writer, "invalid ws id", http.StatusBadRequest)
			return
		}

		workspace, err := ctrl.repo.ById(request.Context(), workspaceId)
		if err != nil {
			slog.Default().Warn("workspace not found")
			sendError(writer, err)
			return
		}
		workspace.addProject(project.Path, project.Project.GitUrl, project.Branch.Name)
		if err = ctrl.repo.Update(request.Context(), workspace); err != nil {
			sendError(writer, err)
			return
		}

		writer.WriteHeader(http.StatusCreated)
	})

	api.HandleFunc("DELETE /api/workspace/{workspaceId}/project/{projectId}", func(writer http.ResponseWriter, request *http.Request) {
		workspaceId, err := uuid.Parse(request.PathValue("workspaceId"))
		if err != nil {
			sendError(writer, err)
			return
		}
		projectId, err := uuid.Parse(request.PathValue("projectId"))
		if err != nil {
			sendError(writer, err)
			return
		}

		err = ctrl.service.DeleteProject(request.Context(), workspaceId, projectId)
		if err != nil {
			sendError(writer, err)
			return
		}
		writer.WriteHeader(http.StatusAccepted)
	})

	api.HandleFunc("GET /api/workspace/{workspaceId}/project/{projectId}/status", func(writer http.ResponseWriter, request *http.Request) {
		workspaceId, err := uuid.Parse(request.PathValue("workspaceId"))
		if err != nil {
			slog.Default().Warn("invalid ws id")
			sendErrorMessage(writer, http.StatusBadRequest, "invalid field: workspaceId")
			return
		}
		projectId, err := uuid.Parse(request.PathValue("projectId"))
		if err != nil {
			slog.Default().Warn("invalid projectId")
			sendErrorMessage(writer, http.StatusBadRequest, "invalid field: projectId")
			return
		}

		workspace, project, err := ctrl.repo.ProjectById(request.Context(), workspaceId, projectId)
		if err != nil {
			sendError(writer, err)
			return
		}

		status, err := service.ProjectStatus(request.Context(), workspace, project)
		if err != nil {
			sendError(writer, err)
			return
		}
		sendJson(writer, http.StatusOK, status)
	})

	api.HandleFunc("POST /api/workspace/{workspaceId}/project/{projectId}/init", func(writer http.ResponseWriter, request *http.Request) {
		workspaceId, err := uuid.Parse(request.PathValue("workspaceId"))
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

		workspace, project, err := ctrl.repo.ProjectById(request.Context(), workspaceId, projectId)
		if err != nil {
			sendError(writer, err)
			return
		}

		if err := service.ProjectInit(request.Context(), workspace, project); err != nil {
			sendError(writer, err)
			return
		}
		writer.WriteHeader(http.StatusAccepted)
	})

	api.HandleFunc("POST /api/workspace/{workspaceId}/project/{projectId}/commit", func(writer http.ResponseWriter, request *http.Request) {
		workspaceId, err := uuid.Parse(request.PathValue("workspaceId"))
		if err != nil {
			http.Error(writer, "invalid ws id", http.StatusBadRequest)
			return
		}
		projectId, err := uuid.Parse(request.PathValue("projectId"))
		if err != nil {
			http.Error(writer, "invalid projectId", http.StatusBadRequest)
			return
		}
		var commitRequest CommitRequest
		if err := json.NewDecoder(request.Body).Decode(&commitRequest); err != nil {
			http.Error(writer, "invalid JSON", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(commitRequest.Message) == "" {
			http.Error(writer, "message is required", http.StatusBadRequest)
			return
		}

		if err := service.ProjectCommitPush(request.Context(), workspaceId, projectId, commitRequest); err != nil {
			slog.Default().Error("failed to push commit", "error", err)
			sendError(writer, err)
			return
		}
		writer.WriteHeader(http.StatusAccepted)
	})

	api.HandleFunc("GET /api/workspace/{workspaceId}/workload/{workloadId}/status", func(writer http.ResponseWriter, request *http.Request) {
		workspaceId, err := uuid.Parse(request.PathValue("workspaceId"))
		if err != nil {
			slog.Default().Warn("invalid ws id")
			http.Error(writer, "invalid ws id", http.StatusBadRequest)
			return
		}
		workloadId, err := uuid.Parse(request.PathValue("workloadId"))
		if err != nil {
			slog.Default().Warn("invalid workloadId")
			http.Error(writer, "invalid workloadId", http.StatusBadRequest)
			return
		}

		workspace, workload, err := ctrl.repo.WorkloadById(request.Context(), workspaceId, workloadId)
		if err != nil {
			sendError(writer, err)
			return
		}

		if status, err := service.WorkloadStatus(request.Context(), workload); err != nil {
			sendError(writer, err)
			return
		} else {
			sendJson(writer, http.StatusOK, struct {
				WorkspaceId uuid.UUID `json:"workspaceId"`
				WorkloadId  uuid.UUID `json:"workloadId"`
				Status      string    `json:"status"`
			}{
				WorkspaceId: workspace.Id,
				WorkloadId:  workload.Id,
				Status:      status,
			})
		}

	})

	api.HandleFunc("POST /api/workspace/{workspaceId}/workload/{workloadId}/init", func(writer http.ResponseWriter, request *http.Request) {
		workspaceId, err := uuid.Parse(request.PathValue("workspaceId"))
		if err != nil {
			slog.Default().Warn("invalid ws id")
			http.Error(writer, "invalid ws id", http.StatusBadRequest)
			return
		}
		workloadId, err := uuid.Parse(request.PathValue("workloadId"))
		if err != nil {
			slog.Default().Warn("invalid workloadId")
			http.Error(writer, "invalid workloadId", http.StatusBadRequest)
			return
		}
		workspace, workload, err := ctrl.repo.WorkloadById(request.Context(), workspaceId, workloadId)
		if err != nil {
			sendError(writer, err)
			return
		}
		if err := service.WorkloadInit(request.Context(), workspace, workload); err != nil {
			sendError(writer, err)
			return
		}
		writer.WriteHeader(http.StatusAccepted)
	})

	api.HandleFunc("POST /api/workspace/{workspaceId}/workload/{workloadId}/run", func(writer http.ResponseWriter, request *http.Request) {
		workspaceId, err := uuid.Parse(request.PathValue("workspaceId"))
		if err != nil {
			slog.Default().Warn("invalid ws id")
			http.Error(writer, "invalid ws id", http.StatusBadRequest)
			return
		}
		workloadId, err := uuid.Parse(request.PathValue("workloadId"))
		if err != nil {
			slog.Default().Warn("invalid workloadId")
			http.Error(writer, "invalid workloadId", http.StatusBadRequest)
			return
		}
		workspace, workload, err := ctrl.repo.WorkloadById(request.Context(), workspaceId, workloadId)
		if err != nil {
			sendError(writer, err)
			return
		}
		if err := service.WorkloadStart(request.Context(), workspace, workload); err != nil {
			sendError(writer, err)
			return
		}
		writer.WriteHeader(http.StatusAccepted)
	})

	api.HandleFunc("POST /api/workspace/{workspaceId}/workload/{workloadId}/stop", func(writer http.ResponseWriter, request *http.Request) {
		workspaceId, err := uuid.Parse(request.PathValue("workspaceId"))
		if err != nil {
			slog.Default().Warn("invalid ws id")
			http.Error(writer, "invalid ws id", http.StatusBadRequest)
			return
		}
		workloadId, err := uuid.Parse(request.PathValue("workloadId"))
		if err != nil {
			slog.Default().Warn("invalid workloadId")
			http.Error(writer, "invalid workloadId", http.StatusBadRequest)
			return
		}
		workspace, workload, err := ctrl.repo.WorkloadById(request.Context(), workspaceId, workloadId)
		if err != nil {
			sendError(writer, err)
			return
		}
		if err := service.WorkloadStop(request.Context(), workspace, workload); err != nil {
			sendError(writer, err)
			return
		}
		writer.WriteHeader(http.StatusAccepted)

	})

	api.HandleFunc("POST /api/workspace/{workspaceId}/workload/{workloadId}/activate", func(writer http.ResponseWriter, request *http.Request) {
		workspaceId, err := uuid.Parse(request.PathValue("workspaceId"))
		if err != nil {
			http.Error(writer, "invalid workspaceId", http.StatusBadRequest)
			return
		}

		workloadId, err := uuid.Parse(request.PathValue("workloadId"))
		if err != nil {
			http.Error(writer, "invalid workloadId", http.StatusBadRequest)
			return
		}

		session, ok := auth.ReadSession(request.Context())
		if !ok {
			http.Error(writer, "not found", http.StatusForbidden)
		}
		var location struct {
			Location string `json:"location"`
		}
		if err := json.NewDecoder(request.Body).Decode(&location); err != nil {
			http.Error(writer, "invalid JSON", http.StatusBadRequest)
			return
		}
		workspace, workload, err := ctrl.repo.WorkloadById(request.Context(), workspaceId, workloadId)
		if err != nil {
			http.Error(writer, "invalid workloadId", http.StatusBadRequest)
			return
		}

		status, err := ctrl.service.WorkloadStatus(request.Context(), workload)
		if err != nil {
			http.Error(writer, "invalid workload status", http.StatusBadRequest)
			return
		}
		if status != "up" {
			http.Error(writer, "workspace is down", http.StatusBadGateway)
			return
		}
		if err := ctrl.sessionManager.ActivateWorkspace(session.Id, auth.WorkloadReference{
			Location:    location.Location,
			WorkspaceId: workspace.Id,
			WorkloadId:  workload.Id,
		}); err != nil {
			slog.Default().Warn("unable to activate workspace", "workspaceId", workspaceId.String(), "err", err)
			http.Error(writer, "invalid cookie", http.StatusInternalServerError)
			return
		}
		writer.WriteHeader(http.StatusAccepted)
	})
	return nil
}

func sendErrorMessage(w http.ResponseWriter, status int, message string) {
	http.Error(w, message, status)
}

func sendError(w http.ResponseWriter, err error) {
	if errors.Is(err, NotFound) {
		http.Error(w, "not found", http.StatusNotFound)
	}

	if errors.Is(err, Forbidden) {
		http.Error(w, "das ist nicht erlaubt", http.StatusForbidden)
	}

	http.Error(w, "ein fehler ist aufgetrten", http.StatusInternalServerError)
}

func sendJson(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

type WorkloadResponse struct {
	Id   uuid.UUID `json:"id"`
	Name string    `json:"name"`
	Type string    `json:"type"`
}

type ProjectResponse struct {
	Id     uuid.UUID `json:"id"`
	Path   string    `json:"path,omitempty"`
	GitUrl string    `json:"git_url,omitempty"`
	Branch string    `json:"branch,omitempty"`
}

type WorkspaceResponse struct {
	Id        uuid.UUID          `json:"id"`
	Owner     string             `json:"owner"`
	Name      string             `json:"name"`
	Created   string             `json:"created"`
	Projects  []ProjectResponse  `json:"projects"`
	Workloads []WorkloadResponse `json:"workloads"`
}

func ToWorkspaceResponse(workspace *Workspace) WorkspaceResponse {
	response := WorkspaceResponse{
		Id:        workspace.Id,
		Name:      workspace.Name,
		Owner:     workspace.Owner,
		Created:   workspace.Created.Format(time.RFC3339),
		Projects:  make([]ProjectResponse, 0),
		Workloads: make([]WorkloadResponse, 0),
	}
	for _, project := range workspace.Projects {
		response.Projects = append(response.Projects, ProjectResponse{
			Id:     project.Id,
			Path:   project.Path,
			GitUrl: project.Path,
			Branch: project.Branch,
		})
	}
	for _, workload := range workspace.Workloads {
		response.Workloads = append(response.Workloads, WorkloadResponse{
			Id:   workload.Id,
			Name: workload.Type + "_workload_" + workload.Id.String()[0:10],
			Type: workload.Type,
		})
	}
	return response
}

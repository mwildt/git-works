package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"
	"uuid"

	"gtihub.com/mwildt/git-works/pkg/auth"
	"gtihub.com/mwildt/git-works/pkg/misc"
)

type Controller struct {
	repo           *Repository
	service        *Service
	sessionManager *auth.Manager
	workspaceView  *UserWorkspacesView
}

type CommitRequest struct {
	Message string `json:"message"`
}

type PushRequest struct {
	Branch string `json:"branch"`
}

func NewController(repo *Repository, service *Service, sessionManager *auth.Manager, workspaceView *UserWorkspacesView) (*Controller, error) {
	return &Controller{
		repo:           repo,
		service:        service,
		sessionManager: sessionManager,
		workspaceView:  workspaceView,
	}, nil
}

func (ctrl *Controller) getWorksspace(w http.ResponseWriter, r *http.Request) {
	owner, ok := auth.Owner(r.Context())
	if !ok {
		slog.Default().Warn("no owner")
		sendErrorMessage(w, http.StatusBadRequest, "no owner")
		return
	}
	workspaces := ctrl.workspaceView.ByOwner(owner)
	sendJson(w, http.StatusOK, workspaces)
}

func (ctrl *Controller) SetUp(api *http.ServeMux) error {

	service := ctrl.service

	api.HandleFunc("GET /api/workspace", ctrl.sessionManager.Authenticated(http.HandlerFunc(ctrl.getWorksspace)))

	api.HandleFunc("POST /api/workspace", ctrl.sessionManager.Authenticated(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		workspace, err := service.Create(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		sendJson(w, http.StatusCreated, ToWorkspaceResponse(workspace))
	})))

	api.HandleFunc("GET /api/workspace/{workspaceId}", ctrl.sessionManager.Authenticated(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		workspaceId, err := uuid.Parse(r.PathValue("workspaceId"))
		if err != nil {
			slog.Default().Warn("invalid ws id")
			sendErrorMessage(w, http.StatusBadRequest, "invalid workspace id")
			return
		}

		workspace, err := ctrl.repo.LoadById(r.Context(), workspaceId)
		if err != nil {
			slog.Default().Warn("workspace not found")
			sendError(w, err)
			return
		}
		sendJson(w, http.StatusOK, ToWorkspaceResponse(workspace))
	})))

	api.HandleFunc("DELETE /api/workspace/{workspaceId}", ctrl.sessionManager.Authenticated(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	})))

	api.HandleFunc("POST /api/workspace/{workspaceId}/init", ctrl.sessionManager.Authenticated(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		workspaceId, err := uuid.Parse(request.PathValue("workspaceId"))
		if err != nil {
			http.Error(writer, "invalid ws id", http.StatusBadRequest)
			return
		}

		workspace, err := ctrl.repo.byId(request.Context(), workspaceId)
		if err != nil {
			sendError(writer, err)
			return
		}

		if err := service.WorkspaceInit(request.Context(), workspace); err != nil {
			sendError(writer, err)
			return
		}
		writer.WriteHeader(http.StatusAccepted)
	})))

	api.HandleFunc("POST /api/workspace/{workspaceId}/project", ctrl.sessionManager.Authenticated(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
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

		workspace, err := ctrl.repo.LoadById(request.Context(), workspaceId)
		if err != nil {
			slog.Default().Warn("workspace not found")
			sendError(writer, err)
			return
		}
		if err := workspace.addProject(project.Path, project.Project.GitUrl, project.Branch.Name); err != nil {
			sendError(writer, err)
			return
		}
		if err = ctrl.repo.Save(request.Context(), workspace); err != nil {
			sendError(writer, err)
			return
		}

		writer.WriteHeader(http.StatusCreated)
	})))

	api.HandleFunc("DELETE /api/workspace/{workspaceId}/project/{projectId}", ctrl.sessionManager.Authenticated(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
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
	})))

	api.HandleFunc("GET /api/workspace/{workspaceId}/project/{projectId}/status", ctrl.sessionManager.Authenticated(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
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
	})))

	api.HandleFunc("POST /api/workspace/{workspaceId}/project/{projectId}/init", ctrl.sessionManager.Authenticated(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
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
	})))

	api.HandleFunc("POST /api/workspace/{workspaceId}/project/{projectId}/commit", ctrl.sessionManager.Authenticated(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
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

		if err := service.ProjectCommit(request.Context(), workspaceId, projectId, commitRequest); err != nil {
			slog.Default().Error("failed to commit", "error", err)
			sendError(writer, err)
			return
		}
		writer.WriteHeader(http.StatusAccepted)
	})))

	api.HandleFunc("POST /api/workspace/{workspaceId}/project/{projectId}/push", ctrl.sessionManager.Authenticated(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
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
		var pushRequest PushRequest
		if err := json.NewDecoder(request.Body).Decode(&pushRequest); err != nil {
			http.Error(writer, "invalid JSON", http.StatusBadRequest)
			return
		}

		if err := service.ProjectPush(request.Context(), workspaceId, projectId, pushRequest); err != nil {
			slog.Default().Error("failed to push", "error", err)
			sendError(writer, err)
			return
		}
		writer.WriteHeader(http.StatusAccepted)
	})))

	api.HandleFunc("GET /api/workspace/{workspaceId}/workload/{workloadId}/status", ctrl.sessionManager.Authenticated(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
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

	})))

	api.HandleFunc("POST /api/workspace/{workspaceId}/workload/{workloadId}/init", ctrl.sessionManager.Authenticated(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
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
	})))

	api.HandleFunc("POST /api/workspace/{workspaceId}/workload/{workloadId}/run", ctrl.sessionManager.Authenticated(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
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
	})))

	api.HandleFunc("POST /api/workspace/{workspaceId}/workload/{workloadId}/stop", ctrl.sessionManager.Authenticated(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
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
	})))

	api.HandleFunc("POST /api/workspace/{workspaceId}/workload/{workloadId}/activate", ctrl.sessionManager.Authenticated(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
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
	})))

	api.HandleFunc("POST /api/workspace/{workspaceId}/vscode", ctrl.sessionManager.Authenticated(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		workspaceId, err := uuid.Parse(request.PathValue("workspaceId"))
		if err != nil {
			http.Error(writer, "invalid workspaceId", http.StatusBadRequest)
			return
		}

		workspace, err := ctrl.repo.LoadById(request.Context(), workspaceId)
		if err != nil {
			sendError(writer, err)
			return
		}

		if err := ctrl.service.CreateVscodeWorkload(request.Context(), workspace); err != nil {
			sendError(writer, err)
			return
		}

		writer.WriteHeader(http.StatusCreated)
	})))

	api.HandleFunc("GET /api/workspace/{workspaceId}/vscode", ctrl.sessionManager.Authenticated(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		workspaceId, err := uuid.Parse(request.PathValue("workspaceId"))
		if err != nil {
			http.Error(writer, "invalid workspaceId", http.StatusBadRequest)
			return
		}

		workspace, err := ctrl.repo.LoadById(request.Context(), workspaceId)
		if err != nil {
			sendError(writer, err)
			return
		}

		// Find the VSCode workload
		var vscodeWorkload *Workload
		var vscodeConfig *VscodeWorkloadConfiguration
		for _, workload := range workspace.Workloads {
			if workload.Type == "VSCode" {
				vscodeWorkload = &workload
				if config, ok := workload.Data.(VscodeWorkloadConfiguration); ok {
					vscodeConfig = &config
				}
				break
			}
		}

		if vscodeWorkload == nil || vscodeConfig == nil {
			http.Error(writer, "VSCode workload not found", http.StatusNotFound)
			return
		}

		sendJson(writer, http.StatusOK, VscodeWorkloadResponse{
			Id:       vscodeWorkload.Id,
			Type:     "VSCode",
			Host:     vscodeConfig.Host,
			Port:     vscodeConfig.Port,
			Password: "", // Don't return password in response
		})
	})))

	//	/-/workspace/a8c2acad-da80-4c60-a024-405519f614e1/workload/073dfe10-e97c-4d35-9d22-10e45854f56b/vscode/

	api.HandleFunc("/-/workspace/{workspaceId}/workload/{workloadId}/vscode/", ctrl.sessionManager.Authenticated(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
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

		_, workload, err := ctrl.repo.WorkloadById(request.Context(), workspaceId, workloadId)
		if err != nil {
			http.Error(writer, "invalid workloadId", http.StatusBadRequest)
			return
		}

		prefix := fmt.Sprintf(
			"/-/workspace/%s/workload/%s/vscode",
			workspaceId.String(),
			workloadId.String(),
		)

		if config, ok := workload.Data.(VscodeWorkloadConfiguration); !ok {
			http.Error(writer, "invalid workloadId", http.StatusBadRequest)
			return
		} else if url, err := config.TargetUrl(); err != nil {
			http.Error(writer, "invalid workloadId", http.StatusBadRequest)
			return
		} else {
			log.Printf("WS request: %s", request.URL.String())
			log.Printf("Upgrade: %s", request.Header.Get("Upgrade"))
			log.Printf("Prefix: %s", prefix)
			log.Printf("Target: %s", url.String())
			proxy := httputil.NewSingleHostReverseProxy(url)
			http.StripPrefix(prefix, proxy).ServeHTTP(writer, request)
		}
	})))

	return nil
}

func sendErrorMessage(w http.ResponseWriter, status int, message string) {
	misc.SendErrorMessage(w, status, message)
}

func sendError(w http.ResponseWriter, err error) {
	if errors.Is(err, NotFound) {
		misc.SendErrorMessage(w, http.StatusNotFound, "not found")
		return
	}
	if errors.Is(err, Forbidden) {
		misc.SendErrorMessage(w, http.StatusForbidden, "das ist nicht erlaubt")
		return
	}
	slog.Default().Warn(err.Error())
	misc.SendErrorMessage(w, http.StatusInternalServerError, "ein fehler ist aufgetrten")
}

func sendJson(w http.ResponseWriter, statusCode int, payload any) {
	misc.SendJson(w, statusCode, payload)
}

type WorkloadResponse struct {
	Id   uuid.UUID `json:"id"`
	Name string    `json:"name"`
	Type string    `json:"type"`
	Host string    `json:"host,omitempty"`
	Port int       `json:"port,omitempty"`
}

type VscodeWorkloadResponse struct {
	Id       uuid.UUID `json:"id"`
	Type     string    `json:"type"`
	Host     string    `json:"host"`
	Port     int       `json:"port"`
	Password string    `json:"password,omitempty"`
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
			GitUrl: project.GitUrl,
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

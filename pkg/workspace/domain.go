package workspace

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"uuid"

	"gtihub.com/mwildt/git-works/pkg/actions"
	"gtihub.com/mwildt/git-works/pkg/config"
	events2 "gtihub.com/mwildt/git-works/pkg/events"
	"gtihub.com/mwildt/git-works/pkg/git"
	"gtihub.com/mwildt/git-works/pkg/gitlab"
	"gtihub.com/mwildt/git-works/pkg/misc"
)

type (
	Workspace struct {
		Id        uuid.UUID
		Name      string
		Owner     string
		Projects  []Project
		Workloads []Workload
		Created   time.Time
		Opencode  OpencodeWorkloadConfiguration
		events    []events2.DomainEvent
	}

	Workload struct {
		Id          uuid.UUID
		WorkspaceId uuid.UUID
		Type        string
		Data        any
		StartAction string
		StopAction  string
	}

	SSHWorkloadConfiguration struct {
		Key  string
		Host string
		Port int
	}

	OpencodeWorkloadConfiguration struct {
		Key          string
		AuthUsername string
		AuthPassword string
		Host         string
		Port         int
	}

	Project struct {
		Id     uuid.UUID `json:"id"`
		Path   string    `json:"path,omitempty"`
		GitUrl string    `json:"git_url,omitempty"`
		Branch string    `json:"branch,omitempty"`
	}
)

func NewWorkspace(owner string) *Workspace {
	id := uuid.New()
	name := misc.RandomName()
	return &Workspace{
		Id:        id,
		Owner:     owner,
		Name:      name,
		Created:   time.Now(),
		Projects:  []Project{},
		Workloads: []Workload{},
		events: []events2.DomainEvent{
			&WorkspaceCreated{
				Id:      id,
				Name:    name,
				Owner:   owner,
				Created: time.Now(),
			},
		},
	}
}

func (workload Workload) WorkloadType() string {
	return workload.Type
}

func (workload Workload) Status(opencodeActions *actions.Client) (string, error) {
	switch config := workload.Data.(type) {
	case OpencodeWorkloadConfiguration:
		params := map[string]string{
			"workspaceId":      workload.WorkspaceId.String(),
			"workloadId":       workload.Id.String(),
			"name":             config.Key,
			"port":             strconv.Itoa(config.Port),
			"oc_password":      config.AuthPassword,
			"oc_username":      config.AuthUsername,
			"oc_authorization": fmt.Sprintf("Basic %s", config.BasicAuth()),
		}
		res, err := opencodeActions.Send(context.Background(), "OPENCODE_WORKSPACE_HEALTH", params)
		if err != nil {
			slog.Default().Warn("unable to run OPENCODE_WORKSPACE_HEALTH", "error", err.Error())
			return "down", nil
		} else if res.StatusCode != 200 {
			body, _ := io.ReadAll(res.Body)
			slog.Default().Warn("status != 200 when running OPENCODE_WORKSPACE_HEALTH", "status", res.StatusCode, "code", res.StatusCode, "body", string(body))
			return "down", nil
		}
		return "up", nil
	case SSHWorkloadConfiguration:
		address := net.JoinHostPort(config.Host, strconv.Itoa(config.Port))
		conn, err := net.DialTimeout("tcp", address, 1*time.Second)
		if err != nil {
			return "down", nil
		}
		defer conn.Close()
		return "up", nil
	}
	return "", errors.New("invalid workload type")
}

func (workload Workload) Start(token gitlab.Token, actions *actions.Client, workspaceId uuid.UUID) error {
	return workload.Action(token, actions, workload.StartAction, workspaceId)
}

func (workload Workload) Stop(token gitlab.Token, actions *actions.Client, workspaceId uuid.UUID) error {
	return workload.Action(token, actions, workload.StopAction, workspaceId)
}

func (workload Workload) Action(token gitlab.Token, actions *actions.Client, action string, workspaceId uuid.UUID) error {
	slog.Default().Info("Run workspace action", "action", action, "id", workload.Id.String(), "workspace", workspaceId.String())

	parameter := map[string]string{
		"workspaceId": workspaceId.String(),
		"workloadId":  workload.Id.String(),
		"token_type":  token.TokenType,
		"token_value": token.Value,
	}

	switch config := workload.Data.(type) {
	case OpencodeWorkloadConfiguration:
		parameter["name"] = config.Key
		parameter["host"] = config.Host
		parameter["port"] = strconv.Itoa(config.Port)
		parameter["oc_username"] = config.AuthUsername
		parameter["oc_password"] = config.AuthPassword
		parameter["oc_authorization"] = fmt.Sprintf("Basic %s", config.BasicAuth())

	case SSHWorkloadConfiguration:
		parameter["name"] = config.Key
		parameter["host"] = config.Host
		parameter["port"] = strconv.Itoa(config.Port)
	}

	res, err := actions.Send(context.Background(), action, parameter)

	if err != nil {
		slog.Default().Error("Failed to start workspace", "id", workload.Id.String(), "err", err, "params", parameter)
		return err
	}
	if res.StatusCode >= 300 {
		slog.Default().Error("Failed to start workspace", "id", workload.Id.String(), "status_code", res.StatusCode)
		return errors.New(res.Status)
	}
	return nil
}

func (workload Workload) Init(basedir string) error {
	switch workload.Data.(type) {
	case OpencodeWorkloadConfiguration:
		source := config.GetEnv("OPENCODE_TEMPLATE_FILE", "opencode.json")
		slog.Default().Info("init workload", "workloadid", workload.Id.String(), "basedir", basedir, "config-source", source)
		opencodeConfig, err := os.ReadFile(source)
		if err != nil {
			return err
		}
		opencodeFile := path.Join(basedir, fmt.Sprintf("opencode.%s.json", workload.Id.String()))
		if err := os.WriteFile(opencodeFile, opencodeConfig, 0644); err != nil {
			slog.Default().Error("Error creating opencode file", "error", err)
			return err
		}

	case SSHWorkloadConfiguration:
		slog.Default().Info("init workload", "workloadid", workload.Id.String(), "basedir", basedir)
	}

	return nil
}

func (c OpencodeWorkloadConfiguration) BasicAuth() any {
	return base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", c.AuthUsername, c.AuthPassword)))
}

func (c OpencodeWorkloadConfiguration) TargetUrl() string {
	return fmt.Sprintf("http://%s:%d/", c.Host, c.Port)
}

func (p Project) Init(token gitlab.Token, client *git.Git, workspace string) error {
	return client.Clone(token, p.GitUrl, path.Join(workspace, p.Path), p.Branch)
}

func (workspace *Workspace) addProject(path string, gitUrl string, branch string) error {

	for _, project := range workspace.Projects {
		if project.Path == path {
			return fmt.Errorf("cannot add project with same path")
		}
		if IsSubPath(path, project.Path) {
			return fmt.Errorf("cannot add project with same path")
		}
		if IsSubPath(project.Path, path) {
			return fmt.Errorf("cannot add project with same path")
		}
	}

	event := &ProjectCreated{
		Id:     uuid.New(),
		Path:   path,
		GitUrl: gitUrl,
		Branch: branch,
	}
	workspace.apply(event)
	workspace.events = append(workspace.events, event)
	return nil
}

func (workspace *Workspace) deleteProjectById(value uuid.UUID) bool {
	event := &ProjectDeleted{Id: value}
	workspace.apply(event)
	workspace.events = append(workspace.events, event)
	return true
}

func (workspace *Workspace) deleteWorkloadById(value uuid.UUID) bool {
	event := &OpencodeWorkloadDeleted{Id: value}
	workspace.apply(event)
	workspace.events = append(workspace.events, event)
	return true
}

func (workspace *Workspace) baseDir() string {
	return path.Join(
		config.GetEnv("DATA_DIR", "./data"),
		workspace.Id.String(),
	)
}

func (workspace *Workspace) workspaceDir() string {
	return path.Join(
		workspace.baseDir(),
		"workspace",
	)
}

func (workspace *Workspace) Init(token gitlab.Token, client *git.Git) error {
	workspacePath := workspace.workspaceDir()
	for _, project := range workspace.Projects {
		if err := project.Init(token, client, workspacePath); err != nil {
			return err
		}
	}
	return nil
}

func (workspace *Workspace) ProjectById(id uuid.UUID) (Project, bool) {
	for _, project := range workspace.Projects {
		if project.Id == id {
			return project, true
		}
	}
	return Project{}, false
}

func (workspace *Workspace) WorkloadById(id uuid.UUID) (Workload, bool) {
	for _, workload := range workspace.Workloads {
		if workload.Id == id {
			return workload, true
		}
	}
	return Workload{}, false
}

func (workspace *Workspace) HasOwner(owner string) bool {
	return workspace.Owner == owner
}

func (workspace *Workspace) apply(event any) {
	switch e := event.(type) {
	case *WorkspaceCreated:
		workspace.Id = e.Id
		workspace.Owner = e.Owner
		workspace.Name = e.Name
		workspace.Created = e.Created

	case *ProjectCreated:
		workspace.Projects = append(workspace.Projects, Project{
			Id:     e.Id,
			Path:   e.Path,
			GitUrl: e.GitUrl,
			Branch: e.Branch,
		})

	case *ProjectDeleted:
		projects := make([]Project, 0, len(workspace.Projects))
		for _, project := range workspace.Projects {
			if project.Id != e.Id {
				projects = append(projects, project)
			}
		}
		workspace.Projects = projects

	case *OpencodeWorkloadCreated:
		workspace.Workloads = append(workspace.Workloads, Workload{
			Id:          e.Id,
			WorkspaceId: workspace.Id,
			Type:        e.Type,
			Data:        e.Data,
			StartAction: "OPENCODE_START",
			StopAction:  "OPENCODE_STOP",
		})

	case *OpencodeWorkloadDeleted:
		workloads := make([]Workload, 0, len(workspace.Workloads))
		for _, workload := range workspace.Workloads {
			if workload.Id != e.Id {
				workloads = append(workloads, workload)
			}
		}
		workspace.Workloads = workloads

	case *SSHWorkloadCreated:
		workspace.Workloads = append(workspace.Workloads, Workload{
			Id:          e.Id,
			WorkspaceId: workspace.Id,
			Type:        "SSH",
			Data: SSHWorkloadConfiguration{
				Key:  e.Key,
				Host: e.Host,
				Port: e.Port,
			},
			StartAction: "SSH_START",
			StopAction:  "SSH_STOP",
		})
	}
}

func (workspace *Workspace) clearEvents() {
	workspace.events = make([]events2.DomainEvent, 0)
}

func (workspace *Workspace) CreateOpencodeWorkload(host string, portManager *PortManager) error {
	id := uuid.New()

	port, err := portManager.Reserve(id, host)
	if err != nil {
		return err
	}
	event := &OpencodeWorkloadCreated{
		Id:          id,
		WorkspaceId: workspace.Id,
		Type:        "OpenCode",
		Data: OpencodeWorkloadConfiguration{
			Key:          "workspaces_opencode_" + id.String()[0:10],
			AuthUsername: "git-works",
			AuthPassword: "password",
			Host:         host,
			Port:         port,
		},
	}
	workspace.apply(event)
	workspace.events = append(workspace.events, event)
	return nil
}

func (workspace *Workspace) CreateSshWorkload(host string, portManager *PortManager) error {
	id := uuid.New()

	port, err := portManager.Reserve(id, host)
	if err != nil {
		return err
	}

	event := &SSHWorkloadCreated{
		Id:          id,
		WorkspaceId: workspace.Id,
		Port:        port,
		Host:        host,
		Key:         "workspaces_ssh_" + id.String()[0:10],
	}
	workspace.apply(event)
	workspace.events = append(workspace.events, event)
	return nil

}

func IsSubPath(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}

	return rel != ".." &&
		!strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

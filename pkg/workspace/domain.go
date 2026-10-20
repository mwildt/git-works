package workspace

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"
	"strconv"
	"text/template"
	"time"
	"uuid"

	"gtihub.com/mwildt/git-works/pkg/actions"
	"gtihub.com/mwildt/git-works/pkg/config"
	"gtihub.com/mwildt/git-works/pkg/git"
	"gtihub.com/mwildt/git-works/pkg/gitlab"
	"gtihub.com/mwildt/git-works/pkg/misc"
)

type (
	Workload struct {
		Id          uuid.UUID
		WorkspaceId uuid.UUID
		Type        string
		Data        OpencodeConfig
	}

	Project struct {
		Id     uuid.UUID `json:"id"`
		Path   string    `json:"path,omitempty"`
		GitUrl string    `json:"git_url,omitempty"`
		Branch string    `json:"branch,omitempty"`
	}

	OpencodeConfig struct {
		Key          string
		AuthUsername string
		AuthPassword string
		Host         string
		Port         int
	}

	Workspace struct {
		Id        uuid.UUID
		Name      string
		Owner     string
		Projects  []Project
		Workloads []Workload
		Created   time.Time
		Opencode  OpencodeConfig
	}
)

func NewWorkspace(owner string) Workspace {
	return Workspace{
		Id:        uuid.NewV4(),
		Owner:     owner,
		Name:      misc.RandomName(),
		Created:   time.Now(),
		Projects:  []Project{},
		Workloads: []Workload{},
	}
}

func (workload Workload) WorkloadType() string {
	return workload.Type
}

func (workload Workload) Status(opencodeActions *actions.Client) (string, error) {
	if workload.Type != "OpenCode" {
		return "", errors.New("invalid workspace type")
	}
	ocConfig := workload.Data

	res, err := opencodeActions.Send(context.Background(), "OPENCODE_WORKSPACE_HEALTH", map[string]string{
		"workspaceId":      workload.WorkspaceId.String(),
		"workloadId":       workload.Id.String(),
		"name":             ocConfig.Key,
		"port":             strconv.Itoa(ocConfig.Port),
		"oc_password":      ocConfig.AuthPassword,
		"oc_username":      ocConfig.AuthUsername,
		"oc_authorization": fmt.Sprintf("Basic %s", ocConfig.BasicAuth()),
	})
	if err != nil {
		slog.Default().Warn("unable to run OPENCODE_WORKSPACE_HEALTH", "error", err.Error())
		return "down", nil
	} else if res.StatusCode != 200 {
		body, _ := io.ReadAll(res.Body)
		slog.Default().Warn("status != 200 when running OPENCODE_WORKSPACE_HEALTH", "status", res.StatusCode, "code", res.StatusCode, "body", string(body))
		return "down", nil
	}
	return "up", nil
}

func (workload Workload) Start(token gitlab.Token, actions *actions.Client, workspaceId uuid.UUID) error {
	return workload.Action(token, actions, "OPENCODE_START", workspaceId)
}

func (workload Workload) Stop(token gitlab.Token, actions *actions.Client, workspaceId uuid.UUID) error {
	return workload.Action(token, actions, "OPENCODE_STOP", workspaceId)
}

func (workload Workload) Action(token gitlab.Token, actions *actions.Client, action string, workspaceId uuid.UUID) error {
	if workload.Type != "OpenCode" {
		return errors.New("invalid workspace type")
	}
	ocConfig := workload.Data
	slog.Default().Info("Run workspace action", "action", action, "name", ocConfig.Key, "id", workload.Id.String(), "port", ocConfig.Port)
	res, err := actions.Send(context.Background(), action, map[string]string{
		"workspaceId": workspaceId.String(),
		"workloadId":  workload.Id.String(),
		"name":        ocConfig.Key,
		"port":        strconv.Itoa(ocConfig.Port),
		"oc_password": ocConfig.AuthPassword,
		"oc_username": ocConfig.AuthUsername,
		"token_type":  token.TokenType,
		"token_value": token.Value,
	})
	if err != nil {
		slog.Default().Error("Failed to start workspace", "name", ocConfig.Key, "id", workload.Id.String(), "err", err)
		return err
	}
	if res.StatusCode >= 300 {
		slog.Default().Error("Failed to start workspace", "name", ocConfig.Key, "id", workload.Id.String(), "status_code", res.StatusCode)
		return errors.New(res.Status)
	}
	return nil
}

func (workload Workload) Init(basedir string) error {

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
	return nil
}

func (c OpencodeConfig) BasicAuth() any {
	return base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", c.AuthUsername, c.AuthPassword)))
}

func (c OpencodeConfig) TargetUrl() string {
	return fmt.Sprintf("http://%s:%d/", c.Host, c.Port)
}

func (p Project) Init(token gitlab.Token, client *git.Git, workspace string) error {
	return client.Clone(token, p.GitUrl, path.Join(workspace, p.Path), p.Branch)
}

func (w *Workspace) deleteProjectById(value uuid.UUID) bool {
	for i, project := range w.Projects {
		if project.Id == value {
			w.Projects = append(w.Projects[:i], w.Projects[i+1:]...)
			return true
		}
	}
	return false
}

func (w *Workspace) deleteWorkloadById(value uuid.UUID) bool {
	for i, workload := range w.Workloads {
		if workload.Id == value {
			w.Workloads = append(w.Workloads[:i], w.Workloads[i+1:]...)
			return true
		}
	}
	return false
}

func (w *Workspace) baseDir() string {
	return path.Join(
		config.GetEnv("DATA_DIR", "./data"),
		w.Id.String(),
	)
}

func (w *Workspace) workspaceDir() string {
	return path.Join(
		w.baseDir(),
		"workspace",
	)
}

func (w *Workspace) Init(token gitlab.Token, client *git.Git) error {
	workspacePath := w.workspaceDir()
	for _, project := range w.Projects {
		if err := project.Init(token, client, workspacePath); err != nil {
			return err
		}
	}
	return nil
}

func (w *Workspace) ProjectById(id uuid.UUID) (Project, bool) {
	for _, project := range w.Projects {
		if project.Id == id {
			return project, true
		}
	}
	return Project{}, false
}

func (w *Workspace) WorkloadById(id uuid.UUID) (Workload, bool) {
	for _, workload := range w.Workloads {
		if workload.Id == id {
			return workload, true
		}
	}
	return Workload{}, false
}

func (w Workload) TargetUrl() string {
	urlTemplate := config.GetEnv("OPENCODE_TARGET_URL", "http://{{.host}}:{{.port}}/")
	t := template.Must(template.New("tpl").Parse(urlTemplate))
	var buf bytes.Buffer
	t.Execute(&buf, map[string]any{
		"host":        w.Data.Host,
		"port":        strconv.Itoa(w.Data.Port),
		"workspaceId": w.Id.String(),
	})
	return buf.String()
}

func (w *Workspace) HasOwner(owner string) bool {
	return w.Owner == owner
}

package workspace

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path"
	"strconv"
	"text/template"
	"time"
	"uuid"

	"gtihub.com/mwildt/git-works/pkg/actions"
	"gtihub.com/mwildt/git-works/pkg/config"
	"gtihub.com/mwildt/git-works/pkg/git"
	"gtihub.com/mwildt/git-works/pkg/gitlab"
)

type (
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
		Id       uuid.UUID
		Name     string
		Owner    string
		Projects []Project
		Created  time.Time
		Opencode OpencodeConfig
	}

	ProjectStatus struct {
		ProjectId   uuid.UUID `json:"project_id"`
		Initialized bool      `json:"initialized"`
		HasChanges  bool      `json:"has_changes"`
	}
)

func (c OpencodeConfig) BasicAuth() any {
	return base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", c.AuthUsername, c.AuthPassword)))
}

func (c OpencodeConfig) TargetUrl() string {
	return fmt.Sprintf("http://%s:%d/", c.Host, c.Port)
}

func (p Project) Init(token gitlab.Token, client *git.Git, workspace string) error {
	return client.Clone(token, p.GitUrl, path.Join(workspace, p.Path), p.Branch)
}

func (w *Workspace) addProject(path string, gitUrl string, branch string) {
	w.Projects = append(w.Projects, Project{Path: path, GitUrl: gitUrl, Branch: branch, Id: uuid.New()})
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

func (w *Workspace) Start(token gitlab.Token, client *actions.Client) error {
	slog.Default().Info("Starting workspace", "name", w.Name, "id", w.Id.String(), "port", w.Opencode.Port)
	res, err := client.Send(context.Background(), "OPENCODE_START", map[string]string{
		"wsid":        w.Id.String(),
		"name":        w.Opencode.Key,
		"port":        strconv.Itoa(w.Opencode.Port),
		"oc_password": w.Opencode.AuthPassword,
		"oc_username": w.Opencode.AuthUsername,
		"token_type":  token.TokenType,
		"token_value": token.Value,
	})
	if err != nil {
		slog.Default().Error("Failed to start workspace", "name", w.Name, "id", w.Id.String(), "err", err)
		return err
	}
	if res.StatusCode >= 300 {
		slog.Default().Error("Failed to start workspace", "name", w.Name, "id", w.Id.String(), "status_code", res.StatusCode)
		return errors.New(res.Status)
	}
	return nil
}

func (w *Workspace) Stop(token gitlab.Token, client *actions.Client) error {
	slog.Default().Info("Starting workspace", "name", w.Name, "id", w.Id.String(), "port", w.Opencode.Port)
	res, err := client.Send(context.Background(), "OPENCODE_STOP", map[string]string{
		"wsid":        w.Id.String(),
		"name":        w.Opencode.Key,
		"port":        strconv.Itoa(w.Opencode.Port),
		"oc_password": w.Opencode.AuthPassword,
		"oc_username": w.Opencode.AuthUsername,
		"token_type":  token.TokenType,
		"token_value": token.Value,
	})
	if err != nil {
		slog.Default().Error("Failed to start workspace", "name", w.Name, "id", w.Id.String(), "err", err)
		return err
	}
	if res.StatusCode >= 300 {
		slog.Default().Error("Failed to start workspace", "name", w.Name, "id", w.Id.String(), "status_code", res.StatusCode)
		return errors.New(res.Status)
	}
	return nil
}

func (w *Workspace) Status(opencodeActions *actions.Client) string {
	res, err := opencodeActions.Send(context.Background(), "OPENCODE_WORKSPACE_HEALTH", map[string]string{
		"wsid":             w.Id.String(),
		"name":             w.Opencode.Key,
		"port":             strconv.Itoa(w.Opencode.Port),
		"oc_password":      w.Opencode.AuthPassword,
		"oc_username":      w.Opencode.AuthUsername,
		"oc_authorization": fmt.Sprintf("Basic %s", w.Opencode.BasicAuth()),
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
}

func (w *Workspace) TargetUrl() string {
	urlTemplate := config.GetEnv("OPENCODE_TARGET_URL", "http://{{.host}}:{{.port}}/")
	t := template.Must(template.New("tpl").Parse(urlTemplate))
	var buf bytes.Buffer
	t.Execute(&buf, map[string]any{
		"host": w.Opencode.Host,
		"port": strconv.Itoa(w.Opencode.Port),
		"wsid": w.Id.String(),
	})
	return buf.String()
}

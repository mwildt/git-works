package workspace

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"strconv"
	"time"
	"uuid"

	"gtihub.com/mwildt/git-works/pkg/actions"
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

func (w *Workspace) AddProject(path string, gitUrl string, branch string) {
	w.Projects = append(w.Projects, Project{Path: path, GitUrl: gitUrl, Branch: branch, Id: uuid.New()})
}

func (w *Workspace) DeleteProjectById(value uuid.UUID) bool {
	for i, project := range w.Projects {
		if project.Id == value {
			w.Projects = append(w.Projects[:i], w.Projects[i+1:]...)
			return true
		}
	}
	return false
}

func (w *Workspace) Init(token gitlab.Token, client *git.Git) error {
	workspacePath := path.Join("data", w.Id.String(), "workspace")
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

func (w *Workspace) Start(client *actions.Client) error {
	slog.Default().Info("Starting workspace", "name", w.Name, "id", w.Id.String(), "port", w.Opencode.Port)
	res, err := client.Send(context.Background(), "OPENCODE_START", map[string]string{
		"wsid":        w.Id.String(),
		"name":        w.Opencode.Key,
		"port":        strconv.Itoa(w.Opencode.Port),
		"oc_password": w.Opencode.AuthPassword,
		"oc_username": w.Opencode.AuthUsername,
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

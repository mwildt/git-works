package workspace

import (
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"
	"uuid"

	"gtihub.com/mwildt/git-works/pkg/config"
	"gtihub.com/mwildt/git-works/pkg/misc"
)

type (
	Manager struct {
		items []*Workspace
	}
)

func NewManager() Manager {
	return Manager{
		items: []*Workspace{},
	}
}

func (m *Manager) Create(owner string) *Workspace {
	id := uuid.New()
	ws := &Workspace{
		Id:       id,
		Owner:    owner,
		Name:     misc.RandomName(),
		Created:  time.Now(),
		Projects: []Project{},
		Opencode: OpencodeConfig{
			Key:          "opencode_container_" + id.String()[0:10],
			AuthUsername: "git-works",
			AuthPassword: "password",
			Host:         config.GetEnv("OPENCODE_REMOTE_HOST", "127.0.0.1"),
			Port:         m.nextPort(),
		},
	}
	m.items = append(m.items, ws)
	go func() {
		if err := os.MkdirAll(ws.workspaceDir(), os.ModePerm); err != nil {
			slog.Default().Error("failed to create workspace directory", "error", err)
		}
		opencodeConfig := []byte("{}")

		opencodeFile := path.Join(ws.baseDir(), "opencode.json")
		if err := os.WriteFile(opencodeFile, opencodeConfig, 0644); err != nil {
			slog.Default().Error("Error creating opencode file", "error", err)
		}
	}()
	return ws
}

func (m *Manager) ByIdAndOwner(id uuid.UUID, owner string) (res *Workspace, found bool) {
	for _, w := range m.items {
		if w.Owner == owner && w.Id == id {
			return w, true
		}
	}
	return res, false
}

func (m *Manager) AllByOwner(owner string) (res []*Workspace) {
	for _, w := range m.items {
		if w.Owner == owner {
			res = append(res, w)
		}
	}
	return res
}

func (m *Manager) DeleteByIdAndOwner(id uuid.UUID, owner string) bool {
	for i, w := range m.items {
		if w.Owner == owner && w.Id == id {
			m.items = append(m.items[:i], m.items[i+1:]...)
			return true
		}
	}
	return false
}

func (m *Manager) GetProjectStatus(username string, workspaceId uuid.UUID, projectId uuid.UUID) (res ProjectStatus, _ error) {
	w, ok := m.ByIdAndOwner(workspaceId, username)
	if !ok {
		return res, errors.New("not found")
	}
	project, ok := w.ProjectById(projectId)
	if !ok {
		return res, errors.New("not found")
	}

	return ProjectStatus{
		ProjectId:   project.Id,
		Initialized: isGitRepo(path.Join(w.workspaceDir(), project.Path)),
		HasChanges:  hasGitChanges(path.Join(w.workspaceDir(), project.Path)),
	}, nil

}

func (m *Manager) nextPort() int {
	port := 9000
	for _, ws := range m.items {
		if ws.Opencode.Port >= port {
			port = ws.Opencode.Port + 1
		}
	}
	return port
}

func isGitRepo(dir string) bool {
	info, err := os.Stat(path.Join(dir, ".git"))
	return err == nil && info.IsDir()
}

func hasGitChanges(dir string) bool {
	if !isGitRepo(dir) {
		return false
	}
	cmd := exec.Command("git", "-C", dir, "status", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		slog.Warn("failed to check git status", "dir", dir, "error", err)
		return false
	}
	return len(strings.TrimSpace(string(output))) > 0
}

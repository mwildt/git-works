package workspace

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"
	"uuid"

	"gtihub.com/mwildt/git-works/pkg/config"
	"gtihub.com/mwildt/git-works/pkg/misc"
	"gtihub.com/mwildt/git-works/pkg/storage"
)

type (
	eventlog struct {
		file     string
		appender storage.Appender
	}

	Manager struct {
		items map[string]*Workspace
		log   *eventlog
	}

	Event struct {
		Eventtype string
		Event     json.RawMessage
	}
)

func NewManager() (*Manager, error) {
	manager := Manager{
		items: make(map[string]*Workspace),
	}

	if os.Getenv("WORSPACE_LOG") != "" {
		file := path.Join(os.Getenv("WORSPACE_LOG"))
		slog.Default().Info("create workspace log", "file", file)

		appender, err := storage.NewFileAppender(file)
		if err != nil {
			return nil, err
		}
		manager.log = &eventlog{
			file:     path.Join(os.Getenv("WORSPACE_LOG")),
			appender: appender,
		}

		if scan, err := storage.NewFileScanner(file); err != nil {
			return nil, err
		} else if err = scan.Replay(storage.Consuming(manager.handleLog)); err != nil && err != io.EOF {
			return nil, err
		}

	}

	return &manager, nil
}

func (m *Manager) handleLog(record storage.OffsetRecord) error {
	var event Event
	if err := json.Unmarshal(record.Record, &event); err != nil {
		return err
	}

	switch event.Eventtype {
	case "workspace.full":
		{
			var ws Workspace
			if err := json.Unmarshal(event.Event, &ws); err != nil {
				slog.Error("error reading event", "type", event.Eventtype, "error", err, "data", event.Event)
				return err
			}
			m.items[ws.Id.String()] = &ws
		}
	case "workspace.delete":
		{
			var wsid uuid.UUID
			if err := json.Unmarshal(event.Event, &wsid); err != nil {
				slog.Error("error reading event", "type", event.Eventtype, "error", err, "data", event.Event)
				return err
			}
			delete(m.items, wsid.String())
		}

	}
	return nil

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
	m.items[ws.Id.String()] = ws
	m.appendEvent("workspace.full", ws)
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
	ws, ok := m.items[id.String()]
	if !ok {
		return nil, false

	}
	if ws.Owner != owner {
		return nil, false
	}
	return ws, true
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
	ws, ok := m.items[id.String()]
	if !ok {
		return false
	}
	if ws.Owner != owner {
		return false
	}
	delete(m.items, ws.Id.String())
	go m.appendEvent("workspace.delete", ws.Id)
	return true
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
	port := config.GetEnvInt("WORKSPACE_START_PORT", 9000)
	for _, ws := range m.items {
		if ws.Opencode.Port >= port {
			port = ws.Opencode.Port + 1
		}
	}
	return port
}

func (m *Manager) appendEvent(eventtype string, ws any) error {
	if m.log != nil {
		payload, err := json.Marshal(ws)
		if err != nil {
			return err
		}

		rawEvent, err := json.Marshal(Event{
			Eventtype: eventtype,
			Event:     payload,
		})
		if err != nil {
			return err
		}
		_, _, err = m.log.appender.Append(rawEvent)
		return err
	}
	return nil

}

func (m *Manager) AddProject(workspace *Workspace, p string, url string, name string) {
	workspace.addProject(p, url, name)
	go m.appendEvent("workspace.full", workspace)
}

func (m *Manager) DeleteProjectById(workspace *Workspace, id uuid.UUID) {
	workspace.deleteProjectById(id)
	go m.appendEvent("workspace.full", workspace)
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

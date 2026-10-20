package workspace

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path"
	"uuid"

	"gtihub.com/mwildt/git-works/pkg/auth"
	"gtihub.com/mwildt/git-works/pkg/storage"
)

type (
	eventlog struct {
		file     string
		appender storage.Appender
	}

	Repository struct {
		items map[string]*Workspace
		log   *eventlog
	}
)

func NewRepository() (*Repository, error) {
	manager := Repository{
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

func (repo *Repository) handleLog(record storage.OffsetRecord) error {
	var event Event
	if err := json.Unmarshal(record.Record, &event); err != nil {
		return err
	}
	switch event.Eventtype {
	case "workspace.full":
		var ws Workspace
		if err := json.Unmarshal(event.Event, &ws); err != nil {
			slog.Error("error reading event", "type", event.Eventtype, "error", err, "data", event.Event)
			return err
		}
		repo.items[ws.Id.String()] = &ws
	case "workspace.delete", EventWorkspaceDeleted:
		var ev WorkspaceDeletedEvent
		if err := json.Unmarshal(event.Event, &ev); err != nil {
			slog.Error("error reading event", "type", event.Eventtype, "error", err, "data", event.Event)
			return err
		}
		delete(repo.items, ev.WorkspaceId.String())
	case EventWorkspaceCreated:
		var ev WorkspaceCreatedEvent
		if err := json.Unmarshal(event.Event, &ev); err != nil {
			slog.Error("error reading event", "type", event.Eventtype, "error", err, "data", event.Event)
			return err
		}
		repo.items[ev.Workspace.Id.String()] = &ev.Workspace
	case EventProjectAdded:
		var ev ProjectAddedEvent
		if err := json.Unmarshal(event.Event, &ev); err != nil {
			slog.Error("error reading event", "type", event.Eventtype, "error", err, "data", event.Event)
			return err
		}
		repo.applyProjectAdded(ev)
	case EventProjectDeleted:
		var ev ProjectDeletedEvent
		if err := json.Unmarshal(event.Event, &ev); err != nil {
			slog.Error("error reading event", "type", event.Eventtype, "error", err, "data", event.Event)
			return err
		}
		repo.applyProjectDeleted(ev)
	case EventWorkloadAdded:
		var ev WorkloadAddedEvent
		if err := json.Unmarshal(event.Event, &ev); err != nil {
			slog.Error("error reading event", "type", event.Eventtype, "error", err, "data", event.Event)
			return err
		}
		repo.applyWorkloadAdded(ev)
	case EventWorkloadDeleted:
		var ev WorkloadDeletedEvent
		if err := json.Unmarshal(event.Event, &ev); err != nil {
			slog.Error("error reading event", "type", event.Eventtype, "error", err, "data", event.Event)
			return err
		}
		repo.applyWorkloadDeleted(ev)
	default:
		slog.Warn("unknown event type", "type", event.Eventtype)
	}
	return nil
}

func (repo *Repository) applyProjectAdded(ev ProjectAddedEvent) {
	ws, ok := repo.items[ev.WorkspaceId.String()]
	if !ok {
		slog.Warn("project.added for unknown workspace", "workspaceId", ev.WorkspaceId.String())
		return
	}
	ws.Projects = append(ws.Projects, ev.Project)
}

func (repo *Repository) applyProjectDeleted(ev ProjectDeletedEvent) {
	ws, ok := repo.items[ev.WorkspaceId.String()]
	if !ok {
		slog.Warn("project.deleted for unknown workspace", "workspaceId", ev.WorkspaceId.String())
		return
	}
	ws.deleteProjectById(ev.ProjectId)
}

func (repo *Repository) applyWorkloadAdded(ev WorkloadAddedEvent) {
	ws, ok := repo.items[ev.WorkspaceId.String()]
	if !ok {
		slog.Warn("workload.added for unknown workspace", "workspaceId", ev.WorkspaceId.String())
		return
	}
	ws.Workloads = append(ws.Workloads, ev.Workload)
}

func (repo *Repository) applyWorkloadDeleted(ev WorkloadDeletedEvent) {
	ws, ok := repo.items[ev.WorkspaceId.String()]
	if !ok {
		slog.Warn("workload.deleted for unknown workspace", "workspaceId", ev.WorkspaceId.String())
		return
	}
	ws.deleteWorkloadById(ev.WorkloadId)
}

func (repo *Repository) CreateWorkspace(ctx context.Context, workspace Workspace) error {
	owner, ok := auth.Owner(ctx)
	if !ok {
		return Forbidden
	}
	if !workspace.HasOwner(owner) {
		return Forbidden
	}
	repo.items[workspace.Id.String()] = &workspace
	return repo.appendEvent(EventWorkspaceCreated, WorkspaceCreatedEvent{Workspace: workspace})
}

func (repo *Repository) AddProject(ctx context.Context, workspaceId uuid.UUID, project Project) error {
	ws, err := repo.itemById(ctx, workspaceId)
	if err != nil {
		return err
	}
	ws.Projects = append(ws.Projects, project)
	if err := repo.appendEvent(EventProjectAdded, ProjectAddedEvent{WorkspaceId: workspaceId, Project: project}); err != nil {
		ws.deleteProjectById(project.Id)
		return err
	}
	return nil
}

func (repo *Repository) DeleteProject(ctx context.Context, workspaceId uuid.UUID, projectId uuid.UUID) error {
	ws, err := repo.itemById(ctx, workspaceId)
	if err != nil {
		return err
	}
	project, ok := ws.ProjectById(projectId)
	if !ok {
		return NotFound
	}
	ws.deleteProjectById(projectId)
	if err := repo.appendEvent(EventProjectDeleted, ProjectDeletedEvent{WorkspaceId: workspaceId, ProjectId: projectId}); err != nil {
		ws.Projects = append(ws.Projects, project)
		return err
	}
	return nil
}

func (repo *Repository) AddWorkload(ctx context.Context, workspaceId uuid.UUID, workload Workload) error {
	ws, err := repo.itemById(ctx, workspaceId)
	if err != nil {
		return err
	}
	ws.Workloads = append(ws.Workloads, workload)
	if err := repo.appendEvent(EventWorkloadAdded, WorkloadAddedEvent{WorkspaceId: workspaceId, Workload: workload}); err != nil {
		ws.deleteWorkloadById(workload.Id)
		return err
	}
	return nil
}

func (repo *Repository) DeleteWorkload(ctx context.Context, workspaceId uuid.UUID, workloadId uuid.UUID) error {
	ws, err := repo.itemById(ctx, workspaceId)
	if err != nil {
		return err
	}
	workload, ok := ws.WorkloadById(workloadId)
	if !ok {
		return NotFound
	}
	ws.deleteWorkloadById(workloadId)
	if err := repo.appendEvent(EventWorkloadDeleted, WorkloadDeletedEvent{WorkspaceId: workspaceId, WorkloadId: workloadId}); err != nil {
		ws.Workloads = append(ws.Workloads, workload)
		return err
	}
	return nil
}

func (repo *Repository) Delete(ctx context.Context, workspaceId uuid.UUID) error {
	workspace, err := repo.ById(ctx, workspaceId)
	if err != nil {
		return err
	}
	delete(repo.items, workspace.Id.String())
	return repo.appendEvent(EventWorkspaceDeleted, WorkspaceDeletedEvent{WorkspaceId: workspace.Id})
}

func (repo *Repository) ById(ctx context.Context, workspaceId uuid.UUID) (workspace Workspace, err error) {
	if owner, ok := auth.Owner(ctx); !ok {
		return Workspace{}, Forbidden
	} else if workspace, ok := repo.items[workspaceId.String()]; !ok {
		return Workspace{}, NotFound
	} else if workspace.Owner != owner {
		return Workspace{}, NotFound
	} else {
		return *workspace, nil
	}
}

func (repo *Repository) itemById(ctx context.Context, workspaceId uuid.UUID) (*Workspace, error) {
	owner, ok := auth.Owner(ctx)
	if !ok {
		return nil, Forbidden
	}
	ws, ok := repo.items[workspaceId.String()]
	if !ok {
		return nil, NotFound
	}
	if ws.Owner != owner {
		return nil, NotFound
	}
	return ws, nil
}

func (repo *Repository) ProjectById(ctx context.Context, workspaceId uuid.UUID, pid uuid.UUID) (_ Workspace, _ Project, err error) {
	if workspace, err := repo.ById(ctx, workspaceId); err != nil {
		return Workspace{}, Project{}, err
	} else if project, ok := workspace.ProjectById(pid); !ok {
		return Workspace{}, Project{}, NotFound
	} else {
		return workspace, project, nil
	}
}

func (repo *Repository) WorkloadById(ctx context.Context, workspaceId uuid.UUID, pid uuid.UUID) (_ Workspace, _ Workload, err error) {
	if workspace, err := repo.ById(ctx, workspaceId); err != nil {
		return Workspace{}, Workload{}, err
	} else if workload, ok := workspace.WorkloadById(pid); !ok {
		return Workspace{}, Workload{}, NotFound
	} else {
		return workspace, workload, nil
	}
}

func (repo *Repository) All(ctx context.Context) (res []*Workspace, err error) {
	if owner, ok := auth.Owner(ctx); !ok {
		return res, Forbidden
	} else {
		for _, w := range repo.items {
			if w.Owner == owner {
				res = append(res, w)
			}
		}
		return res, nil
	}
}

// TODO: DAs hier könnte man besser anhand einer ROlle machen -> Aber fürs erste gehts
func (repo *Repository) all() (res []Workspace) {
	for _, ws := range repo.items {
		res = append(res, *ws)
	}
	return res
}

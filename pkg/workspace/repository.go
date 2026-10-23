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

	Event struct {
		Eventtype string
		Event     json.RawMessage
	}
)

func NewRepository() (*Repository, error) {
	manager := Repository{
		items: make(map[string]*Workspace),
	}

	if os.Getenv("WORKSPACE_LOG") != "" {
		file := path.Join(os.Getenv("WORKSPACE_LOG"))
		slog.Default().Info("create workspace log", "file", file)

		appender, err := storage.NewFileAppender(file)
		if err != nil {
			return nil, err
		}
		manager.log = &eventlog{
			file:     path.Join(os.Getenv("WORKSPACE_LOG")),
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
		{
			var ws Workspace
			if err := json.Unmarshal(event.Event, &ws); err != nil {
				slog.Error("error reading event", "type", event.Eventtype, "error", err, "data", event.Event)
				return err
			}
			repo.items[ws.Id.String()] = &ws
		}
	case "workspace.delete":
		{
			var workspaceId uuid.UUID
			if err := json.Unmarshal(event.Event, &workspaceId); err != nil {
				slog.Error("error reading event", "type", event.Eventtype, "error", err, "data", event.Event)
				return err
			}
			delete(repo.items, workspaceId.String())
		}

	}
	return nil
}

func (repo *Repository) Update(ctx context.Context, workspace Workspace) error {
	owner, ok := auth.Owner(ctx)
	if !ok {
		return Forbidden
	}

	if !workspace.HasOwner(owner) {
		return Forbidden
	}

	if existing, err := repo.ById(ctx, workspace.Id); err == nil && !existing.HasOwner(owner) {
		return Forbidden
	}

	repo.items[workspace.Id.String()] = &workspace
	repo.appendEvent("workspace.full", workspace)
	return nil
}

func (repo *Repository) Delete(ctx context.Context, workspaceId uuid.UUID) error {
	workspace, err := repo.ById(ctx, workspaceId)
	if err != nil {
		return err
	}
	delete(repo.items, workspace.Id.String())
	go repo.appendEvent("workspace.delete", workspace.Id)
	return nil
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

func (repo *Repository) appendEvent(eventtype string, ws any) error {
	if repo.log != nil {
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
		_, _, err = repo.log.appender.Append(rawEvent)
		return err
	}
	return nil

}

// TODO: DAs hier könnte man besser anhand einer ROlle machen -> Aber fürs erste gehts
func (repo *Repository) all() (res []Workspace) {
	for _, ws := range repo.items {
		res = append(res, *ws)
	}
	return res
}

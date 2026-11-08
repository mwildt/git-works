package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"
	"uuid"

	"gtihub.com/mwildt/git-works/pkg/auth"
	"gtihub.com/mwildt/git-works/pkg/events"
	"gtihub.com/mwildt/git-works/pkg/storage"
)

type (
	DomainEvent events.DomainEvent

	eventlog struct {
		file     string
		appender storage.Appender
	}

	Repository struct {
		log        *eventlog
		dispatcher *events.Dispatcher
	}

	PersistentEvent struct {
		AggregateId uuid.UUID       `json:"aggregate_id"`
		EventType   string          `json:"type"`
		Timestamp   int64           `json:"time"`
		Data        json.RawMessage `json:"data"`
	}

	envelope struct {
		isReplay    bool
		aggregateId uuid.UUID
		time        time.Time
		event       events.DomainEvent
	}

	collector struct {
		aggregateId uuid.UUID
		aggregate   *Workspace
	}
)

func (e envelope) IsReplay() bool {
	return e.isReplay
}

func (e envelope) AggregateId() uuid.UUID {
	return e.aggregateId
}

func (e envelope) EventType() string {
	return e.event.EventType()
}

func (e envelope) Event() events.DomainEvent {
	return e.event
}

func (e envelope) Time() time.Time {
	return e.time
}

func (c *collector) handle(envelop events.Envelope) error {
	if c.aggregateId != envelop.AggregateId() {
		return nil
	}
	if _, ok := envelop.Event().(*WorkspaceDeleted); ok {
		c.aggregate = nil
		return nil
	}
	if _, ok := envelop.Event().(*WorkspaceCreated); ok {
		c.aggregate = &Workspace{}
	}
	c.aggregate.apply(envelop.Event())
	return nil
}

func (c collector) Get() (*Workspace, error) {
	if nil == c.aggregate {
		return nil, NotFound
	}
	return c.aggregate, nil
}

func NewRepository(file string, dispatcher *events.Dispatcher) (*Repository, error) {

	appender, err := storage.NewFileAppender(file)
	if err != nil {
		return nil, err
	}

	return &Repository{
		log: &eventlog{
			file:     file,
			appender: appender,
		},
		dispatcher: dispatcher,
	}, nil
}

func (repo *Repository) Replay() error {
	if scan, err := storage.NewFileScanner(repo.log.file); err != nil {
		return err
	} else if err = scan.Replay(storage.Consuming(func(record storage.OffsetRecord) error {
		event, err := repo.deserialize(record)
		if err != nil {
			return err
		}
		return repo.dispatcher.Dispatch(context.Background(), event)
	})); err != nil && err != io.EOF {
		return err
	}
	return nil
}

func (repo *Repository) LoadById(_ context.Context, aggregateId uuid.UUID) (*Workspace, error) {
	collector := &collector{aggregateId: aggregateId}
	if scan, err := storage.NewFileScanner(repo.log.file); err != nil {
		return nil, err
	} else if err = scan.Replay(storage.Consuming(func(record storage.OffsetRecord) error {
		if event, err := repo.deserialize(record); err != nil {
			return err
		} else if err = collector.handle(event); err != nil {
			return err
		}
		return nil
	})); err != nil && err != io.EOF {
		return nil, err
	}
	return collector.Get()
}

func (repo *Repository) Save(ctx context.Context, workspace *Workspace) error {
	owner, ok := auth.Owner(ctx)
	if !ok {
		return Forbidden
	}
	if !workspace.HasOwner(owner) {
		return Forbidden
	}
	for _, event := range workspace.events {
		if err := repo.append(ctx, workspace.Id, event); err != nil {
			return err
		}
	}
	workspace.clearEvents()
	return nil
}

func (repo *Repository) Delete(ctx context.Context, workspaceId uuid.UUID) error {
	workspace, err := repo.LoadById(ctx, workspaceId)
	if err != nil {
		return err
	}
	event := WorkspaceDeleted{
		Owner: workspace.Owner,
		Id:    workspace.Id,
	}

	return repo.append(ctx, workspace.Id, event)
}

func (repo *Repository) ProjectById(ctx context.Context, workspaceId uuid.UUID, pid uuid.UUID) (_ *Workspace, _ Project, err error) {
	if workspace, err := repo.LoadById(ctx, workspaceId); err != nil {
		return nil, Project{}, err
	} else if project, ok := workspace.ProjectById(pid); !ok {
		return nil, Project{}, NotFound
	} else {
		return workspace, project, nil
	}
}

func (repo *Repository) WorkloadById(ctx context.Context, workspaceId uuid.UUID, pid uuid.UUID) (_ *Workspace, _ Workload, err error) {
	if workspace, err := repo.LoadById(ctx, workspaceId); err != nil {
		return nil, Workload{}, err
	} else if workload, ok := workspace.WorkloadById(pid); !ok {
		return nil, Workload{}, NotFound
	} else {
		return workspace, workload, nil
	}
}

func (repo *Repository) byId(ctx context.Context, workspaceId uuid.UUID) (workspace Workspace, err error) {
	if owner, ok := auth.Owner(ctx); !ok {
		return Workspace{}, Forbidden
	} else if workspace, err := repo.LoadById(ctx, workspaceId); err != nil {
		return Workspace{}, NotFound
	} else if workspace.Owner != owner {
		return Workspace{}, NotFound
	} else {
		return *workspace, nil
	}
}

func (repo *Repository) append(ctx context.Context, aggregateId uuid.UUID, event events.DomainEvent) error {
	if persistentEvent, err := repo.serialize(aggregateId, event); err != nil {
		return err
	} else if err = repo.write(persistentEvent); err != nil {
		return err
	} else if repo.dispatcher.Dispatch(ctx, envelope{
		isReplay:    false,
		aggregateId: persistentEvent.AggregateId,
		time:        time.UnixMilli(persistentEvent.Timestamp),
		event:       event,
	}); err != nil {
		return err
	}
	return nil
}

func (repo *Repository) deserialize(record storage.OffsetRecord) (events.Envelope, error) {
	var persistentEvent PersistentEvent
	err := json.Unmarshal(record.Record, &persistentEvent)
	if err != nil {
		return nil, err
	}
	factory, ok := map[string]func() DomainEvent{
		EventTypeCreated: func() DomainEvent {
			return &WorkspaceCreated{}
		},
		EventTypeDeleted: func() DomainEvent {
			return &WorkspaceDeleted{}
		},
		ProjectEventTypeCreated: func() DomainEvent {
			return &ProjectCreated{}
		},
		ProjectEventTypeDeleted: func() DomainEvent {
			return &ProjectDeleted{}
		},
		OpencodeWorkloadEventTypeCreated: func() DomainEvent {
			return &OpencodeWorkloadCreated{}
		},
		OpencodeWorkloadEventTypeDeleted: func() DomainEvent {
			return &OpencodeWorkloadDeleted{}
		},
		SSHWorkloadEventTypeCreated: func() DomainEvent {
			return &SSHWorkloadCreated{}
		},
	}[persistentEvent.EventType]

	if !ok {
		return nil, fmt.Errorf("invalid event type %s", persistentEvent.EventType)
	}

	event := factory()

	err = json.Unmarshal(persistentEvent.Data, event)
	if err != nil {
		return nil, err
	}
	return envelope{
		isReplay:    true,
		aggregateId: persistentEvent.AggregateId,
		time:        time.UnixMilli(persistentEvent.Timestamp),
		event:       event,
	}, nil

}

func (repo *Repository) serialize(aggregateId uuid.UUID, event DomainEvent) (PersistentEvent, error) {
	payload, err := json.Marshal(event)
	if err != nil {
		return PersistentEvent{}, err
	}
	return PersistentEvent{
		AggregateId: aggregateId,
		EventType:   event.EventType(),
		Timestamp:   time.Now().UnixMilli(),
		Data:        payload,
	}, nil
}

func (repo *Repository) write(event PersistentEvent) error {
	if repo.log != nil {
		rawEvent, err := json.Marshal(event)
		if err != nil {
			return err
		}
		_, _, err = repo.log.appender.Append(rawEvent)
		return err
	}
	return nil
}

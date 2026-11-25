package workspace

import (
	"context"
	"os"
	"path"
	"testing"
	"time"
	"uuid"

	"gtihub.com/mwildt/git-works/pkg/auth"
	"gtihub.com/mwildt/git-works/pkg/events"
)

type replayEnvelope struct {
	event events.DomainEvent
}

func (e replayEnvelope) IsReplay() bool            { return true }
func (e replayEnvelope) AggregateId() uuid.UUID    { return uuid.Nil() }
func (e replayEnvelope) EventType() string         { return e.event.EventType() }
func (e replayEnvelope) Event() events.DomainEvent { return e.event }
func (e replayEnvelope) Time() time.Time           { return time.Now() }

func TestIdleReaperHandle(t *testing.T) {
	reaper := NewIdleReaper(nil, nil, nil, time.Minute)
	now := time.Now()
	reaper.now = func() time.Time { return now }

	workspaceId := uuid.New()
	workloadId := uuid.New()
	ctx := context.Background()

	// WorkspaceCreated (live und Replay) legt einen Eintrag mit "jetzt" an -
	// nach einem Restart gelten dadurch alle Workspaces als aktiv.
	if err := reaper.Handle(ctx, NewRuntimeEnvelope(workspaceId, &WorkspaceCreated{Id: workspaceId}, now)); err != nil {
		t.Fatalf("handle: %s", err)
	}
	entry, ok := reaper.entries()[workspaceId]
	if !ok || !entry.At.Equal(now) || entry.WorkloadId != uuid.Nil() {
		t.Fatalf("expected entry {nil, %v}, got %+v (ok=%v)", now, entry, ok)
	}
	if err := reaper.Handle(ctx, replayEnvelope{event: &WorkspaceCreated{Id: uuid.New()}}); err != nil {
		t.Fatalf("handle replay: %s", err)
	}

	// WorkloadActivity aktualisiert Workload + Zeitpunkt
	at := now.Add(time.Minute)
	if err := reaper.Handle(ctx, NewRuntimeEnvelope(workspaceId, &WorkloadActivity{WorkspaceId: workspaceId, WorkloadId: workloadId, At: at}, at)); err != nil {
		t.Fatalf("handle: %s", err)
	}
	entry, _ = reaper.entries()[workspaceId]
	if entry.WorkloadId != workloadId || !entry.At.Equal(at) {
		t.Fatalf("expected entry {%v, %v}, got %+v", workloadId, at, entry)
	}

	// Replay-WorkloadActivity wird ignoriert
	if err := reaper.Handle(ctx, replayEnvelope{event: &WorkloadActivity{WorkspaceId: workspaceId, WorkloadId: workloadId, At: at.Add(time.Hour)}}); err != nil {
		t.Fatalf("handle replay: %s", err)
	}
	entry, _ = reaper.entries()[workspaceId]
	if !entry.At.Equal(at) {
		t.Fatalf("replay event must not change entry, got %+v", entry)
	}

	// WorkspaceDeleted entfernt den Eintrag
	if err := reaper.Handle(ctx, NewRuntimeEnvelope(workspaceId, &WorkspaceDeleted{Id: workspaceId}, now)); err != nil {
		t.Fatalf("handle: %s", err)
	}
	if _, ok := reaper.entries()[workspaceId]; ok {
		t.Fatalf("expected entry removed after workspace deleted")
	}
}

func TestWorkloadIdleTimeoutFromDefault(t *testing.T) {
	t.Setenv("WORKLOAD_IDLE_TIMEOUT", "45m")
	workspace := NewWorkspace("malte")
	if err := workspace.CreateOpencodeWorkload("127.0.0.1", NewPortManager(9000)); err != nil {
		t.Fatalf("create workload: %s", err)
	}
	workload := workspace.Workloads[0]
	if workload.IdleTimeout() != 45*time.Minute {
		t.Fatalf("expected idle timeout 45m, got %v", workload.IdleTimeout())
	}
}

func TestWorkloadIdleTimeoutDefault(t *testing.T) {
	t.Setenv("WORKLOAD_IDLE_TIMEOUT", "")
	workspace := NewWorkspace("malte")
	if err := workspace.CreateVscodeWorkload("127.0.0.1", NewPortManager(9000)); err != nil {
		t.Fatalf("create workload: %s", err)
	}
	workload := workspace.Workloads[0]
	if workload.IdleTimeout() != 30*time.Minute {
		t.Fatalf("expected default idle timeout 30m, got %v", workload.IdleTimeout())
	}
}

func TestIdleReaper(t *testing.T) {
	t.Setenv("WORKLOAD_IDLE_TIMEOUT", "1m")

	dir := t.TempDir()
	defer os.RemoveAll(dir)
	file := path.Join(dir, "log.file")

	dispatcher := events.NewDispatcher()
	repo, err := NewRepository(file, dispatcher)
	if err != nil {
		t.Fatalf("Error creating repository: %s", err)
	}

	reaper := NewIdleReaper(repo, nil, dispatcher, time.Minute)
	dispatcher.RegisterFunc(reaper.Handle)
	portmanager := NewPortManager(9000)
	dispatcher.RegisterFunc(portmanager.Handle)
	dispatcher.RegisterFunc(NewWorkloadInitializer(repo, portmanager).Handle)

	var collected []events.Envelope
	dispatcher.RegisterFunc(func(_ context.Context, envelope events.Envelope) error {
		collected = append(collected, envelope)
		return nil
	})

	ctx := auth.WithSession(context.Background(), auth.Session{
		Id:             "my-session",
		Username:       "malte",
		ActiveWorkload: auth.WorkloadReference{},
	})
	service := NewService(repo, nil, nil, portmanager, nil, dispatcher)
	workspace, err := service.Create(ctx)
	if err != nil {
		t.Fatalf("Error creating workspace: %s", err)
	}

	// durch WorkspaceCreated existiert ein Eintrag (At=jetzt, Workload unbekannt)
	entry, ok := reaper.entries()[workspace.Id]
	if !ok || entry.WorkloadId != uuid.Nil() {
		t.Fatalf("expected entry for workspace with unknown workload, got %+v (ok=%v)", entry, ok)
	}

	loaded, err := repo.LoadById(ctx, workspace.Id)
	if err != nil {
		t.Fatalf("load workspace: %s", err)
	}
	if len(loaded.Workloads) == 0 {
		t.Fatalf("expected workloads to be created by initializer")
	}
	for _, workload := range loaded.Workloads {
		if workload.IdleTimeout() != time.Minute {
			t.Fatalf("expected ttl 1m, got %v", workload.IdleTimeout())
		}
	}

	now := time.Now()
	reaper.now = func() time.Time { return now }
	stopped := map[uuid.UUID]int{}
	reaper.statusFn = func(workload Workload) (string, error) { return "up", nil }
	reaper.stopFn = func(_ context.Context, _ *Workspace, workload Workload) error {
		stopped[workload.Id]++
		return nil
	}

	t.Run("fresh entry is not stopped", func(t *testing.T) {
		reaper.RunOnce(ctx)
		if len(stopped) != 0 {
			t.Fatalf("expected no stop for fresh entry, got %v", stopped)
		}
	})

	t.Run("idle longer than ttl stops all workloads of the workspace", func(t *testing.T) {
		reaper.set(workspace.Id, activityEntry{At: now.Add(-2 * time.Minute)})
		reaper.RunOnce(ctx)
		for _, workload := range loaded.Workloads {
			if stopped[workload.Id] != 1 {
				t.Fatalf("expected workload %v to be stopped once, got %d", workload.Id, stopped[workload.Id])
			}
		}
		activityEvents := 0
		for _, envelope := range collected {
			if _, ok := envelope.Event().(*WorkloadActivity); ok {
				activityEvents++
			}
		}
		if activityEvents != len(loaded.Workloads) {
			t.Fatalf("expected %d activity events, got %d", len(loaded.Workloads), activityEvents)
		}
		// nach dem Stop-Event ist der Workload im Eintrag vermerkt
		if entry, ok := reaper.entries()[workspace.Id]; !ok || entry.WorkloadId == uuid.Nil() || !entry.At.Equal(now) {
			t.Fatalf("expected entry updated by stop event, got %+v (ok=%v)", entry, ok)
		}
	})

	t.Run("only the last active workload is checked", func(t *testing.T) {
		stopped = map[uuid.UUID]int{}
		reaper.set(workspace.Id, activityEntry{WorkloadId: loaded.Workloads[0].Id, At: now.Add(-2 * time.Minute)})
		reaper.RunOnce(ctx)
		if stopped[loaded.Workloads[0].Id] != 1 {
			t.Fatalf("expected last active workload to be stopped, got %v", stopped)
		}
		for _, workload := range loaded.Workloads[1:] {
			if stopped[workload.Id] != 0 {
				t.Fatalf("expected other workload not to be stopped, got %v", stopped)
			}
		}
	})

	t.Run("down workload is not stopped", func(t *testing.T) {
		stopped = map[uuid.UUID]int{}
		reaper.statusFn = func(workload Workload) (string, error) { return "down", nil }
		reaper.set(workspace.Id, activityEntry{At: now.Add(-2 * time.Minute)})
		reaper.RunOnce(ctx)
		if len(stopped) != 0 {
			t.Fatalf("expected no stop for down workload, got %v", stopped)
		}
	})

	t.Run("ttl disabled is not stopped", func(t *testing.T) {
		stopped = map[uuid.UUID]int{}
		reaper.statusFn = func(workload Workload) (string, error) { return "up", nil }
		empty := *loaded
		for i := range empty.Workloads {
			switch data := empty.Workloads[i].Data.(type) {
			case OpencodeWorkloadConfiguration:
				data.IdleTimeout = 0
				empty.Workloads[i].Data = data
			case VscodeWorkloadConfiguration:
				data.IdleTimeout = 0
				empty.Workloads[i].Data = data
			}
			reaper.check(ctx, &empty, empty.Workloads[i], now.Add(-2*time.Minute), now)
		}
		if len(stopped) != 0 {
			t.Fatalf("expected no stop for ttl<=0, got %v", stopped)
		}
	})
}

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

func TestActivityTrackerRuntimeEvents(t *testing.T) {
	tracker := NewActivityTracker()
	dispatcher := events.NewDispatcher(tracker)

	workspaceId := uuid.New()
	workloadId := uuid.New()
	at := time.Now()

	if _, ok := tracker.Last(workloadId); ok {
		t.Fatalf("expected no entry before any event")
	}

	dispatch := func(event events.DomainEvent) {
		if err := dispatcher.Dispatch(context.Background(), NewRuntimeEnvelope(workspaceId, event, at)); err != nil {
			t.Fatalf("dispatch: %s", err)
		}
	}

	dispatch(&WorkloadActivity{WorkspaceId: workspaceId, WorkloadId: workloadId, At: at})
	if last, ok := tracker.Last(workloadId); !ok || !last.Equal(at) {
		t.Fatalf("expected last=%v after activity, got %v (ok=%v)", at, last, ok)
	}

	later := at.Add(time.Minute)
	dispatch(&WorkloadActivity{WorkspaceId: workspaceId, WorkloadId: workloadId, At: later})
	if last, _ := tracker.Last(workloadId); !last.Equal(later) {
		t.Fatalf("expected last=%v after later activity, got %v", later, last)
	}

	// Replay-Events dürfen den Tracker nicht verändern
	if err := tracker.Handle(context.Background(), replayEnvelope{event: &WorkloadActivity{WorkspaceId: workspaceId, WorkloadId: workloadId, At: at}}); err != nil {
		t.Fatalf("handle replay: %s", err)
	}
	if last, ok := tracker.Last(workloadId); !ok || !last.Equal(later) {
		t.Fatalf("replay event must not change tracker, got last=%v ok=%v", last, ok)
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

	tracker := NewActivityTracker()
	dispatcher.RegisterFunc(tracker.Handle)
	view := NewWorkspacesView()
	dispatcher.RegisterFunc(view.Handle)
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

	if ids := view.Ids(); len(ids) != 1 || ids[0] != workspace.Id {
		t.Fatalf("expected view to contain workspace %v, got %v", workspace.Id, ids)
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
	stopped := map[uuid.UUID]int{}
	reaper := NewIdleReaper(repo, nil, tracker, view, dispatcher, time.Minute)
	reaper.now = func() time.Time { return now }
	reaper.statusFn = func(workload Workload) (string, error) { return "up", nil }
	reaper.stopFn = func(_ context.Context, _ *Workspace, workload Workload) error {
		stopped[workload.Id]++
		return nil
	}

	t.Run("first sight after restart is touched, not stopped", func(t *testing.T) {
		reaper.RunOnce(ctx)
		if len(stopped) != 0 {
			t.Fatalf("expected no stop on first sight, got %v", stopped)
		}
		for _, workload := range loaded.Workloads {
			if _, ok := tracker.Last(workload.Id); !ok {
				t.Fatalf("expected workload %v to be touched after first run", workload.Id)
			}
		}
	})

	t.Run("idle longer than ttl is stopped", func(t *testing.T) {
		for _, workload := range loaded.Workloads {
			tracker.TouchAt(workload.Id, now.Add(-2*time.Minute))
		}
		reaper.RunOnce(ctx)
		for _, workload := range loaded.Workloads {
			if stopped[workload.Id] != 1 {
				t.Fatalf("expected workload %v to be stopped once, got %d", workload.Id, stopped[workload.Id])
			}
			if last, ok := tracker.Last(workload.Id); !ok || !last.Equal(now) {
				t.Fatalf("expected tracker entry updated to stop time %v, got %v (ok=%v)", now, last, ok)
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
	})

	t.Run("freshly touched workload is not stopped", func(t *testing.T) {
		stopped = map[uuid.UUID]int{}
		for _, workload := range loaded.Workloads {
			tracker.TouchAt(workload.Id, now)
		}
		reaper.RunOnce(ctx)
		if len(stopped) != 0 {
			t.Fatalf("expected no stop for fresh workload, got %v", stopped)
		}
	})

	t.Run("down workload is not stopped", func(t *testing.T) {
		stopped = map[uuid.UUID]int{}
		reaper.statusFn = func(workload Workload) (string, error) { return "down", nil }
		for _, workload := range loaded.Workloads {
			tracker.TouchAt(workload.Id, now.Add(-2*time.Minute))
		}
		reaper.RunOnce(ctx)
		if len(stopped) != 0 {
			t.Fatalf("expected no stop for down workload, got %v", stopped)
		}
	})

	t.Run("ttl disabled is not stopped", func(t *testing.T) {
		stopped = map[uuid.UUID]int{}
		reaper.statusFn = func(workload Workload) (string, error) { return "up", nil }
		// simuliere Workload mit TTL 0 (z. B. Event aus der Zeit vor dem Feature)
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
			tracker.TouchAt(empty.Workloads[i].Id, now.Add(-2*time.Minute))
		}
		for _, workload := range empty.Workloads {
			reaper.check(ctx, &empty, workload, now)
		}
		if len(stopped) != 0 {
			t.Fatalf("expected no stop for ttl<=0, got %v", stopped)
		}
	})
}

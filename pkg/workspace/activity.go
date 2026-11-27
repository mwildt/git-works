package workspace

import (
	"context"
	"log/slog"
	"sync"
	"time"
	"uuid"

	"gtihub.com/mwildt/git-works/pkg/actions"
	"gtihub.com/mwildt/git-works/pkg/config"
	"gtihub.com/mwildt/git-works/pkg/events"
	"gtihub.com/mwildt/git-works/pkg/gitlab"
)

// activityEntry ist die letzte Aktion pro Workspace: welcher Workload zuletzt
// aktiv war und wann. Rein im Speicher - nicht persistent.
type activityEntry struct {
	WorkloadId uuid.UUID
	At         time.Time
}

// IdleReaper ist zugleich die Sammlung der letzten Aktion pro Workspace und der
// Hintergrund-Job, der inaktive Workloads stoppt. Handle lauscht auf dem
// Event-Dispatcher: WorkspaceCreated/WorkspaceDeleted pflegen die Menge der
// Workspaces (beim Replay wird "jetzt" eingetragen, damit nach einem Restart
// alle laufenden Workloads als aktiv gelten und die TTL erneut ablaufen),
// WorkloadActivity aktualisiert den Zeitpunkt des betroffenen Workloads.
type IdleReaper struct {
	mu         sync.Mutex
	lastAction map[uuid.UUID]activityEntry

	repo       *Repository
	actions    *actions.Client
	dispatcher *events.Dispatcher
	interval   time.Duration
	now        func() time.Time
	statusFn   func(workload Workload) (string, error)
	stopFn     func(ctx context.Context, workspace *Workspace, workload Workload) error
}

func NewIdleReaper(repo *Repository, actionsClient *actions.Client, dispatcher *events.Dispatcher, interval time.Duration) *IdleReaper {
	r := &IdleReaper{
		lastAction: make(map[uuid.UUID]activityEntry),
		repo:       repo,
		actions:    actionsClient,
		dispatcher: dispatcher,
		interval:   interval,
		now:        time.Now,
	}
	r.statusFn = func(workload Workload) (string, error) {
		return workload.Status(actionsClient)
	}
	r.stopFn = func(ctx context.Context, workspace *Workspace, workload Workload) error {
		token := gitlab.Token{Value: config.GetEnv("GITLAB_TOKEN", ""), TokenType: "PRIVATE-TOKEN"}
		return workload.Stop(token, actionsClient, workspace.Id)
	}
	return r
}

func (r *IdleReaper) Handle(_ context.Context, envelope events.Envelope) error {
	switch e := envelope.Event().(type) {
	case *WorkspaceCreated:
		r.set(e.Id, activityEntry{At: r.now()})
	case *WorkspaceDeleted:
		r.remove(e.Id)
	case *WorkloadActivity:
		if !envelope.IsReplay() {
			r.set(e.WorkspaceId, activityEntry{WorkloadId: e.WorkloadId, At: e.At})
		}
	}
	return nil
}

func (r *IdleReaper) set(workspaceId uuid.UUID, entry activityEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastAction[workspaceId] = entry
}

func (r *IdleReaper) remove(workspaceId uuid.UUID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.lastAction, workspaceId)
}

func (r *IdleReaper) entries() map[uuid.UUID]activityEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[uuid.UUID]activityEntry, len(r.lastAction))
	for id, entry := range r.lastAction {
		out[id] = entry
	}
	return out
}

func (r *IdleReaper) Start(ctx context.Context) {
	r.RunOnce(ctx)
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.RunOnce(ctx)
		}
	}
}

func (r *IdleReaper) RunOnce(ctx context.Context) {
	now := r.now()
	for workspaceId, entry := range r.entries() {
		workspace, err := r.repo.LoadById(ctx, workspaceId)
		if err != nil {
			slog.Default().Warn("idle reaper: unable to load workspace", "workspaceId", workspaceId, "error", err)
			continue
		}
		targets := workspace.Workloads
		if entry.WorkloadId != uuid.Nil() {
			if workload, ok := workspace.WorkloadById(entry.WorkloadId); ok {
				targets = []Workload{workload}
			}
		}
		for _, workload := range targets {
			r.check(ctx, workspace, workload, entry.At, now)
		}
	}
}

func (r *IdleReaper) check(ctx context.Context, workspace *Workspace, workload Workload, last, now time.Time) {
	ttl := workload.IdleTimeout()
	if ttl <= 0 {
		return
	}
	status, err := r.statusFn(workload)
	if err != nil {
		slog.Default().Warn("idle reaper: unable to determine workload status", "workspaceId", workspace.Id, "workloadId", workload.Id, "error", err)
		return
	}
	if status != "up" {
		return
	}
	idleFor := now.Sub(last)
	if idleFor <= ttl {
		return
	}
	slog.Default().Info("idle reaper: stopping idle workload", "workspaceId", workspace.Id, "workloadId", workload.Id, "idleFor", idleFor, "ttl", ttl)
	if err := r.stopFn(ctx, workspace, workload); err != nil {
		slog.Default().Error("idle reaper: unable to stop workload", "workspaceId", workspace.Id, "workloadId", workload.Id, "error", err)
		return
	}
	if r.dispatcher != nil {
		event := &WorkloadActivity{WorkspaceId: workspace.Id, WorkloadId: workload.Id, At: now}
		if err := r.dispatcher.Dispatch(ctx, NewRuntimeEnvelope(workspace.Id, event, now)); err != nil {
			slog.Default().Warn("idle reaper: unable to dispatch event", "type", event.EventType(), "error", err)
		}
	}
}

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

// ActivityTracker speichert den letzten Aktivitätszeitpunkt pro Workload -
// nur im Speicher, nie im Event-Log. Nach einem Restart ist die Map leer;
// der Reaper markiert laufende Workloads beim ersten Lauf als "jetzt aktiv".
type ActivityTracker struct {
	mu   sync.Mutex
	last map[uuid.UUID]time.Time
}

func NewActivityTracker() *ActivityTracker {
	return &ActivityTracker{last: make(map[uuid.UUID]time.Time)}
}

func (t *ActivityTracker) Handle(_ context.Context, envelope events.Envelope) error {
	if envelope.IsReplay() {
		return nil
	}
	switch e := envelope.Event().(type) {
	case *WorkloadTouched:
		t.set(e.WorkloadId, e.At)
	case *WorkloadStarted:
		t.set(e.WorkloadId, e.At)
	case *WorkloadStopped:
		t.remove(e.WorkloadId)
	case *WorkloadIdleStopped:
		t.remove(e.WorkloadId)
	}
	return nil
}

func (t *ActivityTracker) Touch(workloadId uuid.UUID) {
	t.TouchAt(workloadId, time.Now())
}

func (t *ActivityTracker) TouchAt(workloadId uuid.UUID, at time.Time) {
	t.set(workloadId, at)
}

func (t *ActivityTracker) Last(workloadId uuid.UUID) (time.Time, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	last, ok := t.last[workloadId]
	return last, ok
}

func (t *ActivityTracker) set(workloadId uuid.UUID, at time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.last[workloadId] = at
}

func (t *ActivityTracker) remove(workloadId uuid.UUID) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.last, workloadId)
}

// WorkspacesView pflegt die Menge aller Workspace-IDs (auch beim Replay) und
// dient dem IdleReaper zur Enumeration aller Workloads.
type WorkspacesView struct {
	mu  sync.Mutex
	ids map[uuid.UUID]bool
}

func NewWorkspacesView() *WorkspacesView {
	return &WorkspacesView{ids: make(map[uuid.UUID]bool)}
}

func (v *WorkspacesView) Handle(_ context.Context, envelope events.Envelope) error {
	switch e := envelope.Event().(type) {
	case *WorkspaceCreated:
		v.mu.Lock()
		v.ids[e.Id] = true
		v.mu.Unlock()
	case *WorkspaceDeleted:
		v.mu.Lock()
		delete(v.ids, e.Id)
		v.mu.Unlock()
	}
	return nil
}

func (v *WorkspacesView) Ids() []uuid.UUID {
	v.mu.Lock()
	defer v.mu.Unlock()
	ids := make([]uuid.UUID, 0, len(v.ids))
	for id := range v.ids {
		ids = append(ids, id)
	}
	return ids
}

// IdleReaper läuft regelmäßig über alle Workloads und stoppt laufende
// Workloads, deren letzte Aktivität länger als die TTL des Workloads
// (Workload.IdleTimeout) zurückliegt.
type IdleReaper struct {
	repo       *Repository
	dispatcher *events.Dispatcher
	tracker    *ActivityTracker
	view       *WorkspacesView
	interval   time.Duration
	now        func() time.Time
	statusFn   func(workload Workload) (string, error)
	stopFn     func(ctx context.Context, workspace *Workspace, workload Workload) error
}

func NewIdleReaper(repo *Repository, actionsClient *actions.Client, tracker *ActivityTracker, view *WorkspacesView, dispatcher *events.Dispatcher, interval time.Duration) *IdleReaper {
	r := &IdleReaper{
		repo:       repo,
		dispatcher: dispatcher,
		tracker:    tracker,
		view:       view,
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
	for _, workspaceId := range r.view.Ids() {
		workspace, err := r.repo.LoadById(ctx, workspaceId)
		if err != nil {
			slog.Default().Warn("idle reaper: unable to load workspace", "workspaceId", workspaceId, "error", err)
			continue
		}
		for _, workload := range workspace.Workloads {
			r.check(ctx, workspace, workload, now)
		}
	}
}

func (r *IdleReaper) check(ctx context.Context, workspace *Workspace, workload Workload, now time.Time) {
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
	last, ok := r.tracker.Last(workload.Id)
	if !ok {
		// Erstmalig gesehen (z. B. nach einem Restart): gilt als jetzt aktiv.
		r.tracker.TouchAt(workload.Id, now)
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
		event := &WorkloadIdleStopped{WorkspaceId: workspace.Id, WorkloadId: workload.Id, At: now, IdleFor: idleFor}
		if err := r.dispatcher.Dispatch(ctx, NewRuntimeEnvelope(workspace.Id, event, now)); err != nil {
			slog.Default().Warn("idle reaper: unable to dispatch event", "type", event.EventType(), "error", err)
		}
	}
}

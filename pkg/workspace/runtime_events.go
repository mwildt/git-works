package workspace

import (
	"time"
	"uuid"

	"gtihub.com/mwildt/git-works/pkg/events"
)

const (
	RuntimeEventTypeWorkloadTouched     = "workload.touched"
	RuntimeEventTypeWorkloadStarted     = "workload.started"
	RuntimeEventTypeWorkloadStopped     = "workload.stopped"
	RuntimeEventTypeWorkloadIdleStopped = "workload.idle-stopped"
)

// Runtime-Events werden nur auf dem Dispatcher gefeuert und nie an das
// Repository angehängt - sie sind also nicht persistent. Beim Replay nach
// einem Restart existieren sie nicht.

type WorkloadTouched struct {
	WorkspaceId uuid.UUID `json:"workspace_id"`
	WorkloadId  uuid.UUID `json:"workload_id"`
	At          time.Time `json:"at"`
}

func (e WorkloadTouched) EventType() string {
	return RuntimeEventTypeWorkloadTouched
}

type WorkloadStarted struct {
	WorkspaceId uuid.UUID `json:"workspace_id"`
	WorkloadId  uuid.UUID `json:"workload_id"`
	At          time.Time `json:"at"`
}

func (e WorkloadStarted) EventType() string {
	return RuntimeEventTypeWorkloadStarted
}

type WorkloadStopped struct {
	WorkspaceId uuid.UUID `json:"workspace_id"`
	WorkloadId  uuid.UUID `json:"workload_id"`
	At          time.Time `json:"at"`
}

func (e WorkloadStopped) EventType() string {
	return RuntimeEventTypeWorkloadStopped
}

type WorkloadIdleStopped struct {
	WorkspaceId uuid.UUID     `json:"workspace_id"`
	WorkloadId  uuid.UUID     `json:"workload_id"`
	At          time.Time     `json:"at"`
	IdleFor     time.Duration `json:"idle_for"`
}

func (e WorkloadIdleStopped) EventType() string {
	return RuntimeEventTypeWorkloadIdleStopped
}

// RuntimeEnvelope verpackt ein Runtime-Event für den Dispatcher. Im Gegensatz
// zur Envelope des Repositories ist sie niemals Teil des Replays.
type RuntimeEnvelope struct {
	aggregateId uuid.UUID
	event       events.DomainEvent
	at          time.Time
}

func NewRuntimeEnvelope(aggregateId uuid.UUID, event events.DomainEvent, at time.Time) RuntimeEnvelope {
	return RuntimeEnvelope{aggregateId: aggregateId, event: event, at: at}
}

func (e RuntimeEnvelope) IsReplay() bool {
	return false
}

func (e RuntimeEnvelope) AggregateId() uuid.UUID {
	return e.aggregateId
}

func (e RuntimeEnvelope) EventType() string {
	return e.event.EventType()
}

func (e RuntimeEnvelope) Event() events.DomainEvent {
	return e.event
}

func (e RuntimeEnvelope) Time() time.Time {
	return e.at
}

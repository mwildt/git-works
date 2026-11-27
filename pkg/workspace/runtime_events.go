package workspace

import (
	"time"
	"uuid"

	"gtihub.com/mwildt/git-works/pkg/events"
)

const RuntimeEventTypeWorkloadActivity = "workload.activity"

// WorkloadActivity ist das einzige Runtime-Event: Es signalisiert Aktivität
// (Proxy-Request, Start oder Stop) eines Workloads zum Zeitpunkt At. Es wird
// nur auf dem Dispatcher gefeuert und nie an das Repository angehängt - es
// ist also nicht persistent. Beim Replay nach einem Restart existiert es nicht.
type WorkloadActivity struct {
	WorkspaceId uuid.UUID `json:"workspace_id"`
	WorkloadId  uuid.UUID `json:"workload_id"`
	At          time.Time `json:"at"`
}

func (e WorkloadActivity) EventType() string {
	return RuntimeEventTypeWorkloadActivity
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

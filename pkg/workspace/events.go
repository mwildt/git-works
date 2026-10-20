package workspace

import (
	"encoding/json"
	"time"

	"uuid"
)

const (
	EventWorkspaceCreated = "workspace.created"
	EventWorkspaceDeleted = "workspace.deleted"
	EventProjectAdded     = "project.added"
	EventProjectDeleted   = "project.deleted"
	EventWorkloadAdded    = "workload.added"
	EventWorkloadDeleted  = "workload.deleted"
)

type Event struct {
	Eventtype string          `json:"eventtype"`
	Event     json.RawMessage `json:"event"`
}

type WorkspaceCreatedEvent struct {
	Workspace Workspace `json:"workspace"`
}

type WorkspaceDeletedEvent struct {
	WorkspaceId uuid.UUID `json:"workspaceId"`
}

type ProjectAddedEvent struct {
	WorkspaceId uuid.UUID `json:"workspaceId"`
	Project     Project   `json:"project"`
}

type ProjectDeletedEvent struct {
	WorkspaceId uuid.UUID `json:"workspaceId"`
	ProjectId   uuid.UUID `json:"projectId"`
}

type WorkloadAddedEvent struct {
	WorkspaceId uuid.UUID `json:"workspaceId"`
	Workload    Workload  `json:"workload"`
}

type WorkloadDeletedEvent struct {
	WorkspaceId uuid.UUID `json:"workspaceId"`
	WorkloadId  uuid.UUID `json:"workloadId"`
}

func (repo *Repository) appendEvent(eventtype string, payload any) error {
	if repo.log == nil {
		return nil
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	envelope, err := json.Marshal(Event{
		Eventtype: eventtype,
		Event:     raw,
	})
	if err != nil {
		return err
	}
	_, _, err = repo.log.appender.Append(envelope)
	return err
}

func workspaceCreatedEvent(ws Workspace) WorkspaceCreatedEvent {
	return WorkspaceCreatedEvent{Workspace: ws}
}

func withTimestamp() time.Time {
	return time.Now()
}

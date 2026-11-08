package workspace

import (
	"time"
	"uuid"
)

const (
	EventTypeCreated                 = "created"
	EventTypeDeleted                 = "deleted"
	ProjectEventTypeCreated          = "project.created"
	ProjectEventTypeDeleted          = "project.deleted"
	OpencodeWorkloadEventTypeCreated = "opencode.created"
	OpencodeWorkloadEventTypeDeleted = "opencode.deleted"
	SSHWorkloadEventTypeCreated      = "ssh.created"
)

type WorkspaceCreated struct {
	Id      uuid.UUID
	Name    string
	Owner   string
	Created time.Time
}

func (e WorkspaceCreated) EventType() string {
	return EventTypeCreated
}

type WorkspaceDeleted struct {
	Owner string
	Id    uuid.UUID
}

func (e WorkspaceDeleted) EventType() string {
	return EventTypeDeleted
}

type ProjectCreated struct {
	Id     uuid.UUID `json:"id"`
	Path   string    `json:"path,omitempty"`
	GitUrl string    `json:"git_url,omitempty"`
	Branch string    `json:"branch,omitempty"`
}

func (e ProjectCreated) EventType() string {
	return ProjectEventTypeCreated
}

type ProjectDeleted struct {
	Id uuid.UUID `json:"id"`
}

func (e ProjectDeleted) EventType() string {
	return ProjectEventTypeDeleted
}

type OpencodeWorkloadCreated struct {
	Id          uuid.UUID                     `json:"id"`
	WorkspaceId uuid.UUID                     `json:"workspace_id"`
	Type        string                        `json:"type"`
	Data        OpencodeWorkloadConfiguration `json:"data"`
}

func (e OpencodeWorkloadCreated) EventType() string {
	return OpencodeWorkloadEventTypeCreated
}

type OpencodeWorkloadDeleted struct {
	Id uuid.UUID `json:"id"`
}

func (e OpencodeWorkloadDeleted) EventType() string {
	return OpencodeWorkloadEventTypeDeleted
}

type SSHWorkloadCreated struct {
	Id          uuid.UUID `json:"id"`
	WorkspaceId uuid.UUID `json:"workspace_id"`
	Host        string    `json:"host"`
	Port        int       `json:"port"`
	Key         string    `json:"key"`
}

func (e SSHWorkloadCreated) EventType() string {
	return SSHWorkloadEventTypeCreated
}

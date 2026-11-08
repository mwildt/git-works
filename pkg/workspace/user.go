package workspace

import (
	"context"
	"time"
	"uuid"

	"gtihub.com/mwildt/git-works/pkg/events"
)

type UserWorkspace struct {
	Id      uuid.UUID `json:"id,omitempty"`
	Name    string    `json:"name,omitempty"`
	Owner   string    `json:"owner,omitempty"`
	Created time.Time `json:"created"`
}

type UserWorkspacesView struct {
	data map[string][]UserWorkspace
}

func NewUserWorkspacesView() *UserWorkspacesView {
	return &UserWorkspacesView{
		data: make(map[string][]UserWorkspace),
	}
}

func (uw *UserWorkspacesView) ByOwner(owner string) []UserWorkspace {
	user, ok := uw.data[owner]
	if !ok {
		return []UserWorkspace{}
	}
	return user
}

func (uw *UserWorkspacesView) Handle(_ context.Context, envelope events.Envelope) error {
	switch e := envelope.Event().(type) {
	case *WorkspaceCreated:
		user, ok := uw.data[e.Owner]
		if !ok {
			user = make([]UserWorkspace, 0)
		}
		user = append(user, UserWorkspace{
			Id:      e.Id,
			Name:    e.Name,
			Created: e.Created,
			Owner:   e.Owner,
		})
		uw.data[e.Owner] = user

	case *WorkspaceDeleted:
		user, ok := uw.data[e.Owner]
		if !ok {
			return nil
		}
		newUser := make([]UserWorkspace, 0)
		for _, item := range user {
			if item.Id != e.Id {
				newUser = append(newUser, item)
			}
		}
		uw.data[e.Owner] = newUser
	}
	return nil
}

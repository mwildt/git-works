package workspace

import (
	"context"
	"os"
	"path"
	"testing"
	"uuid"

	"gtihub.com/mwildt/git-works/pkg/auth"
	"gtihub.com/mwildt/git-works/pkg/events"
)

type EventCollector []events.Envelope

func (ec *EventCollector) Handle(_ context.Context, envelope events.Envelope) error {
	*ec = append(*ec, envelope)
	return nil
}

func TestServiceRepo(t *testing.T) {
	dir := t.TempDir()
	file := path.Join(dir, "log.file")
	os.Setenv("WORKSPACE_LOG", file)

	portmanager := NewPortManager(9000)

	repo, err := NewRepository(events.FilterName("workload.created", portmanager))
	if err != nil {
		t.Fatalf("Error creating repository: %s", err)
	}

	service := NewService(repo, nil, nil, portmanager)

	ctx := auth.WithSession(context.Background(), auth.Session{
		Id:             "my-session",
		Username:       "malte",
		ActiveWorkload: auth.WorkloadReference{},
	})

	t.Run("create", func(t *testing.T) {
		res, err := service.Create(ctx)
		if err != nil {
			t.Fatalf("Error creating service: %s", err)
		}
		if res.Id == uuid.Nil() {
			t.Fatalf("Error creating service: id should not be nil")
		}
		if res.Workloads[0].Data.Port != 9000 {
			t.Fatalf("Error creating service: port should be 9000")
		}

		if portmanager.items[0].WorkspaceId == res.Id {
			t.Error("Workspace id is not progaated into manager")
		}

		res2, err := service.Create(ctx)
		if err != nil {
			t.Fatalf("Error creating service: %s", err)
		}
		if res2.Workloads[0].Data.Port != 9001 {
			t.Fatalf("Error creating service: port should be 9001")
		}

	})

}

func TestAggregateRepo(t *testing.T) {

	ctx := auth.WithSession(context.Background(), auth.Session{
		Id:             "my-session",
		Username:       "malte",
		ActiveWorkload: auth.WorkloadReference{},
	})

	t.Run("load on empty", func(t *testing.T) {
		dir := t.TempDir()
		file := path.Join(dir, "log.file")
		os.Setenv("WORKSPACE_LOG", file)
		repo, err := NewRepository()
		if err != nil {
			t.Fatalf("Error creating repository: %s", err)
		}
		_, err = repo.LoadById(context.Background(), uuid.New())
		if err != NotFound {
			t.Fatalf("Error loading repository: %s", err)
		}

	})

	t.Run("save", func(t *testing.T) {

		createEvents := make(EventCollector, 0)

		dir := t.TempDir()
		file := path.Join(dir, "log.file")

		os.Setenv("WORKSPACE_LOG", file)

		repo, err := NewRepository(events.FilterName("created", &createEvents))
		if err != nil {
			t.Fatalf("Error creating repository: %s", err)
		}

		ws := NewWorkspace("malte")

		err = repo.Save(ctx, ws)
		if err != nil {
			t.Fatalf("Error saving workspace: %s", err)
		}

		loaded, err := repo.LoadById(context.Background(), ws.Id)
		if err != nil {
			t.Fatalf("Error loading workspace: %s", err)
		}

		if loaded.Owner != "malte" {
			t.Error("Expected malte workspace owner, but was", loaded.Owner)
		}

		if len(createEvents) != 1 {
			t.Error("Expected 1 event, found ", len(createEvents))
		} else if createEvents[0].EventType() != "created" {
			t.Error("Expected created event, found ", createEvents[0].EventType())
		} else if created, ok := createEvents[0].Event().(*WorkspaceCreated); !ok {
			t.Error("Expected created event, found ", created)
		}
	})

	t.Run("deleted events are not found", func(t *testing.T) {
		dir := t.TempDir()
		file := path.Join(dir, "log.file")
		os.Setenv("WORKSPACE_LOG", file)
		repo, err := NewRepository()
		if err != nil {
			t.Fatalf("Error creating repository: %s", err)
		}
		ws := NewWorkspace("malte")
		err = repo.Save(ctx, ws)
		if err != nil {
			t.Fatalf("Error saving workspace: %s", err)
		}

		if err = repo.Delete(ctx, ws.Id); err != nil {
			t.Error("unable to delete")
		}

		_, err = repo.LoadById(ctx, ws.Id)
		if err != NotFound {
			t.Fatalf("Error loading workspace: %s", err)
		}
	})

}

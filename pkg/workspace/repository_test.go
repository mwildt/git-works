package workspace

import (
	"context"
	"path/filepath"
	"testing"

	"gtihub.com/mwildt/git-works/pkg/auth"
	"gtihub.com/mwildt/git-works/pkg/storage"
	"uuid"
)

func ownerContext(username string) context.Context {
	return auth.WithSession(context.Background(), auth.Session{Username: username})
}

func newTestRepository(t *testing.T, logFile string) *Repository {
	t.Setenv("WORSPACE_LOG", logFile)
	repo, err := NewRepository()
	if err != nil {
		t.Fatalf("NewRepository: %v", err)
	}
	return repo
}

func TestEventlogRoundtrip(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "events.log")

	repo := newTestRepository(t, logFile)
	ctx := ownerContext("tester")

	ws := NewWorkspace("tester")
	if err := repo.CreateWorkspace(ctx, ws); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

	project := Project{Id: uuid.NewV4(), Path: "repo-a", GitUrl: "https://example.com/repo-a.git", Branch: "main"}
	if err := repo.AddProject(ctx, ws.Id, project); err != nil {
		t.Fatalf("AddProject: %v", err)
	}

	workload := Workload{Id: uuid.NewV4(), WorkspaceId: ws.Id, Type: "OpenCode"}
	if err := repo.AddWorkload(ctx, ws.Id, workload); err != nil {
		t.Fatalf("AddWorkload: %v", err)
	}

	if err := repo.DeleteProject(ctx, ws.Id, project.Id); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}

	replayed := newTestRepository(t, logFile)
	rws, err := replayed.ById(ctx, ws.Id)
	if err != nil {
		t.Fatalf("ById after replay: %v", err)
	}
	if len(rws.Projects) != 0 {
		t.Errorf("expected no projects after replay, got %d", len(rws.Projects))
	}
	if len(rws.Workloads) != 1 || rws.Workloads[0].Id != workload.Id {
		t.Errorf("workload not restored, got %+v", rws.Workloads)
	}
	if rws.Owner != "tester" || rws.Name != ws.Name {
		t.Errorf("workspace meta mismatch: %+v", rws)
	}

	if err := repo.Delete(ctx, ws.Id); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	replayed2 := newTestRepository(t, logFile)
	if _, err := replayed2.ById(ctx, ws.Id); err != NotFound {
		t.Errorf("expected NotFound after delete replay, got %v", err)
	}
}

func TestEventlogLegacyFullEvent(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "legacy.log")

	ws := NewWorkspace("tester")
	project := Project{Id: uuid.NewV4(), Path: "repo-a", GitUrl: "https://example.com/repo-a.git", Branch: "main"}
	ws.Projects = append(ws.Projects, project)

	writer, err := storage.NewFileAppender(logFile)
	if err != nil {
		t.Fatalf("appender: %v", err)
	}
	legacy := Repository{items: map[string]*Workspace{}, log: &eventlog{file: logFile, appender: writer}}
	if err := legacy.appendEvent("workspace.full", ws); err != nil {
		t.Fatalf("append legacy event: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	replayed := newTestRepository(t, logFile)
	ctx := ownerContext("tester")
	rws, err := replayed.ById(ctx, ws.Id)
	if err != nil {
		t.Fatalf("ById after legacy replay: %v", err)
	}
	if len(rws.Projects) != 1 || rws.Projects[0].Path != "repo-a" {
		t.Errorf("legacy full event not restored: %+v", rws.Projects)
	}
}

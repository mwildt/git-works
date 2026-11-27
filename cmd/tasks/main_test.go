package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func openTempStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenStore(filepath.Join(t.TempDir(), "tasks.json"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	return s
}

func TestAddAssignsSequentialIDs(t *testing.T) {
	s := openTempStore(t)
	t1, err := s.Add("first", "", "")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	t2, err := s.Add("second", "desc", priorityHigh)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if t1.ID != "T-1" || t2.ID != "T-2" {
		t.Fatalf("unexpected IDs: %s, %s", t1.ID, t2.ID)
	}
	if t1.Priority != priorityNormal || t1.Status != statusTodo {
		t.Fatalf("unexpected defaults: %+v", t1)
	}
	if t2.Priority != priorityHigh {
		t.Fatalf("priority not applied: %+v", t2)
	}
}

func TestAddRejectsInvalidInput(t *testing.T) {
	s := openTempStore(t)
	if _, err := s.Add("", "", ""); err == nil {
		t.Fatal("empty title accepted")
	}
	if _, err := s.Add("x", "", "urgent"); err == nil {
		t.Fatal("invalid priority accepted")
	}
}

func TestNextPicksHighestPriorityThenOldest(t *testing.T) {
	s := openTempStore(t)
	if _, err := s.Add("low", "", priorityLow); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Add("high-old", "", priorityHigh); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Add("high-new", "", priorityHigh); err != nil {
		t.Fatal(err)
	}
	n1, err := s.Next()
	if err != nil {
		t.Fatal(err)
	}
	if n1 == nil || n1.ID != "T-2" || n1.Status != statusInProgress {
		t.Fatalf("unexpected next: %+v", n1)
	}
	n2, err := s.Next()
	if err != nil {
		t.Fatal(err)
	}
	if n2 == nil || n2.ID != "T-3" {
		t.Fatalf("unexpected second next: %+v", n2)
	}
	n3, err := s.Next()
	if err != nil {
		t.Fatal(err)
	}
	if n3 == nil || n3.ID != "T-1" {
		t.Fatalf("unexpected third next: %+v", n3)
	}
	n4, err := s.Next()
	if err != nil {
		t.Fatal(err)
	}
	if n4 != nil {
		t.Fatalf("expected no open task, got %+v", n4)
	}
}

func TestCompleteAndListFilter(t *testing.T) {
	s := openTempStore(t)
	t1, _ := s.Add("a", "", "")
	if _, err := s.Add("b", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Complete(t1.ID, "done note"); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	done, err := s.List(statusDone)
	if err != nil {
		t.Fatal(err)
	}
	if len(done) != 1 || done[0].Note != "done note" || done[0].CompletedAt == nil {
		t.Fatalf("unexpected done list: %+v", done)
	}
	open, err := s.List(statusTodo)
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 1 || open[0].ID != "T-2" {
		t.Fatalf("unexpected open list: %+v", open)
	}
	if _, err := s.List("bogus"); err == nil {
		t.Fatal("invalid status accepted")
	}
}

func TestUpdateAndRemove(t *testing.T) {
	s := openTempStore(t)
	t1, _ := s.Add("old", "", "")
	title := "new"
	status := statusInProgress
	upd, err := s.Update(t1.ID, &title, nil, nil, &status)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if upd.Title != "new" || upd.Status != statusInProgress {
		t.Fatalf("update not applied: %+v", upd)
	}
	if _, err := s.Update("T-99", &title, nil, nil, nil); err == nil {
		t.Fatal("update of missing task succeeded")
	}
	bad := "bogus"
	if _, err := s.Update(t1.ID, nil, nil, &bad, nil); err == nil {
		t.Fatal("invalid priority accepted")
	}
	if err := s.Remove(t1.ID); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if err := s.Remove(t1.ID); err == nil {
		t.Fatal("double remove succeeded")
	}
	all, _ := s.List("")
	if len(all) != 0 {
		t.Fatalf("expected empty list, got %+v", all)
	}
}

func TestPersistenceAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	t1, _ := s.Add("persist me", "", priorityHigh)
	if _, err := s.Complete(t1.ID, ""); err != nil {
		t.Fatal(err)
	}

	s2, err := OpenStore(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	tasks, err := s2.List("all")
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].ID != t1.ID || tasks[0].Status != statusDone {
		t.Fatalf("state lost after reopen: %+v", tasks)
	}
	t2, err := s2.Add("after reopen", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if t2.ID != "T-2" {
		t.Fatalf("ID counter not persisted, got %s", t2.ID)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var data fileData
	if err := json.Unmarshal(b, &data); err != nil {
		t.Fatalf("store file is not valid JSON: %v", err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("temp file left behind")
	}
}

func TestHTTPTransportEndToEnd(t *testing.T) {
	store := openTempStore(t)
	server := newServer(store)
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	ctx := context.Background()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: ts.URL}, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer session.Close()

	call := func(name string, args map[string]any) map[string]any {
		t.Helper()
		res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if res.IsError {
			t.Fatalf("%s: unexpected tool error: %v", name, res.Content)
		}
		sc, ok := res.StructuredContent.(map[string]any)
		if !ok {
			t.Fatalf("%s: no structured content in %+v", name, res)
		}
		return sc
	}

	add := call("task_add", map[string]any{"title": "via http", "priority": "high"})
	task := add["task"].(map[string]any)
	if task["id"] != "T-1" || task["status"] != statusTodo {
		t.Fatalf("unexpected add result: %+v", task)
	}

	next := call("task_next", map[string]any{})
	nt := next["task"].(map[string]any)
	if nt["id"] != "T-1" || nt["status"] != statusInProgress {
		t.Fatalf("task_next did not claim T-1: %+v", nt)
	}

	call("task_complete", map[string]any{"id": "T-1", "note": "http ok"})

	list := call("task_list", map[string]any{"status": "done"})
	if list["count"].(float64) != 1 {
		t.Fatalf("expected 1 done task, got %+v", list)
	}

	tasks, err := store.List("all")
	if err != nil || len(tasks) != 1 || tasks[0].Note != "http ok" {
		t.Fatalf("store not updated via HTTP: %+v, err=%v", tasks, err)
	}
}

func TestBearerAuth(t *testing.T) {
	h := bearerAuth("secret", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("missing token: got %d, want 401", rec.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong token: got %d, want 401", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("correct token: got %d, want 200", rec.Code)
	}
}

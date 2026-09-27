package localserver

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCustomBoardExpiresSevenDaysAfterCreation(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	hub := newDemoHub(dir)
	hub.now = func() time.Time { return now }
	handler := newDemoHandlerWithHub(hub)
	created := demoRequest(t, handler, http.MethodPost, "/api/boards", map[string]any{"name": "Temporary gallery"})
	if created.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", created.Code, created.Body.String())
	}
	var board boardSummary
	if err := json.Unmarshal(created.Body.Bytes(), &board); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, board.ID+".json")
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var stored map[string]any
	if err := json.Unmarshal(data, &stored); err != nil {
		t.Fatal(err)
	}
	if stored["createdAt"] != now.Format(time.RFC3339) {
		t.Fatalf("createdAt = %v", stored["createdAt"])
	}
	now = now.Add(7*24*time.Hour - time.Second)
	if response := demoRequest(t, handler, http.MethodPost, "/api/events?board="+board.ID, map[string]any{"pixelId": 0, "color": 2}); response.Code != http.StatusOK {
		t.Fatalf("paint before expiry = %d", response.Code)
	}
	now = now.Add(time.Second)
	for _, path := range []string{"/board/" + board.ID, "/api/map?board=" + board.ID} {
		if response := demoRequest(t, handler, http.MethodGet, path, nil); response.Code != http.StatusNotFound {
			t.Errorf("expired %s = %d, want 404", path, response.Code)
		}
	}
	if response := demoRequest(t, handler, http.MethodPost, "/api/events?board="+board.ID, map[string]any{"pixelId": 1, "color": 3}); response.Code != http.StatusNotFound {
		t.Errorf("expired paint = %d, want 404", response.Code)
	}
	var listed struct {
		Boards []boardSummary `json:"boards"`
	}
	if err := json.Unmarshal(demoRequest(t, handler, http.MethodGet, "/api/boards", nil).Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	for _, entry := range listed.Boards {
		if entry.ID == board.ID {
			t.Fatal("expired board remains listed")
		}
	}
}

func TestLegacyCustomBoardGetsPersistedSevenDayGrace(t *testing.T) {
	dir := t.TempDir()
	id := "0123456789abcdef01234567"
	file := filepath.Join(dir, id+".json")
	data, err := json.Marshal(storedBoard{ID: id, Name: "Older board", Width: demoWidth, Height: demoHeight, Cells: make([]int8, demoCells)})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, data, 0644); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	hub := newDemoHub(dir)
	hub.now = func() time.Time { return now }
	if err := hub.loadBoards(); err != nil {
		t.Fatal(err)
	}
	var migrated map[string]any
	data, err = os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &migrated); err != nil {
		t.Fatal(err)
	}
	if migrated["createdAt"] != now.Format(time.RFC3339) {
		t.Fatalf("migrated createdAt = %v", migrated["createdAt"])
	}
	now = now.Add(6 * 24 * time.Hour)
	restarted := newDemoHub(dir)
	restarted.now = func() time.Time { return now }
	if err := restarted.loadBoards(); err != nil {
		t.Fatal(err)
	}
	handler := newDemoHandlerWithHub(restarted)
	if response := demoRequest(t, handler, http.MethodGet, "/api/map?board="+id, nil); response.Code != http.StatusOK {
		t.Fatalf("migrated board before expiry = %d", response.Code)
	}
	now = now.Add(24 * time.Hour)
	if response := demoRequest(t, handler, http.MethodGet, "/api/map?board="+id, nil); response.Code != http.StatusNotFound {
		t.Fatalf("migrated board after expiry = %d, want 404", response.Code)
	}
}

func TestCustomCleanupDeletesOnlyExpiredCustomBoards(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	hub := newDemoHub(dir)
	hub.now = func() time.Time { return now }
	handler := newDemoHandlerWithHub(hub)
	created := demoRequest(t, handler, http.MethodPost, "/api/boards", map[string]any{"name": "Expiring"})
	var custom boardSummary
	if err := json.Unmarshal(created.Body.Bytes(), &custom); err != nil {
		t.Fatal(err)
	}
	quilt := filepath.Join(dir, "quilt.json")
	if err := os.WriteFile(quilt, []byte(`{"preserved":true}`), 0644); err != nil {
		t.Fatal(err)
	}
	commons := filepath.Join(dir, "commons.json")
	if err := os.WriteFile(commons, []byte(`{"preserved":true}`), 0644); err != nil {
		t.Fatal(err)
	}
	now = now.Add(7 * 24 * time.Hour)
	if err := hub.pruneCustom(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, custom.ID+".json")); !os.IsNotExist(err) {
		t.Fatalf("expired file still exists: %v", err)
	}
	for _, file := range []string{quilt, commons} {
		if _, err := os.Stat(file); err != nil {
			t.Fatalf("noncustom file %s: %v", file, err)
		}
	}
}

func TestCustomCleanupRetriesFailedDeletionAndClosesStreams(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	hub := newDemoHub(dir)
	hub.now = func() time.Time { return now }
	id := "0123456789abcdef01234567"
	stream := make(chan []byte, 1)
	board := &demoState{id: id, name: "Blocked removal", createdAt: now.Add(-7 * 24 * time.Hour), now: hub.now, dir: dir, subscribers: map[chan []byte]struct{}{stream: {}}}
	hub.boards[id] = board
	blockedPath := filepath.Join(dir, id+".json")
	if err := os.Mkdir(blockedPath, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(blockedPath, "child"), []byte("block"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := hub.pruneCustom(); err == nil {
		t.Fatal("expected failed deletion")
	}
	if _, open := <-stream; open {
		t.Fatal("expired stream remained open")
	}
	if hub.lookup(id) != nil {
		t.Fatal("expired board accessible after failed deletion")
	}
	if err := os.Remove(filepath.Join(blockedPath, "child")); err != nil {
		t.Fatal(err)
	}
	if err := hub.pruneCustom(); err != nil {
		t.Fatal(err)
	}
	if _, exists := hub.boards[id]; exists {
		t.Fatal("board remained after retry")
	}
}

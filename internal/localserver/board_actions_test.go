package localserver

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestClearAndArtworkReplaceAtomically(t *testing.T) {
	dir := t.TempDir()
	hub := newDemoHub(dir)
	handler := newDemoHandlerWithHub(hub)
	shape := make([]int8, demoCells)
	shape[0] = -1
	shape[1] = 4
	board := hub.lookup("commons")
	board.mu.Lock()
	board.cells = shape
	board.target = make([]int8, demoCells)
	board.mu.Unlock()
	clear := demoRequest(t, handler, http.MethodPost, "/api/clear?board=commons", map[string]any{"generation": 0})
	if clear.Code != http.StatusOK {
		t.Fatalf("clear = %d: %s", clear.Code, clear.Body.String())
	}
	var after struct {
		Cells      []int8 `json:"cells"`
		Generation uint64 `json:"generation"`
	}
	if err := json.Unmarshal(demoRequest(t, handler, http.MethodGet, "/api/map?board=commons", nil).Body.Bytes(), &after); err != nil {
		t.Fatal(err)
	}
	if after.Cells[0] != -1 || after.Cells[1] != 0 || after.Generation != 1 {
		t.Fatalf("clear state: blocked=%d painted=%d generation=%d", after.Cells[0], after.Cells[1], after.Generation)
	}
	if response := demoRequest(t, handler, http.MethodPost, "/api/clear?board=commons", map[string]any{"generation": 0}); response.Code != http.StatusConflict {
		t.Fatalf("stale clear = %d", response.Code)
	}
	next := make([]int8, demoCells)
	next[0] = -1
	next[1] = 9
	if response := demoRequest(t, handler, http.MethodPut, "/api/artwork?board=commons", map[string]any{"generation": 1, "cells": next}); response.Code != http.StatusOK {
		t.Fatalf("artwork = %d: %s", response.Code, response.Body.String())
	}
	if response := demoRequest(t, handler, http.MethodPut, "/api/artwork?board=commons", map[string]any{"generation": 1, "cells": next}); response.Code != http.StatusConflict {
		t.Fatalf("stale artwork = %d", response.Code)
	}
	loaded := newDemoHub(dir)
	if err := loaded.loadBoards(); err != nil {
		t.Fatal(err)
	}
	if got := loaded.lookup("commons"); got == nil || got.generation != 2 || got.cells[1] != 9 || len(got.target) != 0 {
		t.Fatal("replacement not persisted")
	}
	bad := append([]int8(nil), next...)
	bad[0] = 1
	if response := demoRequest(t, handler, http.MethodPut, "/api/artwork?board=commons", map[string]any{"generation": 2, "cells": bad}); response.Code != http.StatusBadRequest {
		t.Fatalf("blocked mutation = %d", response.Code)
	}
}

func TestArchivedDailyRejectsBoardActions(t *testing.T) {
	dir := t.TempDir()
	hub := newDemoHub(dir)
	hub.now = func() time.Time { return time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC) }
	handler := newDemoHandlerWithHub(hub)
	yesterday := "daily-2026-09-25"
	hub.boards[yesterday] = &demoState{id: yesterday, name: "Daily Mosaic", date: "2026-09-25", prompt: "Old", now: hub.now, cells: make([]int8, demoCells), width: demoWidth, height: demoHeight, subscribers: make(map[chan []byte]struct{})}
	for _, route := range []struct {
		method, path string
		body         map[string]any
	}{
		{http.MethodPost, "/api/clear?board=" + yesterday, map[string]any{"generation": 0}},
		{http.MethodPut, "/api/artwork?board=" + yesterday, map[string]any{"generation": 0, "cells": make([]int8, demoCells)}},
	} {
		if response := demoRequest(t, handler, route.method, route.path, route.body); response.Code != http.StatusForbidden {
			t.Errorf("%s = %d", route.path, response.Code)
		}
	}
}

func TestClearBroadcastsOneFullSnapshot(t *testing.T) {
	hub := newDemoHub(t.TempDir())
	handler := newDemoHandlerWithHub(hub)
	board := hub.lookup("commons")
	channel := make(chan []byte, 2)
	board.mu.Lock()
	board.subscribers[channel] = struct{}{}
	board.mu.Unlock()
	if response := demoRequest(t, handler, http.MethodPost, "/api/clear?board=commons", map[string]any{"generation": 0}); response.Code != http.StatusOK {
		t.Fatalf("clear = %d", response.Code)
	}
	select {
	case snapshot := <-channel:
		var update struct {
			Cells      []int8 `json:"cells"`
			Generation uint64 `json:"generation"`
		}
		if err := json.Unmarshal(snapshot, &update); err != nil {
			t.Fatal(err)
		}
		if len(update.Cells) != demoCells || update.Generation != 1 {
			t.Fatal("incomplete clear snapshot")
		}
	default:
		t.Fatal("no clear snapshot")
	}
	select {
	case <-channel:
		t.Fatal("clear broadcast more than once")
	default:
	}
}

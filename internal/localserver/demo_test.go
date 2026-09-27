package localserver

import (
	"bytes"
	"encoding/json"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func demoRequest(t *testing.T, handler http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var input bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&input).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	request := httptest.NewRequest(method, path, &input)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestLiveBatchCarriesRevealMarker(t *testing.T) {
	handler := newDemoHandlerWithHub(newDemoHub(t.TempDir()))
	response := demoRequest(t, handler, http.MethodPost, "/api/events/batch?board=commons", map[string]any{
		"generation": 0, "live": true, "events": []map[string]int{{"pixelId": 1, "color": 3}},
	})
	if response.Code != http.StatusOK {
		t.Fatalf("live batch = %d: %s", response.Code, response.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["live"] != true {
		t.Fatalf("live marker missing: %v", body)
	}
}

func TestThemedToolPageAssets(t *testing.T) {
	handler := newDemoHandler()
	module := demoRequest(t, handler, http.MethodGet, "/board-tools.mjs", nil)
	if module.Code != http.StatusOK || module.Header().Get("Content-Type") != "text/javascript; charset=utf-8" {
		t.Fatalf("tool module response = %d %q", module.Code, module.Header().Get("Content-Type"))
	}
	page := demoRequest(t, handler, http.MethodGet, "/board/garden", nil)
	if page.Code != http.StatusOK {
		t.Fatalf("garden page = %d", page.Code)
	}
	for _, hook := range [][]byte{[]byte(`id="special-tool-panel"`), []byte(`id="tool-choices"`), []byte(`id="special-instruction"`), []byte(`id="night-marker"`), []byte(`type="module" src="/app.js"`)} {
		if !bytes.Contains(page.Body.Bytes(), hook) {
			t.Errorf("missing accessible tool hook %s", hook)
		}
	}
}

func TestArchPageAndDiagramAssets(t *testing.T) {
	handler := newDemoHandler()
	arch := demoRequest(t, handler, http.MethodGet, "/arch", nil)
	if arch.Code != http.StatusOK || arch.Header().Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatalf("arch page = %d %q", arch.Code, arch.Header().Get("Content-Type"))
	}
	if !bytes.Contains(arch.Body.Bytes(), []byte("System Architecture")) {
		t.Fatalf("arch page missing expected heading")
	}

	diagram := demoRequest(t, handler, http.MethodGet, "/assets/diagrams/canvas-architecture.html", nil)
	if diagram.Code != http.StatusOK || diagram.Header().Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatalf("diagram asset = %d %q", diagram.Code, diagram.Header().Get("Content-Type"))
	}
}

func TestLocalLiveConnectionModuleIsServed(t *testing.T) {
	handler := newDemoHandler()
	module := demoRequest(t, handler, http.MethodGet, "/live-connection.mjs", nil)
	if module.Code != http.StatusOK || !bytes.Contains(module.Body.Bytes(), []byte("connectLive")) {
		t.Fatalf("live module response = %d %q", module.Code, module.Body.String())
	}
	for _, path := range []string{"/live-fill.mjs", "/pixel-reveal.mjs"} {
		module := demoRequest(t, handler, http.MethodGet, path, nil)
		if module.Code != http.StatusOK || module.Header().Get("Content-Type") != "text/javascript; charset=utf-8" {
			t.Fatalf("%s response = %d %q", path, module.Code, module.Header().Get("Content-Type"))
		}
	}
	page := demoRequest(t, handler, http.MethodGet, "/map", nil)
	if !bytes.Contains(page.Body.Bytes(), []byte(`data-live-transport="sse"`)) {
		t.Fatal("local page must select SSE transport")
	}
}

func TestCommunityQuiltIsRetiredWithoutDeletingSavedArtwork(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "quilt.json")
	data, err := json.Marshal(storedBoard{ID: "quilt", Name: "Community Quilt", Width: demoWidth, Height: demoHeight, Cells: make([]int8, demoCells)})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, data, 0644); err != nil {
		t.Fatal(err)
	}
	handler, err := newDemoHandlerWithStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	var listed struct {
		Boards []boardSummary `json:"boards"`
	}
	if err := json.Unmarshal(demoRequest(t, handler, http.MethodGet, "/api/boards", nil).Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	for _, board := range listed.Boards {
		if board.ID == "quilt" {
			t.Fatal("retired Quilt remains in the directory")
		}
	}
	for _, path := range []string{"/board/quilt", "/api/map?board=quilt"} {
		if response := demoRequest(t, handler, http.MethodGet, path, nil); response.Code != http.StatusNotFound {
			t.Errorf("%s = %d, want 404", path, response.Code)
		}
	}
	if got, err := os.ReadFile(file); err != nil || !bytes.Equal(got, data) {
		t.Fatalf("retired Quilt data changed: %v", err)
	}
}

func TestPhotoOnlyOnUserCreatedBoard(t *testing.T) {
	handler := newDemoHandler()
	for _, path := range []string{
		"/api/photo",
		"/api/photo?board=commons",
		"/api/photo?board=kaleidoscope",
		"/api/photo?board=garden",
		"/api/photo?board=night-sky",
		"/api/photo?board=tiny-town",
		"/api/photo?board=daily",
	} {
		response := demoRequest(t, handler, http.MethodPut, path, nil)
		if response.Code != http.StatusForbidden {
			t.Errorf("photo replacement %s = %d, want 403", path, response.Code)
		}
	}
	created := demoRequest(t, handler, http.MethodPost, "/api/boards", map[string]any{"name": "My own space"})
	if created.Code != http.StatusCreated {
		t.Fatalf("create board = %d", created.Code)
	}
	var board boardSummary
	if err := json.Unmarshal(created.Body.Bytes(), &board); err != nil {
		t.Fatal(err)
	}
	photo := demoRequest(t, handler, http.MethodPut, "/api/photo?board="+board.ID, map[string]any{"width": 1, "height": 1, "colors": []int{2}})
	if photo.Code != http.StatusOK {
		t.Fatalf("own board photo = %d: %s", photo.Code, photo.Body.String())
	}
}

func TestDemoMapAndEvents(t *testing.T) {
	handler := newDemoHandler()
	mapResponse := demoRequest(t, handler, http.MethodGet, "/api/map", nil)
	if mapResponse.Code != http.StatusOK {
		t.Fatalf("initial map status = %d", mapResponse.Code)
	}
	var initial struct {
		Width  int    `json:"width"`
		Height int    `json:"height"`
		Cells  []int8 `json:"cells"`
	}
	if err := json.Unmarshal(mapResponse.Body.Bytes(), &initial); err != nil {
		t.Fatal(err)
	}
	if initial.Width != 200 || initial.Height != 200 || len(initial.Cells) != 40000 {
		t.Fatalf("unexpected initial map dimensions: %d x %d, %d cells", initial.Width, initial.Height, len(initial.Cells))
	}

	if response := demoRequest(t, handler, http.MethodPut, "/api/map", map[string]any{"cells": []int{-1, 0}}); response.Code != http.StatusBadRequest {
		t.Fatalf("short map status = %d", response.Code)
	}
	cells := make([]int8, 40000)
	cells[1] = -1
	if response := demoRequest(t, handler, http.MethodPut, "/api/map", map[string]any{"cells": cells}); response.Code != http.StatusOK {
		t.Fatalf("valid map status = %d: %s", response.Code, response.Body.String())
	}
	if response := demoRequest(t, handler, http.MethodPost, "/api/events", map[string]any{"pixelId": 1, "color": 3}); response.Code != http.StatusBadRequest {
		t.Fatalf("blocked pixel status = %d", response.Code)
	}
	if response := demoRequest(t, handler, http.MethodPost, "/api/events", map[string]any{"pixelId": 0, "color": 17}); response.Code != http.StatusBadRequest {
		t.Fatalf("invalid color status = %d", response.Code)
	}
	if response := demoRequest(t, handler, http.MethodPost, "/api/events", map[string]any{"pixelId": 40000, "color": 3}); response.Code != http.StatusBadRequest {
		t.Fatalf("out of range pixel status = %d", response.Code)
	}
	if response := demoRequest(t, handler, http.MethodPost, "/api/events", map[string]any{"color": 3}); response.Code != http.StatusBadRequest {
		t.Fatalf("missing pixel status = %d", response.Code)
	}
	if response := demoRequest(t, handler, http.MethodPost, "/api/events", map[string]any{"pixelId": 0, "color": 3}); response.Code != http.StatusOK {
		t.Fatalf("valid event status = %d: %s", response.Code, response.Body.String())
	}
	updated := demoRequest(t, handler, http.MethodGet, "/api/map", nil)
	var result struct {
		Cells []int8 `json:"cells"`
	}
	if err := json.Unmarshal(updated.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Cells[0] != 3 || result.Cells[1] != -1 {
		t.Fatalf("unexpected cells after event: %d, %d", result.Cells[0], result.Cells[1])
	}
	blockedBatch := map[string]any{"events": []map[string]int{{"pixelId": 0, "color": 2}, {"pixelId": 1, "color": 4}}}
	if response := demoRequest(t, handler, http.MethodPost, "/api/events/batch", blockedBatch); response.Code != http.StatusBadRequest {
		t.Fatalf("blocked batch status = %d", response.Code)
	}
	validBatch := map[string]any{"events": []map[string]int{{"pixelId": 0, "color": 2}, {"pixelId": 2, "color": 4}}}
	if response := demoRequest(t, handler, http.MethodPost, "/api/events/batch", validBatch); response.Code != http.StatusOK {
		t.Fatalf("valid batch status = %d: %s", response.Code, response.Body.String())
	}
	updated = demoRequest(t, handler, http.MethodGet, "/api/map", nil)
	if err := json.Unmarshal(updated.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Cells[0] != 2 || result.Cells[1] != -1 || result.Cells[2] != 4 {
		t.Fatalf("unexpected cells after batch: %d, %d, %d", result.Cells[0], result.Cells[1], result.Cells[2])
	}
}

func TestKaleidoscopeSampleIsASeparateBoard(t *testing.T) {
	handler := newDemoHandler()
	response := demoRequest(t, handler, http.MethodGet, "/api/map?board=kaleidoscope", nil)
	var result struct {
		Cells []int8 `json:"cells"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Cells) != 40000 || result.Cells[0] != -1 || result.Cells[100*200+100] != 0 {
		t.Fatal("kaleidoscope board should have transparent corners and a colorable center")
	}
	count := 0
	for _, cell := range result.Cells {
		if cell == 0 {
			count++
		}
	}
	if count < 2000 || count > 15000 {
		t.Fatalf("expected a distinct pattern within the frame, got %d colorable pixels", count)
	}

	imageResponse := demoRequest(t, handler, http.MethodGet, "/sample.png", nil)
	if imageResponse.Code != http.StatusOK {
		t.Fatalf("sample image status = %d", imageResponse.Code)
	}
	image, err := png.Decode(imageResponse.Body)
	if err != nil {
		t.Fatal(err)
	}
	if image.Bounds().Dx() != 200 || image.Bounds().Dy() != 200 {
		t.Fatalf("sample image dimensions = %v", image.Bounds())
	}
	_, _, _, cornerAlpha := image.At(0, 0).RGBA()
	_, _, _, centerAlpha := image.At(100, 100).RGBA()
	if cornerAlpha != 0 || centerAlpha == 0 {
		t.Fatal("sample PNG transparency does not match the map")
	}

	targetResponse := demoRequest(t, handler, http.MethodGet, "/api/target?board=kaleidoscope", nil)
	var target struct {
		Colors []int8 `json:"colors"`
	}
	if err := json.Unmarshal(targetResponse.Body.Bytes(), &target); err != nil {
		t.Fatal(err)
	}
	if len(target.Colors) != 40000 || target.Colors[100*200+100] < 1 || target.Colors[100*200+100] > 10 {
		t.Fatal("target colors should cover the 200 x 200 map")
	}
	center := 100*200 + 100
	if response := demoRequest(t, handler, http.MethodPost, "/api/events?board=kaleidoscope", map[string]any{"pixelId": center, "color": 2}); response.Code != http.StatusOK {
		t.Fatalf("paint sample = %d", response.Code)
	}
	if response := demoRequest(t, handler, http.MethodPost, "/api/sample-reset?board=kaleidoscope", nil); response.Code != http.StatusOK {
		t.Fatalf("reset sample = %d", response.Code)
	}
	var reset struct {
		Cells []int8 `json:"cells"`
	}
	if err := json.Unmarshal(demoRequest(t, handler, http.MethodGet, "/api/map?board=kaleidoscope", nil).Body.Bytes(), &reset); err != nil {
		t.Fatal(err)
	}
	if reset.Cells[center] != 0 {
		t.Fatal("sample pixel stayed colored after reset")
	}
}

func TestDemoStylesheetIsServed(t *testing.T) {
	response := demoRequest(t, newDemoHandler(), http.MethodGet, "/app.css", nil)
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "text/css; charset=utf-8" {
		t.Fatalf("stylesheet response = %d %q", response.Code, response.Header().Get("Content-Type"))
	}
	if !bytes.Contains(response.Body.Bytes(), []byte(".workspace")) {
		t.Fatal("stylesheet is missing the workspace layout")
	}
}

func TestGalleryHasCanvasOnlyPage(t *testing.T) {
	handler := newDemoHandler()
	script := demoRequest(t, handler, http.MethodGet, "/theme.js", nil)
	if script.Code != http.StatusOK || script.Header().Get("Content-Type") != "text/javascript; charset=utf-8" {
		t.Fatalf("theme script response = %d %q", script.Code, script.Header().Get("Content-Type"))
	}
	page := demoRequest(t, handler, http.MethodGet, "/map", nil)
	if page.Code != http.StatusOK || !bytes.Contains(page.Body.Bytes(), []byte("/theme.js")) || !bytes.Contains(page.Body.Bytes(), []byte("data-theme-toggle")) {
		t.Fatal("canvas is missing the theme control")
	}
	if bytes.Contains(page.Body.Bytes(), []byte("href=\"/events\"")) {
		t.Fatal("canvas still links to the removed events page")
	}
	if events := demoRequest(t, handler, http.MethodGet, "/events", nil); events.Code != http.StatusNotFound {
		t.Fatalf("removed events page status = %d", events.Code)
	}
}

func TestCreatedBoardsAreShareableAndIndependent(t *testing.T) {
	handler := newDemoHandler()
	created := demoRequest(t, handler, http.MethodPost, "/api/boards", map[string]any{"name": "Our little corner"})
	if created.Code != http.StatusCreated {
		t.Fatalf("create board status = %d: %s", created.Code, created.Body.String())
	}
	var board struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &board); err != nil {
		t.Fatal(err)
	}
	if board.ID == "" || board.Name != "Our little corner" {
		t.Fatalf("unexpected board: %+v", board)
	}
	if page := demoRequest(t, handler, http.MethodGet, "/board/"+board.ID, nil); page.Code != http.StatusOK {
		t.Fatalf("shareable board page status = %d", page.Code)
	}
	if response := demoRequest(t, handler, http.MethodPost, "/api/events?board="+board.ID, map[string]any{"pixelId": 0, "color": 3}); response.Code != http.StatusOK {
		t.Fatalf("board event status = %d: %s", response.Code, response.Body.String())
	}
	var own, commons struct {
		Cells []int8 `json:"cells"`
	}
	if err := json.Unmarshal(demoRequest(t, handler, http.MethodGet, "/api/map?board="+board.ID, nil).Body.Bytes(), &own); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(demoRequest(t, handler, http.MethodGet, "/api/map", nil).Body.Bytes(), &commons); err != nil {
		t.Fatal(err)
	}
	if len(own.Cells) != demoCells || own.Cells[0] != 3 || commons.Cells[0] != 0 {
		t.Fatalf("board isolation failed: own=%d commons=%d", own.Cells[0], commons.Cells[0])
	}
	if missing := demoRequest(t, handler, http.MethodGet, "/board/not-a-board", nil); missing.Code != http.StatusNotFound {
		t.Fatalf("unknown board status = %d", missing.Code)
	}
}

func TestBoardsAndPixelsSurviveRestart(t *testing.T) {
	dir := t.TempDir()
	first, err := newDemoHandlerWithStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	created := demoRequest(t, first, http.MethodPost, "/api/boards", map[string]any{"name": "Persistent board"})
	if created.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", created.Code, created.Body.String())
	}
	var board boardSummary
	if err := json.Unmarshal(created.Body.Bytes(), &board); err != nil {
		t.Fatal(err)
	}
	if response := demoRequest(t, first, http.MethodPost, "/api/events?board="+board.ID, map[string]any{"pixelId": 42, "color": 4}); response.Code != http.StatusOK {
		t.Fatalf("paint = %d: %s", response.Code, response.Body.String())
	}
	second, err := newDemoHandlerWithStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if response := demoRequest(t, second, http.MethodGet, "/board/"+board.ID, nil); response.Code != http.StatusOK {
		t.Fatalf("page = %d", response.Code)
	}
	var result struct {
		Cells []int8 `json:"cells"`
	}
	if err := json.Unmarshal(demoRequest(t, second, http.MethodGet, "/api/map?board="+board.ID, nil).Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Cells) != demoCells || result.Cells[42] != 4 {
		t.Fatal("board pixel was not restored")
	}
}

func TestPhotoTargetReplacesBoardAndCanBuildThroughEvents(t *testing.T) {
	dir := t.TempDir()
	handler, err := newDemoHandlerWithStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	created := demoRequest(t, handler, http.MethodPost, "/api/boards", map[string]any{"name": "Photo board"})
	var board boardSummary
	if err := json.Unmarshal(created.Body.Bytes(), &board); err != nil {
		t.Fatal(err)
	}
	path := "?board=" + board.ID
	if response := demoRequest(t, handler, http.MethodPost, "/api/events"+path, map[string]any{"pixelId": 0, "color": 3}); response.Code != http.StatusOK {
		t.Fatalf("first event = %d", response.Code)
	}
	colors := make([]int8, demoCells)
	for i := range colors {
		colors[i] = 2
	}
	colors[10] = 17
	if response := demoRequest(t, handler, http.MethodPut, "/api/photo"+path, map[string]any{"width": 200, "height": 200, "colors": colors}); response.Code != http.StatusBadRequest {
		t.Fatalf("invalid palette status = %d", response.Code)
	}
	colors[10] = 2
	if response := demoRequest(t, handler, http.MethodPut, "/api/photo"+path, map[string]any{"width": 200, "height": 200, "colors": colors}); response.Code != http.StatusOK {
		t.Fatalf("photo status = %d: %s", response.Code, response.Body.String())
	}
	handler, err = newDemoHandlerWithStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Cells []int8 `json:"cells"`
	}
	if err := json.Unmarshal(demoRequest(t, handler, http.MethodGet, "/api/map"+path, nil).Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Cells[0] != 0 {
		t.Fatal("photo did not clear existing pixels")
	}
	var target struct {
		Colors []int8 `json:"colors"`
	}
	if err := json.Unmarshal(demoRequest(t, handler, http.MethodGet, "/api/target"+path, nil).Body.Bytes(), &target); err != nil {
		t.Fatal(err)
	}
	if len(target.Colors) != demoCells || target.Colors[10] != 2 {
		t.Fatal("photo target was not stored")
	}
	if response := demoRequest(t, handler, http.MethodPost, "/api/events/batch"+path, map[string]any{"events": []map[string]int{{"pixelId": 0, "color": 2}}}); response.Code != http.StatusOK {
		t.Fatalf("photo event status = %d", response.Code)
	}
}

func TestPhotoChangesBoardDimensionsAndKeepsThemAfterRestart(t *testing.T) {
	dir := t.TempDir()
	handler, err := newDemoHandlerWithStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	created := demoRequest(t, handler, http.MethodPost, "/api/boards", map[string]any{"name": "Wide photo"})
	var board boardSummary
	if err := json.Unmarshal(created.Body.Bytes(), &board); err != nil {
		t.Fatal(err)
	}
	path := "?board=" + board.ID
	colors := make([]int8, 300*100)
	for i := range colors {
		colors[i] = 2
	}
	if response := demoRequest(t, handler, http.MethodPut, "/api/photo"+path, map[string]any{"width": 300, "height": 100, "colors": colors}); response.Code != http.StatusOK {
		t.Fatalf("resize photo status = %d: %s", response.Code, response.Body.String())
	}
	handler, err = newDemoHandlerWithStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Width  int    `json:"width"`
		Height int    `json:"height"`
		Cells  []int8 `json:"cells"`
	}
	if err := json.Unmarshal(demoRequest(t, handler, http.MethodGet, "/api/map"+path, nil).Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Width != 300 || result.Height != 100 || len(result.Cells) != 30000 {
		t.Fatalf("wrong board dimensions: %dx%d, %d cells", result.Width, result.Height, len(result.Cells))
	}
	if response := demoRequest(t, handler, http.MethodPost, "/api/events"+path, map[string]any{"pixelId": 29999, "color": 3}); response.Code != http.StatusOK {
		t.Fatalf("last pixel = %d", response.Code)
	}
	if response := demoRequest(t, handler, http.MethodPost, "/api/events"+path, map[string]any{"pixelId": 30000, "color": 3}); response.Code != http.StatusBadRequest {
		t.Fatalf("out of bounds pixel = %d", response.Code)
	}
	if response := demoRequest(t, handler, http.MethodPut, "/api/photo"+path, map[string]any{"width": 1000, "height": 1000, "colors": colors}); response.Code != http.StatusBadRequest {
		t.Fatalf("oversized photo = %d", response.Code)
	}
}

func TestOldStoredBoardDefaultsToSquareDimensions(t *testing.T) {
	dir := t.TempDir()
	data, err := json.Marshal(map[string]any{"id": "commons", "name": "The Commons", "cells": make([]int8, demoCells)})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "commons.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
	handler, err := newDemoHandlerWithStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Width  int `json:"width"`
		Height int `json:"height"`
	}
	if err := json.Unmarshal(demoRequest(t, handler, http.MethodGet, "/api/map", nil).Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Width != demoWidth || result.Height != demoHeight {
		t.Fatalf("old board dimensions = %dx%d", result.Width, result.Height)
	}
}

func TestReplacedBoardRejectsStalePaintAfterRestart(t *testing.T) {
	dir := t.TempDir()
	handler, err := newDemoHandlerWithStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	created := demoRequest(t, handler, http.MethodPost, "/api/boards", map[string]any{"name": "Generation board"})
	if created.Code != http.StatusCreated {
		t.Fatalf("create board = %d: %s", created.Code, created.Body.String())
	}
	var board boardSummary
	if err := json.Unmarshal(created.Body.Bytes(), &board); err != nil {
		t.Fatal(err)
	}
	path := "?board=" + board.ID
	colors := make([]int8, 12)
	for i := range colors {
		colors[i] = 2
	}
	photo := demoRequest(t, handler, http.MethodPut, "/api/photo"+path, map[string]any{"width": 4, "height": 3, "colors": colors})
	if photo.Code != http.StatusOK {
		t.Fatalf("photo = %d: %s", photo.Code, photo.Body.String())
	}
	var replaced struct {
		Generation uint64 `json:"generation"`
	}
	if err := json.Unmarshal(photo.Body.Bytes(), &replaced); err != nil {
		t.Fatal(err)
	}
	if replaced.Generation != 1 {
		t.Fatalf("photo generation = %d", replaced.Generation)
	}
	handler, err = newDemoHandlerWithStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	stale := demoRequest(t, handler, http.MethodPost, "/api/events"+path, map[string]any{"pixelId": 0, "color": 3, "generation": 0})
	if stale.Code != http.StatusConflict {
		t.Fatalf("stale event = %d", stale.Code)
	}
	staleBatch := demoRequest(t, handler, http.MethodPost, "/api/events/batch"+path, map[string]any{"events": []map[string]int{{"pixelId": 0, "color": 3}}, "generation": 0})
	if staleBatch.Code != http.StatusConflict {
		t.Fatalf("stale batch = %d", staleBatch.Code)
	}
	current := demoRequest(t, handler, http.MethodPost, "/api/events/batch"+path, map[string]any{"events": []map[string]int{{"pixelId": 0, "color": 3}}, "generation": replaced.Generation})
	if current.Code != http.StatusOK {
		t.Fatalf("current batch = %d: %s", current.Code, current.Body.String())
	}
	var result struct {
		Cells []int8 `json:"cells"`
	}
	if err := json.Unmarshal(demoRequest(t, handler, http.MethodGet, "/api/map"+path, nil).Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Cells[0] != 3 || result.Cells[1] != 0 {
		t.Fatalf("unexpected cells after stale requests: %v", result.Cells[:2])
	}
}

func TestThemedBoardsPersistAndList(t *testing.T) {
	dir := t.TempDir()
	handler, err := newDemoHandlerWithStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	var listed struct {
		Boards []boardSummary `json:"boards"`
	}
	if err := json.Unmarshal(demoRequest(t, handler, http.MethodGet, "/api/boards", nil).Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"garden", "night-sky", "tiny-town"} {
		found := false
		for _, board := range listed.Boards {
			if board.ID == id {
				found = true
			}
		}
		if !found {
			t.Errorf("missing board %s", id)
		}
		if page := demoRequest(t, handler, http.MethodGet, "/board/"+id, nil); page.Code != http.StatusOK {
			t.Errorf("page %s = %d", id, page.Code)
		}
	}
	if paint := demoRequest(t, handler, http.MethodPost, "/api/events?board=garden", map[string]any{"pixelId": 17, "color": 5}); paint.Code != http.StatusOK {
		t.Fatalf("garden paint = %d: %s", paint.Code, paint.Body.String())
	}
	handler, err = newDemoHandlerWithStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Cells []int8 `json:"cells"`
	}
	if err := json.Unmarshal(demoRequest(t, handler, http.MethodGet, "/api/map?board=garden", nil).Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Cells) != demoCells || result.Cells[17] != 5 {
		t.Fatal("garden pixel did not survive restart")
	}
}

func TestThemedBoardsRejectReplacement(t *testing.T) {
	handler := newDemoHandler()
	for _, id := range []string{"garden", "night-sky", "tiny-town"} {
		for _, path := range []string{"/api/photo?board=" + id, "/api/map?board=" + id} {
			response := demoRequest(t, handler, http.MethodPut, path, nil)
			if response.Code != http.StatusForbidden {
				t.Errorf("replace %s = %d, want 403", path, response.Code)
			}
		}
	}
}

func TestBlankOnlyBatchPreservesConcurrentPaint(t *testing.T) {
	handler := newDemoHandler()
	mask := make([]int8, demoCells)
	mask[2] = -1
	if response := demoRequest(t, handler, http.MethodPut, "/api/map", map[string]any{"cells": mask}); response.Code != http.StatusOK {
		t.Fatalf("mask = %d", response.Code)
	}
	if response := demoRequest(t, handler, http.MethodPost, "/api/events", map[string]any{"pixelId": 0, "color": 5}); response.Code != http.StatusOK {
		t.Fatalf("existing paint = %d", response.Code)
	}
	response := demoRequest(t, handler, http.MethodPost, "/api/events/batch", map[string]any{"onlyBlank": true, "events": []map[string]int{{"pixelId": 0, "color": 2}, {"pixelId": 1, "color": 3}, {"pixelId": 2, "color": 4}}})
	if response.Code != http.StatusOK {
		t.Fatalf("blank-only batch = %d: %s", response.Code, response.Body.String())
	}
	var applied struct {
		Events []struct {
			PixelID int `json:"pixelId"`
			Color   int `json:"color"`
		} `json:"events"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &applied); err != nil {
		t.Fatal(err)
	}
	if len(applied.Events) != 1 || applied.Events[0].PixelID != 1 || applied.Events[0].Color != 3 {
		t.Fatalf("applied events = %+v", applied.Events)
	}
	var board struct {
		Cells []int8 `json:"cells"`
	}
	if err := json.Unmarshal(demoRequest(t, handler, http.MethodGet, "/api/map", nil).Body.Bytes(), &board); err != nil {
		t.Fatal(err)
	}
	if board.Cells[0] != 5 || board.Cells[1] != 3 || board.Cells[2] != -1 {
		t.Fatalf("cells = %v", board.Cells[:3])
	}
	for _, bad := range []map[string]int{{"pixelId": demoCells, "color": 3}, {"pixelId": 3, "color": 17}} {
		if response := demoRequest(t, handler, http.MethodPost, "/api/events/batch", map[string]any{"onlyBlank": true, "events": []map[string]int{bad}}); response.Code != http.StatusBadRequest {
			t.Errorf("invalid event %+v = %d", bad, response.Code)
		}
	}
}

func TestBlankOnlyBatchEmptyResult(t *testing.T) {
	handler := newDemoHandler()
	demoRequest(t, handler, http.MethodPost, "/api/events", map[string]any{"pixelId": 0, "color": 5})
	response := demoRequest(t, handler, http.MethodPost, "/api/events/batch", map[string]any{"onlyBlank": true, "events": []map[string]int{{"pixelId": 0, "color": 2}}})
	if response.Code != http.StatusOK {
		t.Fatalf("empty batch = %d", response.Code)
	}
	var result struct {
		Events []pixelEvent `json:"events"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Events) != 0 {
		t.Fatalf("applied events = %d", len(result.Events))
	}
	var board struct {
		Cells []int8 `json:"cells"`
	}
	if err := json.Unmarshal(demoRequest(t, handler, http.MethodGet, "/api/map", nil).Body.Bytes(), &board); err != nil {
		t.Fatal(err)
	}
	if board.Cells[0] != 5 {
		t.Fatalf("existing pixel changed to %d", board.Cells[0])
	}
}

func TestBlankOnlyBatchBroadcastsAppliedEvents(t *testing.T) {
	hub := newDemoHub("")
	handler := newDemoHandlerWithHub(hub)
	demoRequest(t, handler, http.MethodPost, "/api/events", map[string]any{"pixelId": 0, "color": 5})
	board := hub.lookup("commons")
	updates := make(chan []byte, 1)
	board.mu.Lock()
	board.subscribers[updates] = struct{}{}
	board.mu.Unlock()
	defer func() { board.mu.Lock(); delete(board.subscribers, updates); board.mu.Unlock() }()
	demoRequest(t, handler, http.MethodPost, "/api/events/batch", map[string]any{"onlyBlank": true, "events": []map[string]int{{"pixelId": 0, "color": 2}, {"pixelId": 1, "color": 3}}})
	select {
	case data := <-updates:
		var message struct {
			Events []struct {
				PixelID int `json:"pixelId"`
			} `json:"events"`
		}
		if err := json.Unmarshal(data, &message); err != nil {
			t.Fatal(err)
		}
		if len(message.Events) != 1 || message.Events[0].PixelID != 1 {
			t.Fatalf("broadcast events = %+v", message.Events)
		}
	default:
		t.Fatal("no live update broadcast")
	}
	demoRequest(t, handler, http.MethodPost, "/api/events/batch", map[string]any{"onlyBlank": true, "events": []map[string]int{{"pixelId": 0, "color": 2}}})
	select {
	case <-updates:
		t.Fatal("empty batch broadcast an update")
	default:
	}
}

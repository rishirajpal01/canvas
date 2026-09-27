package localserver

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDailyRolloverAndArchive(t *testing.T) {
	zone := time.FixedZone("IST", 5*3600+30*60)
	now := time.Date(2026, 9, 26, 23, 59, 0, 0, zone)
	hub := newDemoHub(t.TempDir())
	hub.now = func() time.Time { return now }
	handler := newDemoHandlerWithHub(hub)
	first := demoRequest(t, handler, http.MethodGet, "/api/daily/today", nil)
	if first.Code != http.StatusOK {
		t.Fatalf("today = %d: %s", first.Code, first.Body.String())
	}
	var day1 dailyInfo
	if err := json.Unmarshal(first.Body.Bytes(), &day1); err != nil {
		t.Fatal(err)
	}
	if day1.ID != "daily-2026-09-26" || day1.Date != "2026-09-26" || day1.Prompt == "" {
		t.Fatalf("day 1 = %+v", day1)
	}
	if page := demoRequest(t, handler, http.MethodGet, "/board/daily", nil); page.Code != http.StatusOK {
		t.Fatalf("today page = %d", page.Code)
	}
	if paint := demoRequest(t, handler, http.MethodPost, "/api/events?board="+day1.ID, map[string]any{"pixelId": 4, "color": 3}); paint.Code != http.StatusOK {
		t.Fatalf("day 1 paint = %d", paint.Code)
	}
	now = now.Add(time.Minute)
	second := demoRequest(t, handler, http.MethodGet, "/api/daily/today", nil)
	var day2 dailyInfo
	if err := json.Unmarshal(second.Body.Bytes(), &day2); err != nil {
		t.Fatal(err)
	}
	if day2.ID != "daily-2026-09-27" || day2.Date != "2026-09-27" || day2.Prompt == "" || day2.Prompt == day1.Prompt {
		t.Fatalf("day 2 = %+v, day 1 = %+v", day2, day1)
	}
	if oldPaint := demoRequest(t, handler, http.MethodPost, "/api/events?board="+day1.ID, map[string]any{"pixelId": 5, "color": 3}); oldPaint.Code != http.StatusForbidden {
		t.Fatalf("archived paint = %d", oldPaint.Code)
	}
	if oldBatch := demoRequest(t, handler, http.MethodPost, "/api/events/batch?board="+day1.ID, map[string]any{"events": []map[string]int{{"pixelId": 5, "color": 3}}}); oldBatch.Code != http.StatusForbidden {
		t.Fatalf("archived batch = %d", oldBatch.Code)
	}
	for _, path := range []string{"/api/photo?board=" + day2.ID, "/api/map?board=" + day2.ID} {
		if replace := demoRequest(t, handler, http.MethodPut, path, nil); replace.Code != http.StatusForbidden {
			t.Errorf("replace %s = %d", path, replace.Code)
		}
	}
	var current, archived struct {
		Cells []int8 `json:"cells"`
	}
	if err := json.Unmarshal(demoRequest(t, handler, http.MethodGet, "/api/map?board="+day2.ID, nil).Body.Bytes(), &current); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(demoRequest(t, handler, http.MethodGet, "/api/map?board="+day1.ID, nil).Body.Bytes(), &archived); err != nil {
		t.Fatal(err)
	}
	if current.Cells[4] != 0 || archived.Cells[4] != 3 || archived.Cells[5] != 0 {
		t.Fatalf("daily boards mixed: current=%d archived=%v", current.Cells[4], archived.Cells[4:6])
	}
	var archive struct {
		Boards []dailyInfo `json:"boards"`
	}
	if err := json.Unmarshal(demoRequest(t, handler, http.MethodGet, "/api/daily/archive", nil).Body.Bytes(), &archive); err != nil {
		t.Fatal(err)
	}
	if len(archive.Boards) != 1 || archive.Boards[0] != day1 {
		t.Fatalf("archive = %+v", archive.Boards)
	}
	var listed struct {
		Boards []boardSummary `json:"boards"`
	}
	if err := json.Unmarshal(demoRequest(t, handler, http.MethodGet, "/api/boards", nil).Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	dailyCount := 0
	for _, board := range listed.Boards {
		if board.ID == "daily" {
			dailyCount++
		}
		if board.ID == day1.ID || board.ID == day2.ID {
			t.Errorf("dated board leaked into main list: %s", board.ID)
		}
	}
	if dailyCount != 1 {
		t.Fatalf("daily aliases = %d", dailyCount)
	}
}

func TestDailyArchiveSurvivesRestart(t *testing.T) {
	zone := time.FixedZone("IST", 5*3600+30*60)
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, zone)
	dir := t.TempDir()
	firstHub := newDemoHub(dir)
	firstHub.now = func() time.Time { return now }
	first := newDemoHandlerWithHub(firstHub)
	var created dailyInfo
	if err := json.Unmarshal(demoRequest(t, first, http.MethodGet, "/api/daily/today", nil).Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if response := demoRequest(t, first, http.MethodPost, "/api/events?board="+created.ID, map[string]any{"pixelId": 9, "color": 7}); response.Code != http.StatusOK {
		t.Fatalf("paint = %d", response.Code)
	}
	now = now.AddDate(0, 0, 1)
	secondHub := newDemoHub(dir)
	secondHub.now = func() time.Time { return now }
	if err := secondHub.loadBoards(); err != nil {
		t.Fatal(err)
	}
	second := newDemoHandlerWithHub(secondHub)
	var archive struct {
		Boards []dailyInfo `json:"boards"`
	}
	if err := json.Unmarshal(demoRequest(t, second, http.MethodGet, "/api/daily/archive", nil).Body.Bytes(), &archive); err != nil {
		t.Fatal(err)
	}
	if len(archive.Boards) != 1 || archive.Boards[0] != created {
		t.Fatalf("restored archive = %+v, want %+v", archive.Boards, created)
	}
	var board struct {
		Cells []int8 `json:"cells"`
	}
	if err := json.Unmarshal(demoRequest(t, second, http.MethodGet, "/api/map?board="+created.ID, nil).Body.Bytes(), &board); err != nil {
		t.Fatal(err)
	}
	if board.Cells[9] != 7 {
		t.Fatalf("archived pixel = %d", board.Cells[9])
	}
	if page := demoRequest(t, second, http.MethodGet, "/board/"+created.ID, nil); page.Code != http.StatusOK {
		t.Fatalf("archive page = %d", page.Code)
	}
}

func TestDailyRetentionWindow(t *testing.T) {
	zone := time.FixedZone("IST", 5*3600+30*60)
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, zone)
	dir := t.TempDir()
	hub := newDemoHub(dir)
	hub.now = func() time.Time { return now }
	handler := newDemoHandlerWithHub(hub)
	for day := 0; day < 8; day++ {
		var info dailyInfo
		response := demoRequest(t, handler, http.MethodGet, "/api/daily/today", nil)
		if err := json.Unmarshal(response.Body.Bytes(), &info); err != nil {
			t.Fatal(err)
		}
		if paint := demoRequest(t, handler, http.MethodPost, "/api/events?board="+info.ID, map[string]any{"pixelId": day, "color": day + 1}); paint.Code != http.StatusOK {
			t.Fatalf("day %d paint = %d", day, paint.Code)
		}
		now = now.AddDate(0, 0, 1)
	}
	now = now.AddDate(0, 0, -1)
	if response := demoRequest(t, handler, http.MethodGet, "/api/daily/archive", nil); response.Code != http.StatusOK {
		t.Fatalf("archive = %d", response.Code)
	}
	oldID := "daily-2026-09-20"
	if response := demoRequest(t, handler, http.MethodGet, "/api/map?board="+oldID, nil); response.Code != http.StatusNotFound {
		t.Fatalf("expired map = %d", response.Code)
	}
	if response := demoRequest(t, handler, http.MethodGet, "/board/"+oldID, nil); response.Code != http.StatusNotFound {
		t.Fatalf("expired page = %d", response.Code)
	}
	if _, err := os.Stat(filepath.Join(dir, oldID+".json")); !os.IsNotExist(err) {
		t.Fatalf("expired file still exists: %v", err)
	}
	for day := 1; day < 8; day++ {
		date := time.Date(2026, 9, 20+day, 12, 0, 0, 0, zone).Format("2006-01-02")
		if _, err := os.Stat(filepath.Join(dir, "daily-"+date+".json")); err != nil {
			t.Fatalf("retained %s: %v", date, err)
		}
	}
	var archive struct {
		Boards []dailyInfo `json:"boards"`
	}
	if err := json.Unmarshal(demoRequest(t, handler, http.MethodGet, "/api/daily/archive", nil).Body.Bytes(), &archive); err != nil {
		t.Fatal(err)
	}
	if len(archive.Boards) != 6 || archive.Boards[0].Date != "2026-09-26" || archive.Boards[5].Date != "2026-09-21" {
		t.Fatalf("archive window = %+v", archive.Boards)
	}
	restarted := newDemoHub(dir)
	restarted.now = func() time.Time { return now }
	if err := restarted.loadBoards(); err != nil {
		t.Fatal(err)
	}
	if len(restarted.boards) != 7 {
		t.Fatalf("retained boards after restart = %d", len(restarted.boards))
	}
	for day := 1; day < 8; day++ {
		date := time.Date(2026, 9, 20+day, 12, 0, 0, 0, zone).Format("2006-01-02")
		board := restarted.boards["daily-"+date]
		if board == nil || board.prompt == "" || board.cells[day] != int8(day+1) {
			t.Fatalf("retained board %s incomplete", date)
		}
	}
}

func TestDailyPrunePreservesOtherFiles(t *testing.T) {
	zone := time.FixedZone("IST", 5*3600+30*60)
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, zone)
	dir := t.TempDir()
	hub := newDemoHub(dir)
	hub.now = func() time.Time { return now }
	for _, name := range []string{"commons.json", "daily-2026-9-20.json", "notes.json"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("keep"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "daily-2026-09-20.json"), []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := hub.pruneDaily(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"commons.json", "daily-2026-9-20.json", "notes.json"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("deleted unrelated %s: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "daily-2026-09-20.json")); !os.IsNotExist(err) {
		t.Fatalf("expired daily file remains: %v", err)
	}
}

func TestDailyPruneClosesStream(t *testing.T) {
	zone := time.FixedZone("IST", 5*3600+30*60)
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, zone)
	hub := newDemoHub(t.TempDir())
	hub.now = func() time.Time { return now }
	_, board, err := hub.today()
	if err != nil {
		t.Fatal(err)
	}
	channel := make(chan []byte, 1)
	board.mu.Lock()
	board.subscribers[channel] = struct{}{}
	board.mu.Unlock()
	now = now.AddDate(0, 0, 7)
	if err := hub.pruneDaily(); err != nil {
		t.Fatal(err)
	}
	if _, ok := <-channel; ok {
		t.Fatal("expired stream remained open")
	}
	if hub.lookup(board.id) != nil {
		t.Fatal("expired board remained in memory")
	}
}

func TestIndiaMidnightDelay(t *testing.T) {
	zone := time.FixedZone("IST", 5*3600+30*60)
	now := time.Date(2026, 9, 26, 23, 59, 0, 0, zone)
	if delay := durationUntilNextIndiaMidnight(now); delay != time.Minute {
		t.Fatalf("delay = %v", delay)
	}
}

func TestDailyStartupPrunesExpiredFiles(t *testing.T) {
	dir := t.TempDir()
	date := time.Now().In(indiaTime).AddDate(0, 0, -7).Format("2006-01-02")
	path := filepath.Join(dir, "daily-"+date+".json")
	if err := os.WriteFile(path, []byte("expired"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := newDemoHubWithStore(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expired startup file remains: %v", err)
	}
}

func TestDailyPageMetadata(t *testing.T) {
	zone := time.FixedZone("IST", 5*3600+30*60)
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, zone)
	hub := newDemoHub(t.TempDir())
	hub.now = func() time.Time { return now }
	handler := newDemoHandlerWithHub(hub)
	var today dailyInfo
	if err := json.Unmarshal(demoRequest(t, handler, http.MethodGet, "/api/daily/today", nil).Body.Bytes(), &today); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/board/daily", "/board/" + today.ID} {
		page := demoRequest(t, handler, http.MethodGet, path, nil)
		if page.Code != http.StatusOK {
			t.Fatalf("page %s = %d", path, page.Code)
		}
		for _, hook := range [][]byte{[]byte(`id="daily-panel"`), []byte(`id="daily-date"`), []byte(`id="daily-archive"`), []byte(`id="archive-lock"`), []byte(`data-tip="This mosaic is archived and read only."`)} {
			if !bytes.Contains(page.Body.Bytes(), hook) {
				t.Errorf("page %s missing %s", path, hook)
			}
		}
		if bytes.Contains(page.Body.Bytes(), []byte(`id="daily-prompt"`)) {
			t.Errorf("page %s still displays a theme name", path)
		}
	}
	var mapData struct {
		Date   string `json:"date"`
		Prompt string `json:"prompt"`
	}
	if err := json.Unmarshal(demoRequest(t, handler, http.MethodGet, "/api/map?board="+today.ID, nil).Body.Bytes(), &mapData); err != nil {
		t.Fatal(err)
	}
	if mapData.Date != today.Date || mapData.Prompt != today.Prompt {
		t.Fatalf("map metadata = %+v, today = %+v", mapData, today)
	}
	now = now.AddDate(0, 0, 1)
	var archived struct {
		Date   string `json:"date"`
		Prompt string `json:"prompt"`
	}
	if err := json.Unmarshal(demoRequest(t, handler, http.MethodGet, "/api/map?board="+today.ID, nil).Body.Bytes(), &archived); err != nil {
		t.Fatal(err)
	}
	if archived.Date != today.Date || archived.Prompt != today.Prompt {
		t.Fatalf("archived metadata = %+v", archived)
	}
	var listed struct {
		Boards []boardSummary `json:"boards"`
	}
	if err := json.Unmarshal(demoRequest(t, handler, http.MethodGet, "/api/boards", nil).Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	daily := 0
	for _, board := range listed.Boards {
		if board.ID == "daily" {
			daily++
		}
		if isDailyBoardID(board.ID) {
			t.Errorf("dated board in navigation: %s", board.ID)
		}
	}
	if daily != 1 {
		t.Fatalf("daily navigation entries = %d", daily)
	}
}

type clockFlipReader struct {
	reader io.Reader
	flip   func()
}

func (r *clockFlipReader) Read(p []byte) (int, error) {
	if r.flip != nil {
		r.flip()
		r.flip = nil
	}
	return r.reader.Read(p)
}

func TestDailyWriteCrossingMidnightIsRejected(t *testing.T) {
	zone := time.FixedZone("IST", 5*3600+30*60)
	now := time.Date(2026, 9, 26, 23, 59, 59, 0, zone)
	hub := newDemoHub(t.TempDir())
	hub.now = func() time.Time { return now }
	handler := newDemoHandlerWithHub(hub)
	info, _, err := hub.today()
	if err != nil {
		t.Fatal(err)
	}
	body := &clockFlipReader{reader: bytes.NewBufferString(`{"pixelId":17,"color":3}`), flip: func() { now = now.Add(time.Second) }}
	request := httptest.NewRequest(http.MethodPost, "/api/events?board="+info.ID, body)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("late paint = %d: %s", response.Code, response.Body.String())
	}
	board := hub.lookup(info.ID)
	if board == nil || board.cells[17] != 0 {
		t.Fatal("late paint changed archived board")
	}
}

package localserver

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var indiaTime = time.FixedZone("IST", 5*3600+30*60)

var dailyPrompts = [...]string{
	"A garden after the rain",
	"A city under the stars",
	"The view from your window",
	"A place you want to visit",
	"A tiny world in a teacup",
	"A festival of colors",
	"Something that feels like home",
}

type dailyInfo struct {
	ID     string `json:"id"`
	Date   string `json:"date"`
	Prompt string `json:"prompt"`
}

func isDailyBoardID(id string) bool {
	if !strings.HasPrefix(id, "daily-") {
		return false
	}
	date := strings.TrimPrefix(id, "daily-")
	parsed, err := time.Parse("2006-01-02", date)
	return err == nil && parsed.Format("2006-01-02") == date
}

func (h *demoHub) todayDate() string {
	return h.now().In(indiaTime).Format("2006-01-02")
}

func (h *demoHub) dailyCutoff() string {
	return h.now().In(indiaTime).AddDate(0, 0, -6).Format("2006-01-02")
}

func (h *demoHub) expiredDailyID(id string) bool {
	return isDailyBoardID(id) && strings.TrimPrefix(id, "daily-") < h.dailyCutoff()
}

func (h *demoHub) pruneDaily() error {
	cutoff := h.dailyCutoff()
	h.mu.Lock()
	defer h.mu.Unlock()
	var firstErr error
	if h.dir != "" {
		files, err := os.ReadDir(h.dir)
		if err != nil {
			return err
		}
		for _, file := range files {
			if file.IsDir() || filepath.Ext(file.Name()) != ".json" {
				continue
			}
			id := strings.TrimSuffix(file.Name(), ".json")
			if !isDailyBoardID(id) || strings.TrimPrefix(id, "daily-") >= cutoff {
				continue
			}
			board := h.boards[id]
			if board != nil {
				board.mu.Lock()
			}
			removeErr := os.Remove(filepath.Join(h.dir, file.Name()))
			if board != nil {
				board.mu.Unlock()
			}
			if removeErr != nil && firstErr == nil {
				firstErr = removeErr
			}
		}
	}
	for id, board := range h.boards {
		if !isDailyBoardID(id) || board.date >= cutoff {
			continue
		}
		board.mu.Lock()
		for channel := range board.subscribers {
			close(channel)
			delete(board.subscribers, channel)
		}
		board.mu.Unlock()
		delete(h.boards, id)
	}
	return firstErr
}

func durationUntilNextIndiaMidnight(now time.Time) time.Duration {
	local := now.In(indiaTime)
	next := time.Date(local.Year(), local.Month(), local.Day()+1, 0, 0, 0, 0, indiaTime)
	return next.Sub(now)
}

func (s *demoState) dailyWritableLocked() bool {
	if s.date == "" {
		return true
	}
	clock := s.now
	if clock == nil {
		clock = time.Now
	}
	return s.date == clock().In(indiaTime).Format("2006-01-02")
}

func (h *demoHub) runDailyMaintenance(ctx context.Context) {
	for {
		if err := h.pruneDaily(); err != nil {
			log.Printf("daily cleanup: %v", err)
		}
		timer := time.NewTimer(durationUntilNextIndiaMidnight(h.now()))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (h *demoHub) today() (dailyInfo, *demoState, error) {
	date := h.todayDate()
	id := "daily-" + date
	h.mu.Lock()
	defer h.mu.Unlock()
	if board := h.boards[id]; board != nil {
		return dailyInfo{ID: id, Date: board.date, Prompt: board.prompt}, board, nil
	}
	parsed, _ := time.Parse("2006-01-02", date)
	index := (parsed.Unix() / 86400) % int64(len(dailyPrompts))
	prompt := dailyPrompts[index]
	board := &demoState{
		id: id, name: "Daily Mosaic", date: date, prompt: prompt, now: h.now,
		width: demoWidth, height: demoHeight, cells: make([]int8, demoCells),
		dir: h.dir, subscribers: make(map[chan []byte]struct{}),
	}
	if err := board.saveLocked(board.cells, nil); err != nil {
		return dailyInfo{}, nil, err
	}
	h.boards[id] = board
	return dailyInfo{ID: id, Date: date, Prompt: prompt}, board, nil
}

func (h *demoHub) getToday(w http.ResponseWriter, r *http.Request) {
	info, _, err := h.today()
	if err != nil {
		http.Error(w, "Daily board unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

func (h *demoHub) getDailyArchive(w http.ResponseWriter, r *http.Request) {
	date := h.todayDate()
	cutoff := h.dailyCutoff()
	h.mu.RLock()
	archive := make([]dailyInfo, 0)
	for _, board := range h.boards {
		if isDailyBoardID(board.id) && board.date >= cutoff && board.date < date {
			archive = append(archive, dailyInfo{ID: board.id, Date: board.date, Prompt: board.prompt})
		}
	}
	h.mu.RUnlock()
	sort.Slice(archive, func(i, j int) bool { return archive[i].Date > archive[j].Date })
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"boards": archive})
}

func needsDailyMaintenance(r *http.Request) bool {
	return r.URL.Path == "/api/boards" || strings.HasPrefix(r.URL.Path, "/api/daily/") || strings.HasPrefix(r.URL.Path, "/board/daily") || r.URL.Query().Get("board") == "daily" || isDailyBoardID(r.URL.Query().Get("board"))
}

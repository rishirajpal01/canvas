package localserver

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

type demoHub struct {
	mu     sync.RWMutex
	boards map[string]*demoState
	dir    string
	now    func() time.Time
}

type boardSummary struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
}

func newDemoHub(dir string) *demoHub {
	return &demoHub{boards: make(map[string]*demoState), dir: dir, now: time.Now}
}

func newDemoHandlerWithStore(dir string) (http.Handler, error) {
	hub, err := newDemoHubWithStore(dir)
	if err != nil {
		return nil, err
	}
	return newDemoHandlerWithHub(hub), nil
}

func newDemoHubWithStore(dir string) (*demoHub, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	hub := newDemoHub(dir)
	if err := hub.pruneDaily(); err != nil {
		log.Printf("daily startup cleanup: %v", err)
	}
	if err := hub.loadBoards(); err != nil {
		return nil, err
	}
	if err := hub.pruneCustom(); err != nil {
		log.Printf("custom startup cleanup: %v", err)
	}
	return hub, nil
}

type storedBoard struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Width         int        `json:"width"`
	Height        int        `json:"height"`
	Cells         []int8     `json:"cells"`
	Target        []int8     `json:"target,omitempty"`
	Generation    uint64     `json:"generation,omitempty"`
	Date          string     `json:"date,omitempty"`
	Prompt        string     `json:"prompt,omitempty"`
	CreatedAt     *time.Time `json:"createdAt,omitempty"`
	PaletteScheme []int8     `json:"paletteScheme,omitempty"`
}

func validBoardID(id string) bool {
	if id == "commons" || id == "kaleidoscope" || themedBoardID(id) {
		return true
	}
	return isDailyBoardID(id) || customBoardID(id)
}

func customBoardID(id string) bool {
	if len(id) != 24 {
		return false
	}
	for _, char := range id {
		if !strings.ContainsRune("0123456789abcdef", char) {
			return false
		}
	}
	return true
}

func themedBoardID(id string) bool {
	switch id {
	case "garden", "night-sky", "tiny-town":
		return true
	default:
		return isDailyBoardID(id)
	}
}

func (h *demoHub) loadBoards() error {
	files, err := os.ReadDir(h.dir)
	if err != nil {
		return err
	}
	for _, file := range files {
		if file.IsDir() || filepath.Ext(file.Name()) != ".json" {
			continue
		}
		id := strings.TrimSuffix(file.Name(), ".json")
		if !validBoardID(id) {
			continue
		}
		if h.expiredDailyID(id) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(h.dir, file.Name()))
		if err != nil {
			return err
		}
		var stored storedBoard
		if err := json.Unmarshal(data, &stored); err != nil {
			return fmt.Errorf("load board %s: %w", id, err)
		}
		// Boards saved before variable dimensions always used 200 × 200.
		if stored.Width == 0 && stored.Height == 0 {
			stored.Width, stored.Height = demoWidth, demoHeight
		}
		if stored.ID != id || !validBoardName(stored.Name) || !validBoardDimensions(stored.Width, stored.Height) || len(stored.Cells) != stored.Width*stored.Height || (len(stored.Target) != 0 && len(stored.Target) != len(stored.Cells)) || (isDailyBoardID(id) && (stored.Date != strings.TrimPrefix(id, "daily-") || stored.Prompt == "")) {
			return fmt.Errorf("invalid stored board %s", id)
		}
		for _, cell := range stored.Cells {
			if cell < -1 || cell > demoPaletteSize {
				return fmt.Errorf("invalid cell in board %s", id)
			}
		}
		for _, color := range stored.Target {
			if color < 1 || color > demoPaletteSize {
				return fmt.Errorf("invalid target in board %s", id)
			}
		}
		board := &demoState{id: id, name: stored.Name, date: stored.Date, prompt: stored.Prompt, paletteScheme: stored.PaletteScheme, now: h.now, width: stored.Width, height: stored.Height, cells: stored.Cells, target: stored.Target, generation: stored.Generation, dir: h.dir, subscribers: make(map[chan []byte]struct{})}
		if stored.CreatedAt != nil {
			board.createdAt = *stored.CreatedAt
		}
		if customBoardID(id) && board.createdAt.IsZero() {
			board.createdAt = h.now().UTC()
			if err := board.saveLocked(board.cells, board.target); err != nil {
				return fmt.Errorf("migrate board %s: %w", id, err)
			}
		}
		h.boards[id] = board
	}
	return nil
}

func (s *demoState) saveLocked(cells, target []int8) error {
	return s.saveDimensionsLocked(s.width, s.height, cells, target, s.generation)
}

func (s *demoState) saveDimensionsLocked(width, height int, cells, target []int8, generation uint64) error {
	if s.dir == "" {
		return nil
	}
	file, err := os.CreateTemp(s.dir, ".board-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	var createdAt *time.Time
	if !s.createdAt.IsZero() {
		createdAt = &s.createdAt
	}
	if err := json.NewEncoder(file).Encode(storedBoard{ID: s.id, Name: s.name, Width: width, Height: height, Cells: cells, Target: target, Generation: generation, Date: s.date, Prompt: s.prompt, CreatedAt: createdAt, PaletteScheme: s.paletteScheme}); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), filepath.Join(s.dir, s.id+".json"))
}

func (h *demoHub) addBuiltinBoards(sample, target []int8) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.boards["commons"] == nil {
		h.boards["commons"] = &demoState{id: "commons", name: "The Commons", width: demoWidth, height: demoHeight, cells: make([]int8, demoCells), dir: h.dir, subscribers: make(map[chan []byte]struct{})}
	}
	if h.boards["kaleidoscope"] == nil {
		h.boards["kaleidoscope"] = &demoState{id: "kaleidoscope", name: "Kaleidoscope", width: demoWidth, height: demoHeight, cells: sample, target: target, dir: h.dir, subscribers: make(map[chan []byte]struct{})}
	}
	for id, name := range map[string]string{"garden": "Pixel Garden", "night-sky": "Night Sky", "tiny-town": "Tiny Town"} {
		if h.boards[id] == nil {
			h.boards[id] = &demoState{id: id, name: name, width: demoWidth, height: demoHeight, cells: make([]int8, demoCells), dir: h.dir, subscribers: make(map[chan []byte]struct{})}
		}
	}
}

func (h *demoHub) lookup(id string) *demoState {
	if h.expiredDailyID(id) {
		return nil
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	board := h.boards[id]
	if board != nil && board.expiredCustom() {
		return nil
	}
	return board
}

func (h *demoHub) fromRequest(w http.ResponseWriter, r *http.Request) *demoState {
	id := r.URL.Query().Get("board")
	if id == "" {
		id = "commons"
	}
	if id == "daily" {
		_, board, err := h.today()
		if err != nil {
			http.Error(w, "Daily board unavailable", http.StatusInternalServerError)
			return nil
		}
		return board
	}
	board := h.lookup(id)
	if board == nil {
		http.NotFound(w, r)
	}
	return board
}

func (h *demoHub) withBoard(action func(*demoState, http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		board := h.fromRequest(w, r)
		if board != nil {
			if r.Method != http.MethodGet && isDailyBoardID(board.id) && board.date != h.todayDate() {
				http.Error(w, "This daily board is archived", http.StatusForbidden)
				return
			}
			action(board, w, r)
		}
	}
}

func (h *demoHub) listBoards(w http.ResponseWriter, r *http.Request) {
	if _, _, err := h.today(); err != nil {
		http.Error(w, "Daily board unavailable", http.StatusInternalServerError)
		return
	}
	h.mu.RLock()
	boards := make([]boardSummary, 0, len(h.boards)+1)
	for _, board := range h.boards {
		if isDailyBoardID(board.id) || board.expiredCustom() {
			continue
		}
		boards = append(boards, board.summary())
	}
	h.mu.RUnlock()
	boards = append(boards, boardSummary{ID: "daily", Name: "Daily Mosaic"})
	sort.Slice(boards, func(i, j int) bool { return boards[i].Name < boards[j].Name })
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"boards": boards})
}

func validBoardName(name string) bool {
	if name == "" || utf8.RuneCountInString(name) > 40 {
		return false
	}
	for _, char := range name {
		if unicode.IsControl(char) {
			return false
		}
	}
	return true
}

func (h *demoHub) createBoard(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Name string `json:"name"`
	}
	if !decodeDemoJSON(w, r, &request) {
		return
	}
	name := strings.TrimSpace(request.Name)
	if !validBoardName(name) {
		http.Error(w, "Board name must contain 1–40 printable characters", http.StatusBadRequest)
		return
	}
	bytes := make([]byte, 12)
	if _, err := rand.Read(bytes); err != nil {
		http.Error(w, "Could not create board", http.StatusInternalServerError)
		return
	}
	id := hex.EncodeToString(bytes)
	board := &demoState{id: id, name: name, createdAt: h.now().UTC(), now: h.now, width: demoWidth, height: demoHeight, cells: make([]int8, demoCells), dir: h.dir, subscribers: make(map[chan []byte]struct{})}
	if err := board.saveLocked(board.cells, nil); err != nil {
		http.Error(w, "Could not save board", http.StatusInternalServerError)
		return
	}
	h.mu.Lock()
	h.boards[id] = board
	h.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(board.summary())
}

func (s *demoState) expiredCustom() bool {
	return customBoardID(s.id) && !s.createdAt.IsZero() && !s.now().Before(s.createdAt.Add(7*24*time.Hour))
}

func (s *demoState) summary() boardSummary {
	result := boardSummary{ID: s.id, Name: s.name}
	if customBoardID(s.id) && !s.createdAt.IsZero() {
		expires := s.createdAt.Add(7 * 24 * time.Hour)
		result.ExpiresAt = &expires
	}
	return result
}

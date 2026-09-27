package localserver

import (
	"net/http"
	"testing"
)

func TestKaleidoscopeRerollIsSymmetricAndFresh(t *testing.T) {
	dir := t.TempDir()
	hub := newDemoHub(dir)
	handler := newDemoHandlerWithHub(hub)
	var before []int8
	for turn := 0; turn < 2; turn++ {
		response := demoRequest(t, handler, http.MethodPost, "/api/kaleidoscope/reroll?board=kaleidoscope", map[string]any{"generation": turn})
		if response.Code != http.StatusOK {
			t.Fatalf("reroll %d = %d: %s", turn, response.Code, response.Body.String())
		}
		board := hub.lookup("kaleidoscope")
		board.mu.Lock()
		cells := append([]int8(nil), board.cells...)
		board.mu.Unlock()
		blank := 0
		for y := 0; y < demoHeight; y++ {
			for x := 0; x < demoWidth; x++ {
				cell := cells[y*demoWidth+x]
				if cell < -1 || cell > demoPaletteSize {
					t.Fatalf("invalid cell %d", cell)
				}
				if cell == 0 {
					blank++
				}
				if cell > 0 {
					t.Fatal("new design should be empty")
				}
				for _, point := range [][2]int{{demoWidth - 1 - x, y}, {x, demoHeight - 1 - y}, {y, x}} {
					if cells[point[1]*demoWidth+point[0]] != cell {
						t.Fatalf("asymmetry at (%d,%d)", x, y)
					}
				}
			}
		}
		if blank < 200 {
			t.Fatalf("design too sparse: %d", blank)
		}
		if len(board.target) != 0 {
			t.Fatal("empty design has a color target")
		}
		if turn > 0 {
			if equalCells(cells, before) {
				t.Fatal("reroll repeated design")
			}
		}
		before = cells
	}
	if response := demoRequest(t, handler, http.MethodPost, "/api/kaleidoscope/reroll?board=commons", map[string]any{"generation": 0}); response.Code != http.StatusForbidden {
		t.Fatalf("other board reroll = %d", response.Code)
	}
	if response := demoRequest(t, handler, http.MethodPost, "/api/kaleidoscope/reroll?board=kaleidoscope", map[string]any{"generation": 0}); response.Code != http.StatusConflict {
		t.Fatalf("stale reroll = %d", response.Code)
	}
	loaded := newDemoHub(dir)
	if err := loaded.loadBoards(); err != nil {
		t.Fatal(err)
	}
	if board := loaded.lookup("kaleidoscope"); board == nil || !equalCells(board.cells, before) {
		t.Fatal("reroll did not survive restart")
	}
}

func TestKaleidoscopeAutoFillPaintsSymmetricDesignAtOnce(t *testing.T) {
	hub := newDemoHub(t.TempDir())
	handler := newDemoHandlerWithHub(hub)
	response := demoRequest(t, handler, http.MethodPost, "/api/kaleidoscope/autofill?board=kaleidoscope", map[string]any{"generation": 0})
	if response.Code != http.StatusOK {
		t.Fatalf("auto fill = %d: %s", response.Code, response.Body.String())
	}
	board := hub.lookup("kaleidoscope")
	board.mu.Lock()
	defer board.mu.Unlock()
	if board.generation != 1 {
		t.Fatalf("generation = %d", board.generation)
	}
	painted := 0
	for y := 0; y < demoHeight; y += 11 {
		for x := 0; x < demoWidth; x += 13 {
			color := board.cells[y*demoWidth+x]
			if color > 0 {
				painted++
			}
			for _, point := range [][2]int{{demoWidth - 1 - x, y}, {x, demoHeight - 1 - y}, {y, x}} {
				if board.cells[point[1]*demoWidth+point[0]] != color {
					t.Fatalf("asymmetry at %d,%d", x, y)
				}
			}
		}
	}
	if painted == 0 {
		t.Fatal("auto fill left the pattern empty")
	}
}

func TestKaleidoscopeRecolorKeepsShapeAndRandomizesSymmetricTarget(t *testing.T) {
	dir := t.TempDir()
	hub := newDemoHub(dir)
	handler := newDemoHandlerWithHub(hub)
	board := hub.lookup("kaleidoscope")
	var prior []int8
	var scheme []int8
	for generation := uint64(0); generation < 2; generation++ {
		response := demoRequest(t, handler, http.MethodPost, "/api/kaleidoscope/recolor?board=kaleidoscope", map[string]any{"generation": generation})
		if response.Code != http.StatusOK {
			t.Fatalf("recolor = %d: %s", response.Code, response.Body.String())
		}
		board.mu.Lock()
		cells := append([]int8(nil), board.cells...)
		target := append([]int8(nil), board.target...)
		currentScheme := append([]int8(nil), board.paletteScheme...)
		board.mu.Unlock()
		if len(target) != demoCells {
			t.Fatalf("target length = %d", len(target))
		}
		for y := 0; y < demoHeight; y++ {
			for x := 0; x < demoWidth; x++ {
				i := y*demoWidth + x
				if cells[i] > 0 {
					t.Fatal("recolor must clear painted cells")
				}
				if target[i] < 1 || target[i] > demoPaletteSize {
					t.Fatalf("invalid target %d", target[i])
				}
				for _, point := range [][2]int{{demoWidth - 1 - x, y}, {x, demoHeight - 1 - y}, {y, x}} {
					if target[point[1]*demoWidth+point[0]] != target[i] {
						t.Fatalf("asymmetric target at %d,%d", x, y)
					}
				}
			}
		}
		if generation > 0 && equalCells(currentScheme, scheme) {
			t.Fatal("palette scheme repeated")
		}
		if generation > 0 && !sameKaleidoscopeMask(cells, prior) {
			t.Fatal("recolor changed shape")
		}
		prior, scheme = cells, currentScheme
	}
	if response := demoRequest(t, handler, http.MethodPost, "/api/kaleidoscope/recolor?board=commons", map[string]any{"generation": 0}); response.Code != http.StatusForbidden {
		t.Fatalf("commons recolor = %d", response.Code)
	}
	if response := demoRequest(t, handler, http.MethodPost, "/api/kaleidoscope/recolor?board=kaleidoscope", map[string]any{"generation": 0}); response.Code != http.StatusConflict {
		t.Fatalf("stale recolor = %d", response.Code)
	}
	loaded := newDemoHub(dir)
	if err := loaded.loadBoards(); err != nil {
		t.Fatal(err)
	}
	if got := loaded.lookup("kaleidoscope"); got == nil || !equalCells(got.paletteScheme, scheme) || len(got.target) != demoCells {
		t.Fatal("recolor did not persist")
	}
}

func equalCells(a, b []int8) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

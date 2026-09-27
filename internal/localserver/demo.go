package localserver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"log"
	"math"
	"net/http"
	"sync"
	"time"

	"canvas/pages"
)

const demoWidth = 200
const demoHeight = 200
const demoCells = demoWidth * demoHeight
const demoPaletteSize = 16
const maxBoardSide = 512

type demoState struct {
	mu            sync.Mutex
	id            string
	name          string
	date          string
	prompt        string
	createdAt     time.Time
	paletteScheme []int8
	now           func() time.Time
	width         int
	height        int
	cells         []int8
	target        []int8
	generation    uint64
	dir           string
	subscribers   map[chan []byte]struct{}
}

type pixelEvent struct {
	PixelID    *int    `json:"pixelId"`
	Color      *int    `json:"color"`
	Generation *uint64 `json:"generation,omitempty"`
}

func validBoardDimensions(width, height int) bool {
	return width > 0 && height > 0 && width <= maxBoardSide && height <= maxBoardSide && width*height <= demoCells
}

func validPixelEvent(event pixelEvent, cells int) bool {
	return event.PixelID != nil && event.Color != nil && *event.PixelID >= 0 && *event.PixelID < cells && *event.Color >= 1 && *event.Color <= demoPaletteSize
}

func newDemoHandler() http.Handler {
	return newDemoHandlerWithHub(newDemoHub(""))
}

func newDemoHandlerWithHub(hub *demoHub) http.Handler {
	sample, target, samplePNG := makeSampleKaleidoscope()
	hub.addBuiltinBoards(sample, target)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, "/map", http.StatusFound)
	})
	mux.HandleFunc("GET /map", func(w http.ResponseWriter, r *http.Request) {
		serveDemoPage(w, "map.html")
	})
	mux.HandleFunc("GET /board/{id}", func(w http.ResponseWriter, r *http.Request) {
		if hub.lookup(r.PathValue("id")) == nil {
			http.NotFound(w, r)
			return
		}
		serveDemoPage(w, "map.html")
	})
	mux.HandleFunc("GET /board/daily", func(w http.ResponseWriter, r *http.Request) {
		if _, _, err := hub.today(); err != nil {
			http.Error(w, "Daily board unavailable", http.StatusInternalServerError)
			return
		}
		serveDemoPage(w, "map.html")
	})
	mux.HandleFunc("GET /app.css", func(w http.ResponseWriter, r *http.Request) {
		styles, err := pages.Files.ReadFile("app.css")
		if err != nil {
			http.Error(w, "Stylesheet unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		w.Write(styles)
	})
	mux.HandleFunc("GET /theme.js", func(w http.ResponseWriter, r *http.Request) {
		script, err := pages.Files.ReadFile("theme.js")
		if err != nil {
			http.Error(w, "Theme script unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Write(script)
	})
	mux.HandleFunc("GET /app.js", func(w http.ResponseWriter, r *http.Request) {
		script, err := pages.Files.ReadFile("app.js")
		if err != nil {
			http.Error(w, "Script unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Write(script)
	})
	mux.HandleFunc("GET /board-tools.mjs", func(w http.ResponseWriter, r *http.Request) {
		script, err := pages.Files.ReadFile("board-tools.mjs")
		if err != nil {
			http.Error(w, "Tool script unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Write(script)
	})
	mux.HandleFunc("GET /auto-fill.mjs", func(w http.ResponseWriter, r *http.Request) {
		script, err := pages.Files.ReadFile("auto-fill.mjs")
		if err != nil {
			http.Error(w, "Script unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Write(script)
	})
	mux.HandleFunc("GET /live-connection.mjs", func(w http.ResponseWriter, r *http.Request) {
		script, err := pages.Files.ReadFile("live-connection.mjs")
		if err != nil {
			http.Error(w, "Script unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Write(script)
	})
	for _, file := range []string{"live-fill.mjs", "pixel-reveal.mjs"} {
		mux.HandleFunc("GET /"+file, func(w http.ResponseWriter, r *http.Request) {
			script, err := pages.Files.ReadFile(file)
			if err != nil {
				http.Error(w, "Script unavailable", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
			w.Write(script)
		})
	}
	mux.HandleFunc("GET /sample.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(samplePNG)
	})
	mux.HandleFunc("GET /api/target", func(w http.ResponseWriter, r *http.Request) {
		board := hub.fromRequest(w, r)
		if board == nil {
			return
		}
		board.mu.Lock()
		colors := append([]int8(nil), board.target...)
		board.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"colors": colors})
	})
	mux.HandleFunc("GET /api/boards", hub.listBoards)
	mux.HandleFunc("GET /api/daily/today", hub.getToday)
	mux.HandleFunc("GET /api/daily/archive", hub.getDailyArchive)
	mux.HandleFunc("POST /api/boards", hub.createBoard)
	mux.HandleFunc("GET /api/map", hub.withBoard((*demoState).getMap))
	mux.HandleFunc("PUT /api/map", hub.withBoard((*demoState).putMap))
	mux.HandleFunc("POST /api/clear", hub.withBoard((*demoState).clearBoard))
	mux.HandleFunc("PUT /api/artwork", hub.withBoard((*demoState).putArtwork))
	mux.HandleFunc("POST /api/kaleidoscope/reroll", hub.withBoard((*demoState).rerollKaleidoscope))
	mux.HandleFunc("POST /api/kaleidoscope/recolor", hub.withBoard((*demoState).recolorKaleidoscope))
	mux.HandleFunc("POST /api/kaleidoscope/autofill", hub.withBoard((*demoState).autoFillKaleidoscope))
	mux.HandleFunc("POST /api/events", hub.withBoard((*demoState).postEvent))
	mux.HandleFunc("POST /api/events/batch", hub.withBoard((*demoState).postEventBatch))
	mux.HandleFunc("PUT /api/photo", hub.withBoard((*demoState).putPhoto))
	mux.HandleFunc("POST /api/sample-reset", func(w http.ResponseWriter, r *http.Request) {
		board := hub.fromRequest(w, r)
		if board == nil {
			return
		}
		if board.id != "kaleidoscope" {
			http.Error(w, "Only the kaleidoscope can be reset to the sample", http.StatusBadRequest)
			return
		}
		board.mu.Lock()
		generation := board.generation + 1
		if err := board.saveDimensionsLocked(demoWidth, demoHeight, sample, target, generation); err != nil {
			board.mu.Unlock()
			http.Error(w, "Could not reset sample", http.StatusInternalServerError)
			return
		}
		board.cells = append([]int8(nil), sample...)
		board.target = append([]int8(nil), target...)
		board.width, board.height = demoWidth, demoHeight
		board.generation = generation
		data, _ := json.Marshal(map[string]any{"width": board.width, "height": board.height, "cells": board.cells, "target": board.target, "generation": generation})
		board.broadcast(data)
		board.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"generation": generation})
	})
	mux.HandleFunc("GET /api/stream", hub.withBoard((*demoState).stream))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if needsDailyMaintenance(r) {
			if err := hub.pruneDaily(); err != nil {
				log.Printf("daily request cleanup: %v", err)
			}
		}
		mux.ServeHTTP(w, r)
	})
}

func makeSampleKaleidoscope() ([]int8, []int8, []byte) {
	palette := [...]color.NRGBA{
		{}, {0x00, 0x5d, 0xa0, 0xff}, {0x3a, 0x22, 0x5d, 0xff},
		{0x00, 0x4c, 0x93, 0xff}, {0xb5, 0x07, 0x6b, 0xff}, {0xff, 0x82, 0x2a, 0xff},
		{0xfd, 0xb9, 0x13, 0xff}, {0xeb, 0x00, 0x8b, 0xff}, {0xdc, 0x00, 0x00, 0xff},
		{0x00, 0xae, 0xef, 0xff}, {0xa2, 0x88, 0xe3, 0xff},
	}
	mask := make([]int8, demoCells)
	target := make([]int8, demoCells)
	image := image.NewNRGBA(image.Rect(0, 0, demoWidth, demoHeight))
	for y := 0; y < demoHeight; y++ {
		for x := 0; x < demoWidth; x++ {
			dx, dy := float64(x)-99.5, float64(y)-99.5
			radius := math.Hypot(dx, dy)
			angle := math.Atan2(dy, dx)
			spoke := math.Abs(math.Sin(6 * angle))
			betweenSpokes := math.Abs(math.Cos(6 * angle))
			center := radius <= 13+5*math.Cos(12*angle)
			petal := math.Pow((radius-42)/23, 2)+math.Pow(spoke/0.78, 2) < 1
			diamond := math.Pow((radius-74)/16, 2)+math.Pow(betweenSpokes/0.78, 2) < 1
			crown := radius > 90 && radius < 94 && spoke < 0.24
			index := y*demoWidth + x
			if !(center || petal || diamond || crown) {
				mask[index] = -1
			}
			sector := int(math.Floor(math.Mod(angle+2*math.Pi, 2*math.Pi) / (math.Pi / 6)))
			band := int(radius / 22)
			chosen := int8(1 + (sector%3*3+band*2)%10)
			target[index] = chosen
			if mask[index] == 0 {
				image.SetNRGBA(x, y, palette[chosen])
			}
		}
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, image); err != nil {
		panic(err)
	}
	return mask, target, buffer.Bytes()
}

func serveDemoPage(w http.ResponseWriter, name string) {
	page, err := pages.Files.ReadFile(name)
	if err != nil {
		http.Error(w, "Page unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(page)
}

func (s *demoState) getMap(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	result := map[string]any{"width": s.width, "height": s.height, "cells": append([]int8(nil), s.cells...), "generation": s.generation}
	if isDailyBoardID(s.id) {
		result["date"] = s.date
		result["prompt"] = s.prompt
	}
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func decodeDemoJSON(w http.ResponseWriter, r *http.Request, value any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return false
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		http.Error(w, "Expected one JSON object", http.StatusBadRequest)
		return false
	}
	return true
}

func (s *demoState) broadcast(data []byte) {
	for channel := range s.subscribers {
		select {
		case channel <- data:
		default:
			close(channel)
			delete(s.subscribers, channel)
		}
	}
}

func (s *demoState) replaceArtworkLocked(next []int8) (uint64, error) {
	generation := s.generation + 1
	if err := s.saveDimensionsLocked(s.width, s.height, next, nil, generation); err != nil {
		return 0, err
	}
	s.cells = append([]int8(nil), next...)
	s.target = nil
	s.generation = generation
	data, _ := json.Marshal(map[string]any{"width": s.width, "height": s.height, "cells": s.cells, "target": nil, "generation": generation})
	s.broadcast(data)
	return generation, nil
}

func (s *demoState) replacementAllowed(w http.ResponseWriter, r *http.Request, expected *uint64) bool {
	if s.expiredCustom() {
		http.NotFound(w, r)
		return false
	}
	if !s.dailyWritableLocked() {
		http.Error(w, "This daily board is archived", http.StatusForbidden)
		return false
	}
	if expected == nil {
		http.Error(w, "Generation is required", http.StatusBadRequest)
		return false
	}
	if *expected != s.generation {
		http.Error(w, "Board was replaced; refresh before continuing", http.StatusConflict)
		return false
	}
	return true
}

func (s *demoState) clearBoard(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Generation *uint64 `json:"generation"`
	}
	if !decodeDemoJSON(w, r, &request) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.replacementAllowed(w, r, request.Generation) {
		return
	}
	next := make([]int8, len(s.cells))
	for i, cell := range s.cells {
		if cell == -1 {
			next[i] = -1
		}
	}
	generation, err := s.replaceArtworkLocked(next)
	if err != nil {
		http.Error(w, "Could not clear board", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"generation": generation})
}

func (s *demoState) putArtwork(w http.ResponseWriter, r *http.Request) {
	if s.id == "kaleidoscope" {
		http.Error(w, "Use kaleidoscope reroll", http.StatusForbidden)
		return
	}
	var request struct {
		Generation *uint64 `json:"generation"`
		Cells      []int8  `json:"cells"`
	}
	if !decodeDemoJSON(w, r, &request) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.replacementAllowed(w, r, request.Generation) {
		return
	}
	if len(request.Cells) != len(s.cells) {
		http.Error(w, "Artwork size must match the board", http.StatusBadRequest)
		return
	}
	for i, cell := range request.Cells {
		if cell < -1 || cell > demoPaletteSize || (s.cells[i] == -1 && cell != -1) || (s.cells[i] != -1 && cell == -1) {
			http.Error(w, "Artwork contains an invalid or blocked pixel", http.StatusBadRequest)
			return
		}
	}
	generation, err := s.replaceArtworkLocked(request.Cells)
	if err != nil {
		http.Error(w, "Could not save artwork", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"generation": generation})
}

func (s *demoState) putMap(w http.ResponseWriter, r *http.Request) {
	if themedBoardID(s.id) {
		http.Error(w, "This board's shape cannot be replaced", http.StatusForbidden)
		return
	}
	var request struct {
		Cells []int8 `json:"cells"`
	}
	if !decodeDemoJSON(w, r, &request) {
		return
	}
	for _, cell := range request.Cells {
		if cell != -1 && cell != 0 {
			http.Error(w, "Map cells must be -1 or 0", http.StatusBadRequest)
			return
		}
	}
	s.mu.Lock()
	if s.expiredCustom() {
		s.mu.Unlock()
		http.NotFound(w, r)
		return
	}
	if len(request.Cells) != s.width*s.height {
		expected := s.width * s.height
		s.mu.Unlock()
		http.Error(w, fmt.Sprintf("Map must contain %d cells", expected), http.StatusBadRequest)
		return
	}
	generation := s.generation + 1
	data, _ := json.Marshal(map[string]any{"width": s.width, "height": s.height, "cells": request.Cells, "target": nil, "generation": generation})
	if err := s.saveDimensionsLocked(s.width, s.height, request.Cells, nil, generation); err != nil {
		s.mu.Unlock()
		http.Error(w, "Could not save map", http.StatusInternalServerError)
		return
	}
	s.cells = append([]int8(nil), request.Cells...)
	s.target = nil
	s.generation = generation
	s.broadcast(data)
	s.mu.Unlock()
	w.WriteHeader(http.StatusOK)
}

func (s *demoState) postEvent(w http.ResponseWriter, r *http.Request) {
	var event pixelEvent
	if !decodeDemoJSON(w, r, &event) {
		return
	}
	s.mu.Lock()
	if s.expiredCustom() {
		s.mu.Unlock()
		http.NotFound(w, r)
		return
	}
	if !s.dailyWritableLocked() {
		s.mu.Unlock()
		http.Error(w, "This daily board is archived", http.StatusForbidden)
		return
	}
	if event.Generation != nil && *event.Generation != s.generation {
		s.mu.Unlock()
		http.Error(w, "Board was replaced; refresh before continuing", http.StatusConflict)
		return
	}
	if !validPixelEvent(event, len(s.cells)) {
		s.mu.Unlock()
		http.Error(w, "Pixel is outside the board or color is not 1–16", http.StatusBadRequest)
		return
	}
	if s.cells[*event.PixelID] == -1 {
		s.mu.Unlock()
		http.Error(w, "This pixel is outside the map", http.StatusBadRequest)
		return
	}
	next := append([]int8(nil), s.cells...)
	next[*event.PixelID] = int8(*event.Color)
	if err := s.saveLocked(next, s.target); err != nil {
		s.mu.Unlock()
		http.Error(w, "Could not save pixel", http.StatusInternalServerError)
		return
	}
	s.cells = next
	data, _ := json.Marshal(event)
	s.broadcast(data)
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func (s *demoState) postEventBatch(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Events     []pixelEvent `json:"events"`
		Generation *uint64      `json:"generation,omitempty"`
		OnlyBlank  bool         `json:"onlyBlank,omitempty"`
		Live       bool         `json:"live,omitempty"`
	}
	if !decodeDemoJSON(w, r, &request) {
		return
	}
	if len(request.Events) == 0 || len(request.Events) > 64 {
		http.Error(w, "Batch must contain 1–64 events", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	if s.expiredCustom() {
		s.mu.Unlock()
		http.NotFound(w, r)
		return
	}
	if !s.dailyWritableLocked() {
		s.mu.Unlock()
		http.Error(w, "This daily board is archived", http.StatusForbidden)
		return
	}
	if request.Generation != nil && *request.Generation != s.generation {
		s.mu.Unlock()
		http.Error(w, "Board was replaced; refresh before continuing", http.StatusConflict)
		return
	}
	for _, event := range request.Events {
		if !validPixelEvent(event, len(s.cells)) || (!request.OnlyBlank && s.cells[*event.PixelID] == -1) {
			s.mu.Unlock()
			http.Error(w, "Batch contains an invalid or blocked pixel", http.StatusBadRequest)
			return
		}
	}
	next := append([]int8(nil), s.cells...)
	applied := make([]pixelEvent, 0, len(request.Events))
	for _, event := range request.Events {
		if request.OnlyBlank && next[*event.PixelID] != 0 {
			continue
		}
		next[*event.PixelID] = int8(*event.Color)
		applied = append(applied, event)
	}
	if len(applied) == 0 {
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"events":[]}`))
		return
	}
	if err := s.saveLocked(next, s.target); err != nil {
		s.mu.Unlock()
		http.Error(w, "Could not save pixels", http.StatusInternalServerError)
		return
	}
	s.cells = next
	request.Events = applied
	data, _ := json.Marshal(request)
	s.broadcast(data)
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func (s *demoState) putPhoto(w http.ResponseWriter, r *http.Request) {
	if s.id == "commons" || s.id == "kaleidoscope" || themedBoardID(s.id) {
		http.Error(w, "Photos cannot replace this board", http.StatusForbidden)
		return
	}
	var request struct {
		Width  int    `json:"width"`
		Height int    `json:"height"`
		Colors []int8 `json:"colors"`
	}
	if !decodeDemoJSON(w, r, &request) {
		return
	}
	for _, color := range request.Colors {
		if color < 1 || color > demoPaletteSize {
			http.Error(w, "Photo colors must be 1–16", http.StatusBadRequest)
			return
		}
	}
	s.mu.Lock()
	if s.expiredCustom() {
		s.mu.Unlock()
		http.NotFound(w, r)
		return
	}
	if request.Width == 0 && request.Height == 0 {
		request.Width, request.Height = s.width, s.height
	}
	if !validBoardDimensions(request.Width, request.Height) || len(request.Colors) != request.Width*request.Height {
		s.mu.Unlock()
		http.Error(w, "Photo dimensions must be at most 512 × 512 and 40000 pixels", http.StatusBadRequest)
		return
	}
	blank := make([]int8, len(request.Colors))
	generation := s.generation + 1
	if err := s.saveDimensionsLocked(request.Width, request.Height, blank, request.Colors, generation); err != nil {
		s.mu.Unlock()
		http.Error(w, "Could not save photo", http.StatusInternalServerError)
		return
	}
	s.cells = blank
	s.target = append([]int8(nil), request.Colors...)
	s.width, s.height = request.Width, request.Height
	s.generation = generation
	data, _ := json.Marshal(map[string]any{"width": s.width, "height": s.height, "cells": blank, "target": request.Colors, "generation": generation})
	s.broadcast(data)
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"generation": generation})
}

func (s *demoState) stream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	channel := make(chan []byte, 32)
	s.mu.Lock()
	snapshot, _ := json.Marshal(map[string]any{"width": s.width, "height": s.height, "cells": s.cells, "target": s.target, "generation": s.generation})
	s.subscribers[channel] = struct{}{}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		if _, exists := s.subscribers[channel]; exists {
			delete(s.subscribers, channel)
			close(channel)
		}
		s.mu.Unlock()
	}()
	if _, err := fmt.Fprintf(w, "data: %s\n\n", snapshot); err != nil {
		return
	}
	flusher.Flush()
	for {
		select {
		case data, open := <-channel:
			if !open {
				return
			}
			if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
				return
			}
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

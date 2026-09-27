package localserver

import (
	crand "crypto/rand"
	"encoding/binary"
	"encoding/json"
	"math"
	"math/rand"
	"net/http"
)

func randomKaleidoscope(source *rand.Rand, previous []int8) ([]int8, []int8) {
	var scheme []int8
	for {
		permutation := source.Perm(demoPaletteSize)
		scheme = []int8{int8(permutation[0] + 1), int8(permutation[1] + 1), int8(permutation[2] + 1), int8(permutation[3] + 1)}
		if !samePalette(scheme, previous) {
			break
		}
	}
	inner := 12 + source.Intn(13)
	ring := 42 + source.Intn(22)
	ringWidth := 9 + source.Intn(11)
	outer := 76 + source.Intn(16)
	spokes := 3 + source.Intn(5)
	phase := source.Float64() * math.Pi
	pattern := make([]int8, demoCells)
	for y := 0; y < demoHeight; y++ {
		for x := 0; x < demoWidth; x++ {
			dx := math.Abs(float64(x) - 99.5)
			dy := math.Abs(float64(y) - 99.5)
			large, small := math.Max(dx, dy), math.Min(dx, dy)
			radius := math.Hypot(large, small)
			angle := math.Atan2(small, large)
			wave := math.Abs(math.Sin(float64(spokes)*angle + phase))
			inside := radius < float64(inner) || (math.Abs(radius-float64(ring)) < float64(ringWidth)*(0.4+wave)) || (math.Abs(radius-float64(outer)) < 4+4*wave)
			index := y*demoWidth + x
			if !inside {
				pattern[index] = -1
				continue
			}
			band := int(radius / float64(19+spokes))
			wedge := int(angle * float64(5+spokes) / math.Pi)
			pattern[index] = scheme[(band+wedge)%len(scheme)]
		}
	}
	return pattern, scheme
}

func samePalette(a, b []int8) bool {
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

func sameKaleidoscopeMask(a, b []int8) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if (a[i] == -1) != (b[i] == -1) {
			return false
		}
	}
	return true
}

func (s *demoState) rerollKaleidoscope(w http.ResponseWriter, r *http.Request) {
	if s.id != "kaleidoscope" {
		http.Error(w, "Only Kaleidoscope can reroll", http.StatusForbidden)
		return
	}
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
	var seed [8]byte
	if _, err := crand.Read(seed[:]); err != nil {
		http.Error(w, "Could not generate design", http.StatusInternalServerError)
		return
	}
	source := rand.New(rand.NewSource(int64(binary.LittleEndian.Uint64(seed[:]))))
	var pattern []int8
	for {
		pattern, _ = randomKaleidoscope(source, s.paletteScheme)
		if !sameKaleidoscopeMask(pattern, s.cells) {
			break
		}
	}
	blank := make([]int8, len(pattern))
	for i, color := range pattern {
		if color == -1 {
			blank[i] = -1
		}
	}
	generation, err := s.replaceArtworkLocked(blank)
	if err != nil {
		http.Error(w, "Could not save design", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"generation": generation})
}

func (s *demoState) recolorKaleidoscope(w http.ResponseWriter, r *http.Request) {
	if s.id != "kaleidoscope" {
		http.Error(w, "Only Kaleidoscope can be recolored", http.StatusForbidden)
		return
	}
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
	var seed [8]byte
	if _, err := crand.Read(seed[:]); err != nil {
		http.Error(w, "Could not generate colors", http.StatusInternalServerError)
		return
	}
	source := rand.New(rand.NewSource(int64(binary.LittleEndian.Uint64(seed[:]))))
	var scheme []int8
	for {
		permutation := source.Perm(demoPaletteSize)
		scheme = []int8{int8(permutation[0] + 1), int8(permutation[1] + 1), int8(permutation[2] + 1), int8(permutation[3] + 1)}
		if !samePalette(scheme, s.paletteScheme) {
			break
		}
	}
	bandSize := 12 + source.Intn(21)
	wedgeCount := 4 + source.Intn(11)
	phase := source.Intn(4)
	target := make([]int8, len(s.cells))
	blank := make([]int8, len(s.cells))
	for y := 0; y < s.height; y++ {
		for x := 0; x < s.width; x++ {
			id := y*s.width + x
			dx := math.Abs(float64(x) - float64(s.width-1)/2)
			dy := math.Abs(float64(y) - float64(s.height-1)/2)
			large, small := math.Max(dx, dy), math.Min(dx, dy)
			band := int(math.Hypot(large, small)) / bandSize
			wedge := int(math.Atan2(small, large) * float64(wedgeCount) / math.Pi)
			target[id] = scheme[(band+wedge+phase)%len(scheme)]
			if s.cells[id] == -1 {
				blank[id] = -1
			}
		}
	}
	prior := s.paletteScheme
	s.paletteScheme = scheme
	generation := s.generation + 1
	if err := s.saveDimensionsLocked(s.width, s.height, blank, target, generation); err != nil {
		s.paletteScheme = prior
		http.Error(w, "Could not save colors", http.StatusInternalServerError)
		return
	}
	s.cells, s.target, s.generation = blank, target, generation
	data, _ := json.Marshal(map[string]any{"width": s.width, "height": s.height, "cells": blank, "target": target, "generation": generation})
	s.broadcast(data)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"generation": generation})
}

func (s *demoState) autoFillKaleidoscope(w http.ResponseWriter, r *http.Request) {
	if s.id != "kaleidoscope" {
		http.Error(w, "Only Kaleidoscope can be auto filled", http.StatusForbidden)
		return
	}
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
	var seed [8]byte
	if _, err := crand.Read(seed[:]); err != nil {
		http.Error(w, "Could not generate design", http.StatusInternalServerError)
		return
	}
	source := rand.New(rand.NewSource(int64(binary.LittleEndian.Uint64(seed[:]))))
	var pattern, scheme []int8
	for {
		pattern, scheme = randomKaleidoscope(source, s.paletteScheme)
		if !sameKaleidoscopeMask(pattern, s.cells) {
			break
		}
	}
	target := append([]int8(nil), pattern...)
	for i, color := range target {
		if color < 0 {
			target[i] = 1
		}
	}
	generation := s.generation + 1
	prior := s.paletteScheme
	s.paletteScheme = scheme
	if err := s.saveDimensionsLocked(s.width, s.height, pattern, target, generation); err != nil {
		s.paletteScheme = prior
		http.Error(w, "Could not save design", http.StatusInternalServerError)
		return
	}
	s.cells, s.target, s.generation = pattern, target, generation
	data, _ := json.Marshal(map[string]any{"width": s.width, "height": s.height, "cells": pattern, "target": target, "generation": generation})
	s.broadcast(data)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"generation": generation})
}

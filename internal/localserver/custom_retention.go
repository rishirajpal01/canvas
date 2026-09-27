package localserver

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"time"
)

// pruneCustom removes expired custom boards and closes their live streams.
// A failed file removal stays in memory so the next pass can retry it.
func (h *demoHub) pruneCustom() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	var firstErr error
	for id, board := range h.boards {
		if !board.expiredCustom() {
			continue
		}
		board.mu.Lock()
		for channel := range board.subscribers {
			close(channel)
			delete(board.subscribers, channel)
		}
		if h.dir != "" {
			if err := os.Remove(filepath.Join(h.dir, id+".json")); err != nil && !os.IsNotExist(err) {
				if firstErr == nil {
					firstErr = err
				}
				board.mu.Unlock()
				continue
			}
		}
		board.mu.Unlock()
		delete(h.boards, id)
	}
	return firstErr
}

func (h *demoHub) runCustomMaintenance(ctx context.Context) {
	for {
		if err := h.pruneCustom(); err != nil {
			log.Printf("custom board cleanup: %v", err)
		}
		timer := time.NewTimer(h.nextCustomSweepDelay())
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (h *demoHub) nextCustomSweepDelay() time.Duration {
	delay := time.Minute
	now := h.now()
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, board := range h.boards {
		if !customBoardID(board.id) {
			continue
		}
		until := board.createdAt.Add(7 * 24 * time.Hour).Sub(now)
		if until < delay {
			delay = until
		}
	}
	if delay < time.Second {
		return time.Second
	}
	return delay
}

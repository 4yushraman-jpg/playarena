// Package realtime implements the in-process pub/sub hub for Server-Sent Events.
//
// Architecture:
//
//	Each SSE client calls Subscribe to receive a buffered channel.
//	Domain services call Publish after their DB transaction commits.
//	The SSE handler reads from the channel and writes text/event-stream frames.
//
// Delivery guarantee: at-most-once for SSE.
// If a subscriber's buffer is full, the event is dropped for that subscriber.
// The DB record already exists, so the client can recover missed events via
// the REST GET /notifications endpoint.
package realtime

import (
	"encoding/json"
	"sync"

	"github.com/jackc/pgx/v5/pgtype"
)

// subKey identifies a single user's subscription within an org.
// Using [16]byte makes it a comparable map key without heap allocation.
type subKey struct {
	orgID  [16]byte
	userID [16]byte
}

// Hub manages SSE client connections and event fan-out.
// A single Hub instance is shared across all HTTP requests.
// All methods are safe for concurrent use.
type Hub struct {
	mu   sync.RWMutex
	subs map[subKey]map[chan []byte]struct{}
	done chan struct{}
}

// NewHub constructs an idle Hub ready to accept subscriptions.
func NewHub() *Hub {
	return &Hub{
		subs: make(map[subKey]map[chan []byte]struct{}),
		done: make(chan struct{}),
	}
}

// Subscribe registers a new SSE client channel for the given (orgID, userID).
// Returns the channel the SSE handler reads from.
// Buffer size 32: a slow client drops events rather than blocking the publisher.
func (h *Hub) Subscribe(orgID, userID pgtype.UUID) chan []byte {
	ch := make(chan []byte, 32)
	key := subKey{orgID: orgID.Bytes, userID: userID.Bytes}
	h.mu.Lock()
	if h.subs[key] == nil {
		h.subs[key] = make(map[chan []byte]struct{})
	}
	h.subs[key][ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

// Unsubscribe removes the channel from the registry and closes it.
// The SSE handler calls this in a defer when the client disconnects.
// Safe to call concurrently with Shutdown: if Shutdown already cleared the map,
// found will be false and the channel is not double-closed.
func (h *Hub) Unsubscribe(orgID, userID pgtype.UUID, ch chan []byte) {
	key := subKey{orgID: orgID.Bytes, userID: userID.Bytes}
	h.mu.Lock()
	found := false
	if subs, ok := h.subs[key]; ok {
		if _, exists := subs[ch]; exists {
			delete(subs, ch)
			if len(subs) == 0 {
				delete(h.subs, key)
			}
			found = true
		}
	}
	h.mu.Unlock()
	if found {
		close(ch)
	}
}

// Publish fans out a JSON-encoded event to all subscribers for (orgID, userID).
// Non-blocking: drops the event for any subscriber whose buffer is full.
// Only called after the DB transaction commits (publish-after-commit).
//
// The RLock is held for the entire fan-out (map iteration + channel sends).
// Holding it through the sends prevents Unsubscribe/Shutdown from closing a
// channel between when we read it from the map and when we attempt to send,
// which would otherwise cause a send-on-closed-channel panic.
func (h *Hub) Publish(orgID, userID pgtype.UUID, event any) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	key := subKey{orgID: orgID.Bytes, userID: userID.Bytes}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.subs[key] {
		select {
		case ch <- data:
		default:
			// Buffer full — drop. Client recovers via REST polling.
		}
	}
}

// Shutdown closes all active subscriber channels and marks the hub as stopped.
// Called once during App.Shutdown. All SSE handlers will exit when their channel
// is closed. Safe to call concurrently with Unsubscribe: channels removed from
// the map by Shutdown are never passed to close a second time.
func (h *Hub) Shutdown() {
	select {
	case <-h.done:
		return // already shut down
	default:
		close(h.done)
	}
	// Collect all live channels under the write lock, then close them outside
	// the lock so Unsubscribe can acquire the lock and see an empty map.
	h.mu.Lock()
	toClose := make([]chan []byte, 0)
	for _, subs := range h.subs {
		for ch := range subs {
			toClose = append(toClose, ch)
		}
	}
	h.subs = make(map[subKey]map[chan []byte]struct{})
	h.mu.Unlock()
	for _, ch := range toClose {
		close(ch)
	}
}

// Done returns the hub's shutdown signal channel.
// SSE handlers select on this to detect server shutdown independently of
// client disconnection.
func (h *Hub) Done() <-chan struct{} {
	return h.done
}

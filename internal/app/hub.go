package app

import (
	"sync"
	"time"
)

// Topic identifies a realtime stream pushed to clients over the websocket.
type Topic string

const (
	// TopicSandboxes carries the sandbox summary list.
	TopicSandboxes Topic = "sandboxes"
	// TopicProfiles carries profile views with their rules and assignments.
	TopicProfiles Topic = "profiles"
	// TopicSecrets carries the stored secret inventory.
	TopicSecrets Topic = "secrets"
	// TopicTraffic carries the global proxy allow/deny log.
	TopicTraffic Topic = "traffic"
	// TopicPolicyRules carries every daemon policy rule.
	TopicPolicyRules Topic = "policy-rules"
	// TopicBlocked carries a single newly blocked host.
	TopicBlocked Topic = "blocked"
	// TopicCaches carries the shared cache configuration list.
	TopicCaches Topic = "caches"
	// TopicSkills carries the registered skill stores.
	TopicSkills Topic = "skills"
)

// TopicSandbox is the per-sandbox detail stream.
func TopicSandbox(name string) Topic { return Topic("sandbox:" + name) }

// TopicJob is the output stream of one background job.
func TopicJob(id string) Topic { return Topic("jobs:" + id) }

// BlockedEvent describes one host the egress proxy denied.
type BlockedEvent struct {
	Host       string `json:"host"`
	Sandbox    string `json:"sandbox"`
	ProxyType  string `json:"proxy_type"`
	Rule       string `json:"rule"`
	CountSince int    `json:"count_since"`
	At         string `json:"at"`
}

// Event is the realtime envelope pushed over the websocket.
type Event struct {
	Topic Topic  `json:"topic"`
	Data  any    `json:"data,omitempty"`
	TS    string `json:"ts"`
}

// EventFor stamps an event with the current time.
func EventFor(topic Topic, data any) Event {
	return Event{Topic: topic, Data: data, TS: time.Now().UTC().Format(time.RFC3339Nano)}
}

// Hub broadcasts events to websocket subscribers. A subscriber whose buffer is
// full is dropped (its channel is closed) so it can reconnect and resync;
// publishers never block.
type Hub struct {
	mu   sync.Mutex
	next int
	subs map[int]chan Event
}

// NewHub builds an empty hub.
func NewHub() *Hub {
	return &Hub{subs: make(map[int]chan Event)}
}

// Subscribe registers a subscriber and returns its channel plus an unsubscribe
// function that must be called to release it.
func (h *Hub) Subscribe() (<-chan Event, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	id := h.next
	h.next++
	ch := make(chan Event, 64)
	h.subs[id] = ch
	return ch, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if c, ok := h.subs[id]; ok {
			delete(h.subs, id)
			close(c)
		}
	}
}

// Publish sends an event to all subscribers. A subscriber with a full buffer
// is dropped rather than blocking the publisher.
func (h *Hub) Publish(ev Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for id, ch := range h.subs {
		select {
		case ch <- ev:
		default:
			delete(h.subs, id)
			close(ch)
		}
	}
}

// TopicKits is published when a kit repository or its catalog changes.
const TopicKits Topic = "kits"

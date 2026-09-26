package chat

import (
	"context"
	"sync"

	"github.com/google/uuid"
)

const (
	EventMessage  = "message"
	EventPresence = "presence"
	EventKick     = "kick"
	EventClose    = "close"
)

// Event is what travels between server instances. Frame is the exact JSON
// sent to clients, so a Redis-backed Broker only has to ship bytes.
type Event struct {
	Kind     string    `json:"kind"`
	RoomID   uuid.UUID `json:"room_id"`
	MemberID uuid.UUID `json:"member_id"` // target of a kick
	Frame    []byte    `json:"frame"`
}

// Broker fans events out to every server instance. MemoryBroker serves a
// single instance; a Redis Pub/Sub implementation can replace it in Phase 3.
type Broker interface {
	Publish(ctx context.Context, ev Event) error
	Subscribe(fn func(Event))
}

type MemoryBroker struct {
	mu   sync.RWMutex
	subs []func(Event)
}

func NewMemoryBroker() *MemoryBroker { return &MemoryBroker{} }

func (b *MemoryBroker) Publish(_ context.Context, ev Event) error {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, fn := range b.subs {
		fn(ev)
	}
	return nil
}

func (b *MemoryBroker) Subscribe(fn func(Event)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subs = append(b.subs, fn)
}

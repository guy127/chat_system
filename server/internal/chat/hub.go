package chat

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/coder/websocket"
	"github.com/google/uuid"

	"qrchat/internal/apperr"
)

// Close codes in the 4000 range tell the client not to reconnect.
const (
	CloseUnauthorized websocket.StatusCode = 4001
	CloseRemoved      websocket.StatusCode = 4003
	CloseRoomClosed   websocket.StatusCode = 4004
)

// Client is one WebSocket connection. A member may hold several (one per tab).
type Client struct {
	RoomID      uuid.UUID
	MemberID    uuid.UUID
	DisplayName string
	out         chan outbound
	cancel      context.CancelFunc
}

type outbound struct {
	data        []byte
	closeCode   websocket.StatusCode // non-zero: close the connection after writing data
	closeReason string
}

func newClient(roomID, memberID uuid.UUID, name string, cancel context.CancelFunc) *Client {
	return &Client{RoomID: roomID, MemberID: memberID, DisplayName: name, out: make(chan outbound, 64), cancel: cancel}
}

// push queues a frame without blocking; a client that cannot keep up is dropped
// and will resume from its last message after reconnecting.
func (c *Client) push(o outbound) {
	select {
	case c.out <- o:
	default:
		c.cancel()
	}
}

// Hub tracks the connections held by this instance and delivers broker events to them.
type Hub struct {
	broker Broker
	mu     sync.Mutex
	rooms  map[uuid.UUID]map[*Client]struct{}
}

func NewHub(broker Broker) *Hub {
	h := &Hub{broker: broker, rooms: map[uuid.UUID]map[*Client]struct{}{}}
	broker.Subscribe(h.deliver)
	return h
}

func (h *Hub) Register(ctx context.Context, c *Client) {
	h.mu.Lock()
	clients, ok := h.rooms[c.RoomID]
	if !ok {
		clients = map[*Client]struct{}{}
		h.rooms[c.RoomID] = clients
	}
	first := !h.hasMemberLocked(c.RoomID, c.MemberID)
	clients[c] = struct{}{}
	h.mu.Unlock()
	if first {
		h.publishPresence(ctx, c, true)
	}
}

func (h *Hub) Unregister(ctx context.Context, c *Client) {
	h.mu.Lock()
	clients := h.rooms[c.RoomID]
	delete(clients, c)
	if len(clients) == 0 {
		delete(h.rooms, c.RoomID)
	}
	last := !h.hasMemberLocked(c.RoomID, c.MemberID)
	h.mu.Unlock()
	if last {
		h.publishPresence(ctx, c, false)
	}
}

func (h *Hub) hasMemberLocked(roomID, memberID uuid.UUID) bool {
	for c := range h.rooms[roomID] {
		if c.MemberID == memberID {
			return true
		}
	}
	return false
}

// Online implements room.Presence; presence is counted per member, not per tab.
func (h *Hub) Online(roomID uuid.UUID) map[uuid.UUID]bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := map[uuid.UUID]bool{}
	for c := range h.rooms[roomID] {
		out[c.MemberID] = true
	}
	return out
}

// Kick implements room.Notifier.
func (h *Hub) Kick(ctx context.Context, roomID, memberID uuid.UUID, reason string) error {
	return h.broker.Publish(ctx, Event{
		Kind: EventKick, RoomID: roomID, MemberID: memberID,
		Frame: mustJSON(map[string]string{"type": "kicked", "reason": reason}),
	})
}

// CloseRoom implements room.Notifier.
func (h *Hub) CloseRoom(ctx context.Context, roomID uuid.UUID) error {
	return h.broker.Publish(ctx, Event{
		Kind: EventClose, RoomID: roomID,
		Frame: mustJSON(map[string]string{"type": "room_closed", "reason": apperr.RoomClosed.Code}),
	})
}

func (h *Hub) publishPresence(ctx context.Context, c *Client, online bool) {
	frame := mustJSON(map[string]any{
		"type": "presence", "member_id": c.MemberID, "display_name": c.DisplayName, "online": online,
	})
	if err := h.broker.Publish(ctx, Event{Kind: EventPresence, RoomID: c.RoomID, Frame: frame}); err != nil {
		slog.ErrorContext(ctx, "publish presence", "room_id", c.RoomID, "member_id", c.MemberID, "err", err)
	}
}

func (h *Hub) deliver(ev Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.rooms[ev.RoomID] {
		switch ev.Kind {
		case EventMessage, EventPresence:
			c.push(outbound{data: ev.Frame})
		case EventKick:
			if c.MemberID == ev.MemberID {
				c.push(outbound{data: ev.Frame, closeCode: CloseRemoved, closeReason: "removed"})
			}
		case EventClose:
			c.push(outbound{data: ev.Frame, closeCode: CloseRoomClosed, closeReason: "room closed"})
		}
	}
}

// mustJSON is only called with maps of strings, UUIDs and bools, which cannot fail to marshal.
func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

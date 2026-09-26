package chat

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"qrchat/internal/apperr"
	"qrchat/internal/auth"
	"qrchat/internal/platform/httpx"
	"qrchat/internal/room"
)

const (
	subprotocol  = "chat"
	readLimit    = 16 << 10 // 2,000 Thai characters are ~6 KB of UTF-8 plus JSON
	pingTimeout  = 60 * time.Second
	writeTimeout = 10 * time.Second
)

type Handler struct {
	svc            *Service
	hub            *Hub
	rooms          *room.Service
	roomHandler    *room.Handler
	issuer         *auth.Issuer
	originPatterns []string
}

func NewHandler(svc *Service, hub *Hub, rooms *room.Service, roomHandler *room.Handler, issuer *auth.Issuer, originPatterns []string) *Handler {
	return &Handler{svc: svc, hub: hub, rooms: rooms, roomHandler: roomHandler, issuer: issuer, originPatterns: originPatterns}
}

func (h *Handler) Register(r gin.IRouter) {
	r.GET("/rooms/:id/messages", h.roomHandler.RequireMember(), h.history)
	r.GET("/ws", h.serveWS)
}

func (h *Handler) history(c *gin.Context) {
	limit := DefaultPageSize
	if s := c.Query("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > MaxPageSize {
			httpx.Error(c, apperr.Invalid("limit must be between 1 and 100"))
			return
		}
		limit = n
	}
	page, err := h.svc.History(c.Request.Context(), room.MemberFrom(c).RoomID, c.Query("before"), limit)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, page)
}

// serveWS upgrades /ws?room={id}. Browsers cannot set headers on a WebSocket,
// so the JWT travels as the second subprotocol: new WebSocket(url, ["chat", jwt]).
// That keeps it out of URLs and access logs. Authentication happens after the
// upgrade so the client receives an error frame and a 4xxx close code.
func (h *Handler) serveWS(c *gin.Context) {
	token := tokenFromProtocols(c.Request.Header.Values("Sec-WebSocket-Protocol"))
	conn, err := websocket.Accept(c.Writer, c.Request, &websocket.AcceptOptions{
		Subprotocols:   []string{subprotocol},
		OriginPatterns: h.originPatterns,
	})
	if err != nil {
		return // Accept has already written the HTTP error (e.g. bad Origin)
	}
	defer func() { _ = conn.CloseNow() }()
	conn.SetReadLimit(readLimit)
	ctx := c.Request.Context()

	roomID, err := uuid.Parse(c.Query("room"))
	if err != nil {
		closeWithError(ctx, conn, apperr.NotFound)
		return
	}
	claims, err := h.issuer.Parse(token)
	if err != nil || claims.RoomID != roomID {
		closeWithError(ctx, conn, apperr.Unauthorized)
		return
	}
	member, err := h.rooms.CanPost(ctx, roomID, claims.MemberID)
	if err != nil {
		closeWithError(ctx, conn, err)
		return
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	client := newClient(roomID, member.ID, member.DisplayName, cancel)
	h.hub.Register(ctx, client)
	defer h.hub.Unregister(context.WithoutCancel(ctx), client)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer cancel()
		writeLoop(ctx, conn, client)
	}()
	h.readLoop(ctx, conn, client)
	cancel()
	wg.Wait()
}

func tokenFromProtocols(headers []string) string {
	for _, h := range headers {
		for p := range strings.SplitSeq(h, ",") {
			if p = strings.TrimSpace(p); p != "" && p != subprotocol {
				return p
			}
		}
	}
	return ""
}

func writeLoop(ctx context.Context, conn *websocket.Conn, c *Client) {
	for {
		select {
		case <-ctx.Done():
			return
		case o := <-c.out:
			wctx, cancel := context.WithTimeout(ctx, writeTimeout)
			err := conn.Write(wctx, websocket.MessageText, o.data)
			cancel()
			if err != nil {
				return
			}
			if o.closeCode != 0 {
				_ = conn.Close(o.closeCode, o.closeReason)
				return
			}
		}
	}
}

type inbound struct {
	Type          string `json:"type"`
	ClientMsgID   string `json:"client_msg_id"`
	Body          string `json:"body"`
	LastMessageID string `json:"last_message_id"`
}

func (h *Handler) readLoop(ctx context.Context, conn *websocket.Conn, c *Client) {
	for {
		// Clients ping every 25 s; a silent connection is dropped after 60 s.
		rctx, cancel := context.WithTimeout(ctx, pingTimeout)
		typ, data, err := conn.Read(rctx)
		cancel()
		if err != nil {
			return
		}
		var in inbound
		if typ != websocket.MessageText || json.Unmarshal(data, &in) != nil {
			c.push(outbound{data: errorFrame(apperr.Invalid("frames must be JSON text"), "")})
			continue
		}
		switch in.Type {
		case "ping":
			c.push(outbound{data: []byte(`{"type":"pong"}`)})
		case "resume":
			h.resume(ctx, c, in.LastMessageID)
		case "send":
			h.send(ctx, c, in)
		default:
			c.push(outbound{data: errorFrame(apperr.Invalid("unknown frame type"), "")})
		}
	}
}

func (h *Handler) resume(ctx context.Context, c *Client, lastID string) {
	err := h.svc.Missed(ctx, c.RoomID, lastID, func(m Message) {
		c.push(outbound{data: messageFrame(m)})
	})
	if err != nil {
		h.pushError(ctx, c, err, "")
	}
}

func (h *Handler) send(ctx context.Context, c *Client, in inbound) {
	clientMsgID, err := uuid.Parse(in.ClientMsgID)
	if err != nil {
		c.push(outbound{data: errorFrame(apperr.Invalid("client_msg_id must be a UUID"), in.ClientMsgID)})
		return
	}
	msg, err := h.svc.Send(ctx, c.RoomID, c.MemberID, clientMsgID, in.Body)
	if err != nil {
		h.pushError(ctx, c, err, in.ClientMsgID)
		return
	}
	c.push(outbound{data: mustJSON(map[string]any{
		"type": "ack", "client_msg_id": msg.ClientMsgID, "id": msg.ID, "created_at": msg.CreatedAt,
	})})
}

// pushError reports err to the client and closes the connection when the
// member may no longer take part in the room.
func (h *Handler) pushError(ctx context.Context, c *Client, err error, clientMsgID string) {
	e, ok := apperr.As(err)
	if !ok {
		slog.ErrorContext(ctx, "ws request failed", "room_id", c.RoomID, "member_id", c.MemberID, "err", err)
		e = &apperr.Error{Code: "internal", Message: "internal server error"}
	}
	o := outbound{data: errorFrame(e, clientMsgID)}
	if code, fatal := fatalCloseCode(e); fatal {
		o.closeCode, o.closeReason = code, e.Code
	}
	c.push(o)
}

func fatalCloseCode(e *apperr.Error) (websocket.StatusCode, bool) {
	switch {
	case errors.Is(e, apperr.Banned), errors.Is(e, apperr.Kicked):
		return CloseRemoved, true
	case errors.Is(e, apperr.RoomClosed):
		return CloseRoomClosed, true
	case errors.Is(e, apperr.Unauthorized), errors.Is(e, apperr.NotFound):
		return CloseUnauthorized, true
	}
	return 0, false
}

func closeWithError(ctx context.Context, conn *websocket.Conn, err error) {
	e, ok := apperr.As(err)
	if !ok {
		slog.ErrorContext(ctx, "ws handshake failed", "err", err)
		_ = conn.Close(websocket.StatusInternalError, "internal error")
		return
	}
	wctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	_ = conn.Write(wctx, websocket.MessageText, errorFrame(e, ""))
	code, fatal := fatalCloseCode(e)
	if !fatal {
		code = CloseUnauthorized
	}
	_ = conn.Close(code, e.Code)
}

func errorFrame(e *apperr.Error, clientMsgID string) []byte {
	frame := map[string]string{"type": "error", "code": e.Code, "message": e.Message}
	if clientMsgID != "" {
		frame["client_msg_id"] = clientMsgID
	}
	return mustJSON(frame)
}

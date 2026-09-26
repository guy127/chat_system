package app

// Integration tests against a real PostgreSQL. They run when TEST_DATABASE_URL
// is set, e.g. postgres://postgres:test@localhost:55432/smalltalk_test?sslmode=disable

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"smalltalk/internal/chat"
	"smalltalk/internal/platform/config"
	"smalltalk/internal/platform/db"
)

var pool *pgxpool.Pool

func TestMain(m *testing.M) {
	slog.SetDefault(slog.New(slog.DiscardHandler))
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		fmt.Println("TEST_DATABASE_URL not set; skipping integration tests")
		os.Exit(0)
	}
	if err := db.Migrate(url); err != nil {
		fmt.Println("migrate:", err)
		os.Exit(1)
	}
	var err error
	pool, err = db.Open(context.Background(), url)
	if err != nil {
		fmt.Println("open:", err)
		os.Exit(1)
	}
	code := m.Run()
	pool.Close()
	os.Exit(code)
}

type testAPI struct {
	t   *testing.T
	srv *httptest.Server
}

func newAPI(t *testing.T) *testAPI {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cfg := config.Config{
		JWTSecret: []byte(strings.Repeat("s", 32)),
		JWTTTL:    time.Hour,
	}
	r, err := NewRouter(ctx, cfg, pool, chat.NewMemoryBroker())
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(r)
	t.Cleanup(func() { srv.Close(); cancel() })
	return &testAPI{t: t, srv: srv}
}

func (a *testAPI) do(method, path, jwt string, body, out any) int {
	a.t.Helper()
	var rd *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req, _ := http.NewRequestWithContext(context.Background(), method, a.srv.URL+path, rd)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if jwt != "" {
		req.Header.Set("Authorization", "Bearer "+jwt)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if out != nil {
		_ = json.NewDecoder(res.Body).Decode(out)
	}
	return res.StatusCode
}

type session struct {
	RoomID     string `json:"room_id"`
	MemberID   string `json:"member_id"`
	JWT        string `json:"jwt"`
	OwnerToken string `json:"owner_token"`
}

type errBody struct {
	Error struct {
		Code string `json:"code"`
	} `json:"error"`
}

func (a *testAPI) createRoom() session {
	a.t.Helper()
	var s session
	if code := a.do("POST", "/rooms", "", map[string]string{"name": "ห้องทดสอบ", "display_name": "Host"}, &s); code != 201 {
		a.t.Fatalf("create room: status %d", code)
	}
	return s
}

func (a *testAPI) invite(host session, body map[string]any) (id, token string) {
	a.t.Helper()
	var inv struct{ ID, Token, InviteURL string }
	if code := a.do("POST", "/rooms/"+host.RoomID+"/invites", host.JWT, body, &inv); code != 201 {
		a.t.Fatalf("create invite: status %d", code)
	}
	return inv.ID, inv.Token
}

// join returns the session, or a non-empty status string on failure.
func (a *testAPI) join(token, name string) (session, string) {
	a.t.Helper()
	var s session
	if code := a.do("POST", "/join", "", map[string]string{"token": token, "display_name": name}, &s); code != 200 {
		return session{}, fmt.Sprint(code)
	}
	return s, ""
}

func (a *testAPI) joinErr(token, name string) string {
	a.t.Helper()
	var e errBody
	a.do("POST", "/join", "", map[string]string{"token": token, "display_name": name}, &e)
	return e.Error.Code
}

type wsClient struct {
	t    *testing.T
	conn *websocket.Conn
	in   chan map[string]any
	done chan struct{}
	mu   sync.Mutex
	err  error
}

func (a *testAPI) dial(s session) *wsClient {
	a.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	url := "ws" + strings.TrimPrefix(a.srv.URL, "http") + "/ws?room=" + s.RoomID
	conn, res, err := websocket.Dial(ctx, url, &websocket.DialOptions{Subprotocols: []string{"chat", s.JWT}})
	if res != nil && res.Body != nil {
		_ = res.Body.Close()
	}
	if err != nil {
		a.t.Fatalf("dial: %v", err)
	}
	c := &wsClient{t: a.t, conn: conn, in: make(chan map[string]any, 256), done: make(chan struct{})}
	go func() {
		defer close(c.done)
		for {
			_, data, err := conn.Read(context.Background())
			if err != nil {
				c.mu.Lock()
				c.err = err
				c.mu.Unlock()
				return
			}
			var m map[string]any
			_ = json.Unmarshal(data, &m)
			c.in <- m
		}
	}()
	a.t.Cleanup(func() { _ = conn.CloseNow() })
	return c
}

func (c *wsClient) send(v any) {
	c.t.Helper()
	b, _ := json.Marshal(v)
	if err := c.conn.Write(context.Background(), websocket.MessageText, b); err != nil {
		c.t.Fatalf("ws write: %v", err)
	}
}

// next waits for the next frame of the given type, skipping others.
func (c *wsClient) next(typ string) map[string]any {
	c.t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case m := <-c.in:
			if m["type"] == typ {
				return m
			}
		case <-c.done:
			// Frames read before the close are still buffered; check them first.
			for {
				select {
				case m := <-c.in:
					if m["type"] == typ {
						return m
					}
					continue
				default:
				}
				break
			}
			c.t.Fatalf("connection closed while waiting for %q: %v", typ, c.err)
		case <-timeout:
			c.t.Fatalf("timed out waiting for %q", typ)
		}
	}
}

func (c *wsClient) closeStatus() websocket.StatusCode {
	c.t.Helper()
	select {
	case <-c.done:
	case <-time.After(5 * time.Second):
		c.t.Fatal("connection was not closed")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return websocket.CloseStatus(c.err)
}

func TestChatFlow(t *testing.T) {
	a := newAPI(t)
	host := a.createRoom()
	_, token := a.invite(host, nil)
	guest, errc := a.join(token, "สมชาย")
	if errc != "" {
		t.Fatalf("join failed: %s", errc)
	}

	hc := a.dial(host)
	gc := a.dial(guest)
	hc.next("presence") // host sees itself, then the guest
	if p := hc.next("presence"); p["member_id"] != guest.MemberID || p["online"] != true {
		t.Fatalf("unexpected presence %v", p)
	}

	cmid := uuid.NewString()
	gc.send(map[string]string{"type": "send", "client_msg_id": cmid, "body": "สวัสดีครับ"})
	ack := gc.next("ack")
	got := hc.next("message")
	if got["body"] != "สวัสดีครับ" || got["display_name"] != "สมชาย" || got["id"] != ack["id"] {
		t.Fatalf("host got %v, ack %v", got, ack)
	}

	// Retrying the same client_msg_id must not create a second message.
	gc.send(map[string]string{"type": "send", "client_msg_id": cmid, "body": "สวัสดีครับ"})
	if ack2 := gc.next("ack"); ack2["id"] != ack["id"] {
		t.Fatalf("duplicate send got new id %v, want %v", ack2["id"], ack["id"])
	}

	for i := range 3 {
		hc.send(map[string]string{"type": "send", "client_msg_id": uuid.NewString(), "body": fmt.Sprintf("m%d", i)})
		hc.next("ack")
	}

	var page struct {
		Messages   []map[string]any `json:"messages"`
		NextCursor *string          `json:"next_cursor"`
	}
	if code := a.do("GET", "/rooms/"+host.RoomID+"/messages?limit=2", guest.JWT, nil, &page); code != 200 {
		t.Fatalf("history status %d", code)
	}
	if len(page.Messages) != 2 || page.Messages[0]["body"] != "m1" || page.Messages[1]["body"] != "m2" || page.NextCursor == nil {
		t.Fatalf("first page = %+v", page)
	}
	cursor := *page.NextCursor
	page.NextCursor = nil
	a.do("GET", "/rooms/"+host.RoomID+"/messages?limit=2&before="+cursor, guest.JWT, nil, &page)
	if len(page.Messages) != 2 || page.Messages[0]["body"] != "สวัสดีครับ" || page.Messages[1]["body"] != "m0" || page.NextCursor != nil {
		t.Fatalf("second page = %+v", page)
	}

	// Reconnect and resume: messages sent while offline arrive first.
	_ = gc.conn.Close(websocket.StatusNormalClosure, "")
	hc.send(map[string]string{"type": "send", "client_msg_id": uuid.NewString(), "body": "while you were away"})
	hc.next("ack")
	gc2 := a.dial(guest)
	gc2.send(map[string]string{"type": "resume", "last_message_id": ack["id"].(string)})
	for _, want := range []string{"m0", "m1", "m2", "while you were away"} {
		if m := gc2.next("message"); m["body"] != want {
			t.Fatalf("resume got %v, want %q", m["body"], want)
		}
	}

	var members struct {
		Members []struct {
			ID     string `json:"id"`
			Online bool   `json:"online"`
		} `json:"members"`
	}
	a.do("GET", "/rooms/"+host.RoomID+"/members", guest.JWT, nil, &members)
	if len(members.Members) != 2 || !members.Members[0].Online || !members.Members[1].Online {
		t.Fatalf("members = %+v", members)
	}
}

func TestMessageValidationAndRateLimit(t *testing.T) {
	a := newAPI(t)
	host := a.createRoom()
	c := a.dial(host)

	c.send(map[string]string{"type": "send", "client_msg_id": uuid.NewString(), "body": strings.Repeat("ก", 2001)})
	if e := c.next("error"); e["code"] != "message_too_long" {
		t.Fatalf("got %v", e)
	}

	limited := false
	for range 8 {
		c.send(map[string]string{"type": "send", "client_msg_id": uuid.NewString(), "body": "spam"})
	}
drain:
	for {
		select {
		case m := <-c.in:
			if m["type"] == "error" && m["code"] == "rate_limited" {
				limited = true
			}
		case <-time.After(500 * time.Millisecond):
			break drain
		}
	}
	if !limited {
		t.Fatal("expected rate_limited after bursting 8 messages")
	}
}

func TestInviteRules(t *testing.T) {
	a := newAPI(t)
	host := a.createRoom()

	_, one := a.invite(host, map[string]any{"max_uses": 1})
	if _, e := a.join(one, "a"); e != "" {
		t.Fatalf("first use failed: %s", e)
	}
	if code := a.joinErr(one, "b"); code != "invite_exhausted" {
		t.Fatalf("second use = %q, want invite_exhausted", code)
	}

	id, revoked := a.invite(host, nil)
	if code := a.do("DELETE", "/rooms/"+host.RoomID+"/invites/"+id, host.JWT, nil, nil); code != 204 {
		t.Fatalf("revoke status %d", code)
	}
	if code := a.joinErr(revoked, "c"); code != "invite_invalid" {
		t.Fatalf("revoked = %q", code)
	}

	_, expiring := a.invite(host, nil)
	if _, err := pool.Exec(context.Background(),
		`UPDATE room_invites SET expires_at = now() - interval '1 second' WHERE token_hash = encode(sha256($1::bytea), 'hex')`,
		expiring); err != nil {
		t.Fatal(err)
	}
	if code := a.joinErr(expiring, "d"); code != "invite_expired" {
		t.Fatalf("expired = %q", code)
	}
	if code := a.joinErr("not-a-real-token", "e"); code != "invite_invalid" {
		t.Fatalf("unknown = %q", code)
	}

	// Guests cannot manage invites.
	_, tok := a.invite(host, nil)
	guest, _ := a.join(tok, "guest")
	var e errBody
	if code := a.do("POST", "/rooms/"+host.RoomID+"/invites", guest.JWT, nil, &e); code != 403 {
		t.Fatalf("guest create invite status %d", code)
	}
}

func TestJoinRateLimit(t *testing.T) {
	a := newAPI(t)
	codes := map[string]int{}
	for range 11 {
		codes[a.joinErr("nope", "x")]++
	}
	if codes["rate_limited"] != 1 || codes["invite_invalid"] != 10 {
		t.Fatalf("codes = %v", codes)
	}
}

func TestKickAndBan(t *testing.T) {
	a := newAPI(t)
	host := a.createRoom()
	_, token := a.invite(host, nil)
	kicked, _ := a.join(token, "kickme")
	banned, _ := a.join(token, "banme")

	kc := a.dial(kicked)
	bc := a.dial(banned)
	kc.next("presence")
	bc.next("presence")

	if code := a.do("POST", "/rooms/"+host.RoomID+"/members/"+kicked.MemberID+"/kick", host.JWT, nil, nil); code != 204 {
		t.Fatalf("kick status %d", code)
	}
	if f := kc.next("kicked"); f["reason"] != "kicked" {
		t.Fatalf("kick frame %v", f)
	}
	if s := kc.closeStatus(); s != chat.CloseRemoved {
		t.Fatalf("kick close status %d", s)
	}

	if code := a.do("POST", "/rooms/"+host.RoomID+"/members/"+banned.MemberID+"/kick", host.JWT, map[string]bool{"ban": true}, nil); code != 204 {
		t.Fatalf("ban status %d", code)
	}
	if f := bc.next("kicked"); f["reason"] != "banned" {
		t.Fatalf("ban frame %v", f)
	}

	var e errBody
	a.do("GET", "/rooms/"+host.RoomID+"/messages", banned.JWT, nil, &e)
	if e.Error.Code != "banned" {
		t.Fatalf("banned REST = %q", e.Error.Code)
	}
	again := a.dial(kicked)
	if f := again.next("error"); f["code"] != "kicked" {
		t.Fatalf("kicked reconnect = %v", f)
	}

	if code := a.do("POST", "/rooms/"+host.RoomID+"/members/"+host.MemberID+"/kick", host.JWT, nil, nil); code != 400 {
		t.Fatalf("kicking host status %d, want 400", code)
	}
}

func TestCloseRoomAndOwnerSession(t *testing.T) {
	a := newAPI(t)
	host := a.createRoom()
	_, token := a.invite(host, nil)
	guest, _ := a.join(token, "g")
	gc := a.dial(guest)
	gc.next("presence")

	var sess session
	if code := a.do("POST", "/rooms/"+host.RoomID+"/owner-session", "", map[string]string{"owner_token": host.OwnerToken}, &sess); code != 200 || sess.MemberID != host.MemberID {
		t.Fatalf("owner session status %d %+v", code, sess)
	}
	if code := a.do("POST", "/rooms/"+host.RoomID+"/owner-session", "", map[string]string{"owner_token": "wrong"}, nil); code != 401 {
		t.Fatalf("wrong owner token status %d", code)
	}

	if code := a.do("POST", "/rooms/"+host.RoomID+"/close", sess.JWT, nil, nil); code != 204 {
		t.Fatalf("close status %d", code)
	}
	gc.next("room_closed")
	if s := gc.closeStatus(); s != chat.CloseRoomClosed {
		t.Fatalf("close status %d", s)
	}
	if code := a.joinErr(token, "late"); code != "room_closed" {
		t.Fatalf("join closed room = %q", code)
	}
}

func TestRoomFull(t *testing.T) {
	a := newAPI(t)
	host := a.createRoom()
	_, token := a.invite(host, nil)
	// Fill up to the cap directly; the join limiter would stop us over HTTP.
	for range 199 {
		if _, err := pool.Exec(context.Background(),
			`INSERT INTO room_members (id, room_id, display_name) VALUES ($1, $2, 'filler')`,
			uuid.New(), host.RoomID); err != nil {
			t.Fatal(err)
		}
	}
	if code := a.joinErr(token, "one too many"); code != "room_full" {
		t.Fatalf("join full room = %q", code)
	}
}

package app

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"smalltalk/internal/chat"
	"smalltalk/internal/platform/config"
)

type serviceRoom struct {
	RoomID string `json:"room_id"`
	Status string `json:"status"`
}

func (a *testAPI) ensureServiceRoom(ref string) serviceRoom {
	a.t.Helper()
	var r serviceRoom
	if code := a.do("PUT", "/service/v1/rooms/"+ref, testServiceKey, map[string]string{"name": "เรื่อง #1"}, &r); code != 200 {
		a.t.Fatalf("ensure room status %d", code)
	}
	return r
}

func (a *testAPI) serviceSession(ref, userID, name, label string) (session, int) {
	a.t.Helper()
	var s session
	code := a.do("POST", "/service/v1/rooms/"+ref+"/sessions", testServiceKey,
		map[string]string{"external_user_id": userID, "display_name": name, "label": label}, &s)
	return s, code
}

func TestServiceAPIKey(t *testing.T) {
	a := newAPI(t)
	ref := testRef()
	body := map[string]string{"name": "x"}
	if code := a.do("PUT", "/service/v1/rooms/"+ref, "", body, nil); code != 401 {
		t.Fatalf("no key = %d", code)
	}
	if code := a.do("PUT", "/service/v1/rooms/"+ref, "wrong-key-wrong-key-wrong-key-wrong", body, nil); code != 401 {
		t.Fatalf("wrong key = %d", code)
	}
	// A room JWT is not a service key.
	host := a.createRoom()
	if code := a.do("PUT", "/service/v1/rooms/"+ref, host.JWT, body, nil); code != 401 {
		t.Fatalf("room jwt = %d", code)
	}

	off := newAPIWith(t, func(c *config.Config) { c.ServiceAPIKey = "" })
	if code := off.do("PUT", "/service/v1/rooms/"+ref, testServiceKey, body, nil); code != 404 {
		t.Fatalf("disabled = %d", code)
	}
}

func TestServiceRoomsAndSessions(t *testing.T) {
	a := newAPI(t)
	ref := testRef()
	first := a.ensureServiceRoom(ref)
	if again := a.ensureServiceRoom(ref); again != first || first.Status != "active" {
		t.Fatalf("ensure twice = %+v then %+v", first, again)
	}
	if code := a.do("PUT", "/service/v1/rooms/Bad%20Ref", testServiceKey, map[string]string{"name": "x"}, nil); code != 400 {
		t.Fatalf("bad ref = %d", code)
	}
	if _, code := a.serviceSession(testRef(), "user:1", "A", ""); code != 404 {
		t.Fatalf("session before PUT = %d", code)
	}
	if _, code := a.serviceSession(ref, "bad id", "A", ""); code != 400 {
		t.Fatalf("bad external_user_id = %d", code)
	}

	before := time.Now()
	emp, code := a.serviceSession(ref, "user:1", "สมชาย", "ผู้ยื่น")
	if code != 200 || emp.RoomID != first.RoomID || emp.Role != "member" {
		t.Fatalf("session = %d %+v", code, emp)
	}
	if ttl := emp.ExpiresAt.Sub(before); ttl < 14*time.Minute || ttl > 16*time.Minute {
		t.Fatalf("service JWT lifetime %v, want ~15m", ttl)
	}
	again, _ := a.serviceSession(ref, "user:1", "สมชาย ใจดี", "ผู้ยื่น")
	if again.MemberID != emp.MemberID {
		t.Fatalf("same user got member %s then %s", emp.MemberID, again.MemberID)
	}
	staff, _ := a.serviceSession(ref, "user:2", "วิภา", "เจ้าหน้าที่ตรวจสอบ")

	// Sessions work on the normal member routes and the WebSocket.
	sc := a.dial(staff)
	ec := a.dial(again)
	ec.send(map[string]string{"type": "send", "client_msg_id": uuid.NewString(), "body": "แนบใบเสร็จแล้วครับ"})
	ec.next("ack")
	m := sc.next("message")
	if m["display_name"] != "สมชาย ใจดี" || m["label"] != "ผู้ยื่น" {
		t.Fatalf("message = %v", m)
	}
	var members struct {
		Members []struct {
			DisplayName string `json:"display_name"`
			Label       string `json:"label"`
			Role        string `json:"role"`
		} `json:"members"`
	}
	a.do("GET", "/rooms/"+first.RoomID+"/members", staff.JWT, nil, &members)
	if len(members.Members) != 2 || members.Members[1].Label != "เจ้าหน้าที่ตรวจสอบ" || members.Members[0].Role != "member" {
		t.Fatalf("members = %+v", members)
	}
}

func TestServiceCloseRoom(t *testing.T) {
	a := newAPI(t)
	ref := testRef()
	rm := a.ensureServiceRoom(ref)
	emp, _ := a.serviceSession(ref, "user:1", "A", "")
	ec := a.dial(emp)
	ec.send(map[string]string{"type": "send", "client_msg_id": uuid.NewString(), "body": "ก่อนปิด"})
	ec.next("ack")

	if code := a.do("POST", "/service/v1/rooms/"+ref+"/close", testServiceKey, nil, nil); code != 204 {
		t.Fatalf("close = %d", code)
	}
	ec.next("room_closed")
	if s := ec.closeStatus(); s != chat.CloseRoomClosed {
		t.Fatalf("ws close status %d", s)
	}
	if code := a.do("POST", "/service/v1/rooms/"+ref+"/close", testServiceKey, nil, nil); code != 204 {
		t.Fatalf("second close = %d", code)
	}
	if st := a.ensureServiceRoom(ref); st.Status != "closed" || st.RoomID != rm.RoomID {
		t.Fatalf("after close = %+v", st)
	}

	// Still readable: a new session can load history, but cannot post.
	late, code := a.serviceSession(ref, "user:1", "A", "")
	if code != 200 {
		t.Fatalf("session on closed room = %d", code)
	}
	var page struct {
		Messages []map[string]any `json:"messages"`
	}
	if code := a.do("GET", "/rooms/"+rm.RoomID+"/messages", late.JWT, nil, &page); code != 200 || len(page.Messages) != 1 {
		t.Fatalf("history = %d %v", code, page.Messages)
	}
	lc := a.dial(late)
	if s := lc.closeStatus(); s != chat.CloseRoomClosed {
		t.Fatalf("ws on closed room status %d", s)
	}
}

func TestServiceRemovedMember(t *testing.T) {
	a := newAPI(t)
	ref := testRef()
	a.ensureServiceRoom(ref)
	s, _ := a.serviceSession(ref, "user:9", "A", "")
	if _, err := pool.Exec(context.Background(), `UPDATE room_members SET banned_at = now() WHERE id = $1`, s.MemberID); err != nil {
		t.Fatal(err)
	}
	var e errBody
	if code := a.do("POST", "/service/v1/rooms/"+ref+"/sessions", testServiceKey,
		map[string]string{"external_user_id": "user:9", "display_name": "A"}, &e); code != 409 || e.Error.Code != "member_removed" {
		t.Fatalf("removed member = %d %q", code, e.Error.Code)
	}
}

func (a *testAPI) serviceImage(ref, imageID string) (int, http.Header, []byte) {
	a.t.Helper()
	req, _ := http.NewRequestWithContext(context.Background(), "GET",
		a.srv.URL+"/service/v1/rooms/"+ref+"/images/"+imageID, nil)
	req.Header.Set("Authorization", "Bearer "+testServiceKey)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, res.Header, b
}

func TestServiceSentImage(t *testing.T) {
	a := newAPI(t)
	ref, otherRef := testRef(), testRef()
	a.ensureServiceRoom(ref)
	a.ensureServiceRoom(otherRef)
	emp, _ := a.serviceSession(ref, "user:1", "A", "ผู้ยื่น")
	staff, _ := a.serviceSession(ref, "user:2", "B", "เจ้าหน้าที่")

	_, img := a.upload(emp, "receipt.png", pngBytes(t, 40, 30))
	sentID := img["id"].(string)
	ec := a.dial(emp)
	ec.send(map[string]any{"type": "send", "client_msg_id": uuid.NewString(), "body": "", "image_id": sentID})
	ec.next("ack")
	_, unsent := a.upload(staff, "draft.png", pngBytes(t, 2, 2))

	code, hdr, data := a.serviceImage(ref, sentID)
	if code != 200 || hdr.Get("Content-Type") != "image/png" || hdr.Get("X-Sender-External-Id") != "user:1" {
		t.Fatalf("sent image = %d %v", code, hdr)
	}
	_, _, viaMember := a.getImage(emp, emp.RoomID, sentID)
	if !bytes.Equal(data, viaMember) {
		t.Fatal("service bytes differ from what members see")
	}

	for name, path := range map[string][2]string{
		"unsent upload": {ref, unsent["id"].(string)},
		"other room":    {otherRef, sentID},
		"unknown image": {ref, uuid.NewString()},
		"not a uuid":    {ref, "nope"},
		"unknown room":  {testRef(), sentID},
	} {
		if code, _, _ := a.serviceImage(path[0], path[1]); code != 404 {
			t.Errorf("%s = %d, want 404", name, code)
		}
	}
}

func TestServiceRoomsRejectInvites(t *testing.T) {
	a := newAPI(t)
	ref := testRef()
	rm := a.ensureServiceRoom(ref)
	s, _ := a.serviceSession(ref, "user:1", "A", "")
	ctx := context.Background()

	// Service rooms have no owner; promote one by hand to reach the invite routes.
	if _, err := pool.Exec(ctx, `UPDATE room_members SET role = 'owner' WHERE id = $1`, s.MemberID); err != nil {
		t.Fatal(err)
	}
	var e errBody
	if code := a.do("POST", "/rooms/"+rm.RoomID+"/invites", s.JWT, nil, &e); code != 404 || e.Error.Code != "not_found" {
		t.Fatalf("create invite = %d %q", code, e.Error.Code)
	}

	// An invite row that somehow exists must not let anyone in.
	token := "svc-" + uuid.NewString()
	if _, err := pool.Exec(ctx,
		`INSERT INTO room_invites (id, room_id, token_hash, expires_at)
		 VALUES ($1, $2, encode(sha256($3::bytea), 'hex'), now() + interval '1 hour')`,
		uuid.New(), rm.RoomID, token); err != nil {
		t.Fatal(err)
	}
	if code := a.joinErr(token, "intruder"); code != "not_found" {
		t.Fatalf("join service room = %q", code)
	}
}

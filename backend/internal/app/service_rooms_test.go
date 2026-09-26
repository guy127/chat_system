package app

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"smalltalk/internal/apperr"
	"smalltalk/internal/auth"
	"smalltalk/internal/room"
)

// roomService builds a room.Service on the test pool; these tests never
// notify or read presence, so both are nil.
func roomService() *room.Service {
	issuer := auth.NewIssuer([]byte(strings.Repeat("s", 32)), time.Hour)
	return room.NewService(room.NewRepository(pool), issuer, nil, nil)
}

func testRef() string { return "test:" + uuid.NewString() }

func TestEnsureByRefConcurrent(t *testing.T) {
	svc := roomService()
	ref := testRef()
	ids := make([]uuid.UUID, 10)
	var wg sync.WaitGroup
	for i := range ids {
		wg.Go(func() {
			rm, err := svc.EnsureByRef(context.Background(), ref, "Ticket")
			if err != nil {
				t.Error(err)
				return
			}
			ids[i] = rm.ID
		})
	}
	wg.Wait()
	for _, id := range ids {
		if id != ids[0] {
			t.Fatalf("got different rooms for one ref: %v", ids)
		}
	}
	rm, err := svc.EnsureByRef(context.Background(), ref, "Renamed")
	if err != nil || rm.ID != ids[0] || rm.Name != "Ticket" || rm.ExternalRef == nil || *rm.ExternalRef != ref {
		t.Fatalf("second ensure = %+v, %v", rm, err)
	}
	if _, err := svc.GetByRef(context.Background(), testRef()); err != apperr.NotFound {
		t.Fatalf("unknown ref err = %v", err)
	}
}

func TestJoinExternalUpserts(t *testing.T) {
	svc := roomService()
	ctx := context.Background()
	rm, err := svc.EnsureByRef(ctx, testRef(), "Ticket")
	if err != nil {
		t.Fatal(err)
	}
	first, err := svc.JoinExternal(ctx, rm.ID, "user:1", "สมชาย", "ผู้ยื่น")
	if err != nil {
		t.Fatal(err)
	}
	again, err := svc.JoinExternal(ctx, rm.ID, "user:1", "สมชาย ใจดี", "เจ้าหน้าที่ตรวจสอบ")
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != first.ID || again.DisplayName != "สมชาย ใจดี" || again.Label != "เจ้าหน้าที่ตรวจสอบ" || again.Role != room.RoleMember {
		t.Fatalf("upsert = %+v, first %+v", again, first)
	}
	other, _ := svc.JoinExternal(ctx, rm.ID, "user:2", "B", "")
	if other.ID == first.ID {
		t.Fatal("different external users must be different members")
	}

	if _, err := svc.JoinExternal(ctx, rm.ID, "user:3", "C", strings.Repeat("ก", room.MaxLabelLen+1)); !isCode(err, "invalid_input") {
		t.Fatalf("long label err = %v", err)
	}
	if _, err := svc.JoinExternal(ctx, rm.ID, "user:3", strings.Repeat("ก", room.MaxServiceNameLen), ""); err != nil {
		t.Fatalf("64-character name err = %v", err)
	}

	if _, err := pool.Exec(ctx, `UPDATE room_members SET kicked_at = now() WHERE id = $1`, first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.JoinExternal(ctx, rm.ID, "user:1", "สมชาย", ""); err != apperr.MemberRemoved {
		t.Fatalf("removed member err = %v", err)
	}
}

func isCode(err error, code string) bool {
	e, ok := apperr.As(err)
	return ok && e.Code == code
}

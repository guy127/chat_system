package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestIssueForUsesGivenTTL(t *testing.T) {
	now := time.Date(2026, 9, 26, 6, 0, 0, 0, time.UTC)
	i := NewIssuer([]byte(strings.Repeat("s", 32)), 12*time.Hour)
	i.now = func() time.Time { return now }
	c := Claims{RoomID: uuid.New(), MemberID: uuid.New(), Role: "member"}

	s, err := i.IssueFor(c, 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if !s.ExpiresAt.Equal(now.Add(15 * time.Minute)) {
		t.Fatalf("service session expires %v", s.ExpiresAt)
	}
	if got, err := i.Parse(s.JWT); err != nil || got != c {
		t.Fatalf("parse = %+v, %v", got, err)
	}
	d, _ := i.Issue(c)
	if !d.ExpiresAt.Equal(now.Add(12 * time.Hour)) {
		t.Fatalf("default session expires %v", d.ExpiresAt)
	}
	i.now = func() time.Time { return now.Add(16 * time.Minute) }
	if _, err := i.Parse(s.JWT); err == nil {
		t.Fatal("expired service JWT must not parse")
	}
}

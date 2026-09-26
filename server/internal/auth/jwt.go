package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Claims bind a token to exactly one room and one member.
type Claims struct {
	RoomID   uuid.UUID
	MemberID uuid.UUID
	Role     string
}

// Session is what a client receives after joining or creating a room.
type Session struct {
	RoomID    uuid.UUID `json:"room_id"`
	MemberID  uuid.UUID `json:"member_id"`
	Role      string    `json:"role"`
	JWT       string    `json:"jwt"`
	ExpiresAt time.Time `json:"expires_at"`
}

type jwtClaims struct {
	Room string `json:"room"`
	Role string `json:"role"`
	jwt.RegisteredClaims
}

type Issuer struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

func NewIssuer(secret []byte, ttl time.Duration) *Issuer {
	return &Issuer{secret: secret, ttl: ttl, now: time.Now}
}

func (i *Issuer) Issue(c Claims) (Session, error) {
	now := i.now().UTC()
	exp := now.Add(i.ttl)
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwtClaims{
		Room: c.RoomID.String(),
		Role: c.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   c.MemberID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	})
	signed, err := tok.SignedString(i.secret)
	if err != nil {
		return Session{}, fmt.Errorf("sign jwt: %w", err)
	}
	return Session{RoomID: c.RoomID, MemberID: c.MemberID, Role: c.Role, JWT: signed, ExpiresAt: exp}, nil
}

func (i *Issuer) Parse(token string) (Claims, error) {
	var jc jwtClaims
	_, err := jwt.ParseWithClaims(token, &jc,
		func(*jwt.Token) (any, error) { return i.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(i.now),
	)
	if err != nil {
		return Claims{}, fmt.Errorf("parse jwt: %w", err)
	}
	roomID, err := uuid.Parse(jc.Room)
	if err != nil {
		return Claims{}, fmt.Errorf("parse jwt room: %w", err)
	}
	memberID, err := uuid.Parse(jc.Subject)
	if err != nil {
		return Claims{}, fmt.Errorf("parse jwt subject: %w", err)
	}
	return Claims{RoomID: roomID, MemberID: memberID, Role: jc.Role}, nil
}

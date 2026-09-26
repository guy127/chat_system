# smalltalk Service API Implementation Plan (Plan 1 of 2: smalltalk side)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn smalltalk into a chat service that a trusted backend (the medical claims system first) can call: it creates rooms by external reference, issues member sessions for its own users, closes rooms and fetches sent images, while browsers from the caller's origin talk to the public REST/WebSocket API directly.

**Architecture:** A new `internal/service` package serves `/service/v1/*` behind a shared API key and orchestrates the existing `room` and `media` services. Migration `000006` adds `rooms.external_ref` and `room_members.external_user_id` / `label`. A CORS middleware in `httpx` opens the public REST API to configured origins. Invites are refused for service rooms. The claims-system side (fork `guy127/pea-medical-claims`) is Plan 2, written after this plan ships because it consumes this API.

**Tech Stack:** Go 1.26, Gin, pgx/v5, golang-migrate, coder/websocket, PostgreSQL 17, OpenAPI 3.1 (`docs/openapi.yaml`), Docker Compose + Nginx.

**Spec:** `docs/superpowers/specs/2026-09-26-claims-chat-integration-design.md` (sections "ฝั่ง smalltalk (repo นี้)", "Error handling สรุป", "การทดสอบ → smalltalk").

## Global Constraints

- Service routes live under `/service/v1` and require `Authorization: Bearer <SERVICE_API_KEY>`, compared in constant time.
- `SERVICE_API_KEY` empty = every `/service/v1/*` route answers `404`; when set it must be ≥ 32 characters.
- `SERVICE_JWT_TTL` default `15m`; normal rooms keep `JWT_TTL` behaviour (12 h in `config.Load`).
- `CORS_ORIGINS` default empty; allowed request headers exactly `Authorization, Content-Type`; never send `Access-Control-Allow-Credentials`.
- `ref` must match `^[a-z0-9:_-]{1,128}$`.
- Sessions from `/service/v1` always have `role: "member"`; service rooms never get an owner member.
- Nginx must not forward `/service` (i.e. `/api/service/...` from the browser side) — internal network only.
- Invite creation and joining on a room with `external_ref` answers `404` with code `not_found` (the spec's "room_not_found" means the existing not-found response; the codebase's code is `not_found`).
- Error envelope stays `{"error":{"code","message"}}`; new error codes go in `apperr` + `httpx.statusByCode` + `ErrorCode` enum in `docs/openapi.yaml`.
- `cmd/server/openapi_test.go` must stay green: every route added must be documented in `docs/openapi.yaml` in the same task.
- CI gates (run before each commit): `gofmt -l .` prints nothing, `go vet ./...`, `go test -count=1 ./...` with `TEST_DATABASE_URL` set, and `golangci-lint run ./...` if installed.
- Code comments in English, matching the existing density (one doc comment per exported symbol that is not obvious).

## Review Focus

1. **Two people open the same ticket at the same moment** → both `PUT /service/v1/rooms/{ref}` calls must return the same `room_id` (no unique-violation 500). Pinned in Task 2 (`TestEnsureByRefConcurrent`).
2. **A browser reaches `/api/service/v1/...` through the public Nginx** → must get `404`, never reach the API. Pinned in Task 6 (manual curl step against compose).
3. **Claims asks for an image that was uploaded but never sent, or belongs to another ticket's room** → `404`, never the bytes. Pinned in Task 4 (`TestServiceSentImage`).
4. **A user's name or role label changes in the claims system and they reopen the chat** → same `member_id`, updated `display_name`/`label` in new messages and the member list. Pinned in Task 2 (`TestJoinExternalUpserts`).
5. **A ticket is paid, the room is closed, then someone opens the chat again** → session still issued, history readable, WebSocket closes with `4004`, repeated `close` is a harmless `204`. Pinned in Task 3 (`TestServiceCloseRoom`).

---

## Running the integration tests locally

Integration tests in `backend/internal/app` skip unless `TEST_DATABASE_URL` is set. Start a throwaway Postgres once:

```bash
docker run -d --name smalltalk-test-db -e POSTGRES_PASSWORD=test -e POSTGRES_DB=smalltalk_test -p 55432:5432 postgres:17-alpine
export TEST_DATABASE_URL='postgres://postgres:test@localhost:55432/smalltalk_test?sslmode=disable'
```

All commands below run from `backend/` unless stated.

---

### Task 1: Config and CORS middleware

**Files:**
- Modify: `backend/internal/platform/config/config.go`
- Create: `backend/internal/platform/config/config_test.go`
- Create: `backend/internal/platform/httpx/cors.go`
- Create: `backend/internal/platform/httpx/cors_test.go`
- Modify: `backend/internal/app/app.go` (add middleware)
- Modify: `backend/internal/app/app_test.go` (`newAPI` → `newAPIWith`, CORS integration test)

**Interfaces:**
- Consumes: nothing new.
- Produces:
  - `config.Config.ServiceAPIKey string`, `config.Config.ServiceJWTTTL time.Duration`, `config.Config.CORSOrigins []string`
  - `httpx.CORS(origins []string) gin.HandlerFunc`
  - Test helpers in package `app`: `newAPIWith(t *testing.T, edit func(*config.Config)) *testAPI`, consts `testServiceKey`, `testOrigin`. `newAPI(t)` keeps working and now sets `ServiceAPIKey: testServiceKey`, `ServiceJWTTTL: 15 * time.Minute`, `CORSOrigins: []string{testOrigin}`.

- [ ] **Step 1: Write the failing config tests**

`backend/internal/platform/config/config_test.go`:

```go
package config

import (
	"strings"
	"testing"
	"time"
)

func setRequired(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://localhost/x")
	t.Setenv("JWT_SECRET", strings.Repeat("s", 32))
	t.Setenv("SERVICE_API_KEY", "")
	t.Setenv("SERVICE_JWT_TTL", "")
	t.Setenv("CORS_ORIGINS", "")
}

func TestLoadServiceDefaults(t *testing.T) {
	setRequired(t)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ServiceAPIKey != "" || cfg.ServiceJWTTTL != 15*time.Minute || cfg.CORSOrigins != nil {
		t.Fatalf("defaults = %q %v %v", cfg.ServiceAPIKey, cfg.ServiceJWTTTL, cfg.CORSOrigins)
	}
}

func TestLoadServiceSettings(t *testing.T) {
	setRequired(t)
	key := strings.Repeat("k", 32)
	t.Setenv("SERVICE_API_KEY", " "+key+"\n") // stray whitespace from .env files is trimmed
	t.Setenv("SERVICE_JWT_TTL", "5m")
	t.Setenv("CORS_ORIGINS", "https://a.test, https://b.test")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ServiceAPIKey != key || cfg.ServiceJWTTTL != 5*time.Minute {
		t.Fatalf("got %q %v", cfg.ServiceAPIKey, cfg.ServiceJWTTTL)
	}
	if len(cfg.CORSOrigins) != 2 || cfg.CORSOrigins[1] != "https://b.test" {
		t.Fatalf("cors = %v", cfg.CORSOrigins)
	}
}

func TestLoadRejectsShortServiceKey(t *testing.T) {
	setRequired(t)
	t.Setenv("SERVICE_API_KEY", "too-short")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for a key under 32 characters")
	}
}

func TestLoadRejectsBadServiceTTL(t *testing.T) {
	for _, v := range []string{"abc", "0s", "-1m"} {
		setRequired(t)
		t.Setenv("SERVICE_JWT_TTL", v)
		if _, err := Load(); err == nil {
			t.Fatalf("SERVICE_JWT_TTL=%q: expected error", v)
		}
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/platform/config/`
Expected: FAIL — `cfg.ServiceAPIKey undefined` (compile error).

- [ ] **Step 3: Implement config**

In `config.go`, add to `Config` after `MediaDir`:

```go
	// ServiceAPIKey lets trusted backends call /service/v1. Empty turns that API off.
	ServiceAPIKey string
	// ServiceJWTTTL is the lifetime of JWTs issued through /service/v1.
	ServiceJWTTTL time.Duration
	// CORSOrigins are full origins (https://claims.example.com) whose pages may
	// call the REST API from the browser.
	CORSOrigins []string
```

In `Load`, add to the literal:

```go
		ServiceAPIKey:  strings.TrimSpace(os.Getenv("SERVICE_API_KEY")),
		CORSOrigins:    list(os.Getenv("CORS_ORIGINS")),
```

and after the `JWT_SECRET` check:

```go
	if cfg.ServiceAPIKey != "" && len(cfg.ServiceAPIKey) < 32 {
		return Config{}, errors.New("SERVICE_API_KEY must be at least 32 characters")
	}
	ttl, err := time.ParseDuration(env("SERVICE_JWT_TTL", "15m"))
	if err != nil || ttl <= 0 {
		return Config{}, errors.New("SERVICE_JWT_TTL must be a positive duration such as 15m")
	}
	cfg.ServiceJWTTTL = ttl
```

- [ ] **Step 4: Run config tests**

Run: `go test ./internal/platform/config/`
Expected: PASS.

- [ ] **Step 5: Write the failing CORS unit tests**

`backend/internal/platform/httpx/cors_test.go`:

```go
package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

const allowedOrigin = "https://claims.test"

func corsRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CORS([]string{allowedOrigin}))
	ok := func(c *gin.Context) { c.String(http.StatusOK, "ok") }
	r.GET("/rooms/:id", ok)
	r.PUT("/service/v1/rooms/:ref", ok)
	return r
}

func corsDo(method, path, origin string, preflight bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if preflight {
		req.Header.Set("Access-Control-Request-Method", "POST")
		req.Header.Set("Access-Control-Request-Headers", "authorization, content-type")
	}
	w := httptest.NewRecorder()
	corsRouter().ServeHTTP(w, req)
	return w
}

func TestCORSPreflightAllowed(t *testing.T) {
	w := corsDo(http.MethodOptions, "/rooms/x/messages", allowedOrigin, true)
	h := w.Header()
	if w.Code != http.StatusNoContent || h.Get("Access-Control-Allow-Origin") != allowedOrigin {
		t.Fatalf("status %d, allow-origin %q", w.Code, h.Get("Access-Control-Allow-Origin"))
	}
	if h.Get("Access-Control-Allow-Headers") != "Authorization, Content-Type" {
		t.Fatalf("allow-headers %q", h.Get("Access-Control-Allow-Headers"))
	}
	if h.Get("Access-Control-Allow-Credentials") != "" {
		t.Fatal("credentials must never be allowed")
	}
}

func TestCORSPreflightRejected(t *testing.T) {
	w := corsDo(http.MethodOptions, "/rooms/x", "https://evil.test", true)
	if w.Code != http.StatusForbidden || w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("status %d, allow-origin %q", w.Code, w.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestCORSSimpleRequests(t *testing.T) {
	cases := []struct {
		name, path, origin, want string
	}{
		{"allowed origin", "/rooms/x", allowedOrigin, allowedOrigin},
		{"other origin", "/rooms/x", "https://evil.test", ""},
		{"same origin (no header)", "/rooms/x", "", ""},
		{"service API is server-to-server", "/service/v1/rooms/r", allowedOrigin, ""},
	}
	for _, tc := range cases {
		method := http.MethodGet
		if tc.path == "/service/v1/rooms/r" {
			method = http.MethodPut
		}
		w := corsDo(method, tc.path, tc.origin, false)
		if w.Code != http.StatusOK || w.Header().Get("Access-Control-Allow-Origin") != tc.want {
			t.Errorf("%s: status %d, allow-origin %q, want %q", tc.name, w.Code, w.Header().Get("Access-Control-Allow-Origin"), tc.want)
		}
	}
}
```

- [ ] **Step 6: Run to verify they fail**

Run: `go test ./internal/platform/httpx/`
Expected: FAIL — `undefined: CORS`.

- [ ] **Step 7: Implement CORS**

`backend/internal/platform/httpx/cors.go`:

```go
package httpx

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// CORS lets pages from the listed origins call the REST API. JWTs travel in
// the Authorization header, so credentials are never allowed. /service/v1 is
// server-to-server and never answers cross-origin requests.
func CORS(origins []string) gin.HandlerFunc {
	allowed := make(map[string]bool, len(origins))
	for _, o := range origins {
		allowed[o] = true
	}
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin == "" || strings.HasPrefix(c.Request.URL.Path, "/service/") {
			c.Next()
			return
		}
		preflight := c.Request.Method == http.MethodOptions && c.GetHeader("Access-Control-Request-Method") != ""
		h := c.Writer.Header()
		h.Add("Vary", "Origin")
		if !allowed[origin] {
			if preflight {
				c.AbortWithStatus(http.StatusForbidden)
				return
			}
			c.Next()
			return
		}
		h.Set("Access-Control-Allow-Origin", origin)
		if preflight {
			h.Set("Access-Control-Allow-Methods", "GET, POST, DELETE")
			h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			h.Set("Access-Control-Max-Age", "600")
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
```

Note: preflights hit no registered route, so they reach this middleware through Gin's NoRoute chain (Gin runs `r.Use` middleware on NoRoute too) and abort before the 404.

- [ ] **Step 8: Run CORS tests**

Run: `go test ./internal/platform/httpx/`
Expected: PASS.

- [ ] **Step 9: Wire it and add the integration test**

In `backend/internal/app/app.go` change the middleware line to:

```go
	r.Use(gin.Recovery(), httpx.RequestLog(), httpx.CORS(cfg.CORSOrigins))
```

In `backend/internal/app/app_test.go` replace `newAPI` with:

```go
const (
	testServiceKey = "test-service-key-0123456789abcdef"
	testOrigin     = "http://claims.test"
)

func newAPI(t *testing.T) *testAPI { return newAPIWith(t, nil) }

// newAPIWith builds the API with test defaults; edit may change the config first.
func newAPIWith(t *testing.T, edit func(*config.Config)) *testAPI {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cfg := config.Config{
		JWTSecret:     []byte(strings.Repeat("s", 32)),
		JWTTTL:        time.Hour,
		MediaDir:      t.TempDir(),
		ServiceAPIKey: testServiceKey,
		ServiceJWTTTL: 15 * time.Minute,
		CORSOrigins:   []string{testOrigin},
	}
	if edit != nil {
		edit(&cfg)
	}
	r, err := NewRouter(ctx, cfg, pool, chat.NewMemoryBroker())
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(r)
	t.Cleanup(func() { srv.Close(); cancel() })
	return &testAPI{t: t, srv: srv}
}
```

Append to `app_test.go`:

```go
func TestCORSPreflight(t *testing.T) {
	a := newAPI(t)
	host := a.createRoom()
	preflight := func(origin string) *http.Response {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodOptions,
			a.srv.URL+"/rooms/"+host.RoomID+"/messages", nil)
		req.Header.Set("Origin", origin)
		req.Header.Set("Access-Control-Request-Method", "GET")
		req.Header.Set("Access-Control-Request-Headers", "authorization")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		return res
	}
	if res := preflight(testOrigin); res.StatusCode != 204 || res.Header.Get("Access-Control-Allow-Origin") != testOrigin {
		t.Fatalf("allowed preflight = %d %q", res.StatusCode, res.Header.Get("Access-Control-Allow-Origin"))
	}
	if res := preflight("http://evil.test"); res.StatusCode != 403 || res.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("rejected preflight = %d %q", res.StatusCode, res.Header.Get("Access-Control-Allow-Origin"))
	}
}
```

- [ ] **Step 10: Run the whole backend suite**

Run: `gofmt -l . && go vet ./... && go test -count=1 ./...`
Expected: no gofmt output; all PASS (including `TestCORSPreflight` with `TEST_DATABASE_URL` set).

- [ ] **Step 11: Commit**

```bash
git add backend/internal/platform/config backend/internal/platform/httpx/cors.go backend/internal/platform/httpx/cors_test.go backend/internal/app/app.go backend/internal/app/app_test.go
git commit -m "feat(backend): add service config and CORS for external origins"
```

---

### Task 2: Service-room data model (migration, room repository, labels)

**Files:**
- Create: `backend/internal/platform/db/migrations/000006_service_rooms.up.sql`
- Create: `backend/internal/platform/db/migrations/000006_service_rooms.down.sql`
- Modify: `backend/internal/apperr/apperr.go` (add `MemberRemoved`)
- Modify: `backend/internal/platform/httpx/httpx.go` (map `member_removed` → 409)
- Modify: `backend/internal/room/repository.go`
- Modify: `backend/internal/room/service.go`
- Modify: `backend/internal/chat/message.go`, `backend/internal/chat/repository.go`, `backend/internal/chat/service.go` (sender `label`)
- Modify: `docs/openapi.yaml` (`label` on `Member` and `Message`, `member_removed` in `ErrorCode`)
- Create: `backend/internal/app/service_rooms_test.go`

**Interfaces:**
- Consumes: nothing from Task 1 beyond the test helpers.
- Produces:
  - `room.Room.ExternalRef *string`
  - `room.Member.ExternalUserID *string`, `room.Member.Label string`
  - `room.MemberView.Label string` (JSON `label`); `chat.Message.Label string` (JSON `label`)
  - `apperr.MemberRemoved` (code `member_removed`, HTTP 409)
  - `func (s *room.Service) EnsureByRef(ctx context.Context, ref, name string) (room.Room, error)`
  - `func (s *room.Service) GetByRef(ctx context.Context, ref string) (room.Room, error)` — `apperr.NotFound` if missing
  - `func (s *room.Service) JoinExternal(ctx context.Context, roomID uuid.UUID, externalUserID, displayName, label string) (room.Member, error)` — `apperr.MemberRemoved` if that user was kicked/banned
  - `func (s *room.Service) Member(ctx context.Context, roomID, memberID uuid.UUID) (room.Member, error)` — any member, removed or not
  - Constants `room.MaxServiceNameLen = 64`, `room.MaxLabelLen = 40`

- [ ] **Step 1: Write the migration**

`000006_service_rooms.up.sql`:

```sql
-- Rooms created by a trusted backend through /service/v1, e.g. one per claims
-- ticket channel. NULL = a normal room joined through invite links.
ALTER TABLE rooms ADD COLUMN external_ref TEXT UNIQUE;

-- Members of service rooms are keyed by the caller's own user id, so the same
-- person always maps to the same member. label is a role shown beside the name.
ALTER TABLE room_members
  ADD COLUMN external_user_id TEXT,
  ADD COLUMN label TEXT NOT NULL DEFAULT '',
  ADD CONSTRAINT room_members_room_external_user_key UNIQUE (room_id, external_user_id);
```

`000006_service_rooms.down.sql`:

```sql
ALTER TABLE room_members
  DROP CONSTRAINT room_members_room_external_user_key,
  DROP COLUMN label,
  DROP COLUMN external_user_id;

ALTER TABLE rooms DROP COLUMN external_ref;
```

- [ ] **Step 2: Write the failing integration tests**

`backend/internal/app/service_rooms_test.go`:

```go
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
```

- [ ] **Step 3: Run to verify they fail**

Run: `go test -count=1 ./internal/app/ -run 'TestEnsureByRef|TestJoinExternal'`
Expected: FAIL — compile errors (`svc.EnsureByRef undefined`, `apperr.MemberRemoved undefined`, …).

- [ ] **Step 4: Add the error code**

In `apperr.go` add to the `var` block:

```go
	MemberRemoved   = &Error{"member_removed", "this user was removed from the room"}
```

In `httpx.go` add to `statusByCode`:

```go
	apperr.MemberRemoved.Code:   http.StatusConflict,
```

- [ ] **Step 5: Extend the room repository**

In `room/repository.go`:

Add to `Room` (after `Status`): `ExternalRef *string // set for rooms created through /service/v1`.
Add to `Member` (after `Role`): `ExternalUserID *string` and `Label string`.

Replace `Get` and add the new room queries:

```go
const roomCols = `id, name, owner_token_hash, status, external_ref, created_at`

func scanRoom(row pgx.Row) (Room, error) {
	var rm Room
	err := row.Scan(&rm.ID, &rm.Name, &rm.OwnerTokenHash, &rm.Status, &rm.ExternalRef, &rm.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Room{}, apperr.NotFound
	}
	if err != nil {
		return Room{}, fmt.Errorf("get room: %w", err)
	}
	return rm, nil
}

func (r *Repository) Get(ctx context.Context, id uuid.UUID) (Room, error) {
	return scanRoom(r.db.QueryRow(ctx, `SELECT `+roomCols+` FROM rooms WHERE id = $1`, id))
}

func (r *Repository) GetByRef(ctx context.Context, ref string) (Room, error) {
	return scanRoom(r.db.QueryRow(ctx, `SELECT `+roomCols+` FROM rooms WHERE external_ref = $1`, ref))
}

// EnsureByRef creates the room for ref unless it exists, then returns it.
// Concurrent callers with the same ref all get the same room.
func (r *Repository) EnsureByRef(ctx context.Context, rm Room, ref string) (Room, error) {
	if _, err := r.db.Exec(ctx,
		`INSERT INTO rooms (id, name, owner_token_hash, status, external_ref) VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (external_ref) DO NOTHING`,
		rm.ID, rm.Name, rm.OwnerTokenHash, StatusActive, ref); err != nil {
		return Room{}, fmt.Errorf("ensure room: %w", err)
	}
	return r.GetByRef(ctx, ref)
}
```

Update member columns and scan:

```go
const memberCols = `id, room_id, display_name, role, joined_at, kicked_at, banned_at, external_user_id, label`

func scanMember(row pgx.Row) (Member, error) {
	var m Member
	err := row.Scan(&m.ID, &m.RoomID, &m.DisplayName, &m.Role, &m.JoinedAt, &m.KickedAt, &m.BannedAt,
		&m.ExternalUserID, &m.Label)
	return m, err
}
```

Add the upsert:

```go
// UpsertExternalMember adds an external user to the room, or refreshes the
// name and label of the member they already are. It never clears kicked_at or
// banned_at, so the caller must check the returned member.
func (r *Repository) UpsertExternalMember(ctx context.Context, m Member) (Member, error) {
	got, err := scanMember(r.db.QueryRow(ctx,
		`INSERT INTO room_members (id, room_id, display_name, role, external_user_id, label)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (room_id, external_user_id) DO UPDATE
		   SET display_name = EXCLUDED.display_name, label = EXCLUDED.label
		 RETURNING `+memberCols,
		m.ID, m.RoomID, m.DisplayName, m.Role, m.ExternalUserID, m.Label))
	if err != nil {
		return Member{}, fmt.Errorf("upsert member: %w", err)
	}
	return got, nil
}
```

- [ ] **Step 6: Extend the room service**

In `room/service.go` add:

```go
const (
	// MaxServiceNameLen is longer than the 32 of invite rooms because names
	// come from another system's user records (full Thai names), not a text box.
	MaxServiceNameLen = 64
	MaxLabelLen       = 40
)

// EnsureByRef returns the service room for ref, creating it on first use.
// Nobody holds its owner token, so a service room never has an owner session.
func (s *Service) EnsureByRef(ctx context.Context, ref, name string) (Room, error) {
	name, err := CleanName(name, 80, "name")
	if err != nil {
		return Room{}, err
	}
	rm := Room{ID: uuid.New(), Name: name, OwnerTokenHash: auth.HashToken(auth.NewToken())}
	return s.repo.EnsureByRef(ctx, rm, ref)
}

func (s *Service) GetByRef(ctx context.Context, ref string) (Room, error) {
	return s.repo.GetByRef(ctx, ref)
}

// JoinExternal makes the external user a member of a service room, keeping
// their member id across calls. Removed members stay removed.
func (s *Service) JoinExternal(ctx context.Context, roomID uuid.UUID, externalUserID, displayName, label string) (Member, error) {
	name, err := CleanName(displayName, MaxServiceNameLen, "display_name")
	if err != nil {
		return Member{}, err
	}
	label = strings.Join(strings.Fields(label), " ")
	if utf8.RuneCountInString(label) > MaxLabelLen {
		return Member{}, apperr.Invalid(fmt.Sprintf("label must be at most %d characters", MaxLabelLen))
	}
	m, err := s.repo.UpsertExternalMember(ctx, Member{
		ID: uuid.New(), RoomID: roomID, DisplayName: name, Role: RoleMember,
		ExternalUserID: &externalUserID, Label: label,
	})
	if err != nil {
		return Member{}, fmt.Errorf("join external: %w", err)
	}
	if m.KickedAt != nil || m.BannedAt != nil {
		return Member{}, apperr.MemberRemoved
	}
	return m, nil
}

// Member loads a member whether or not they are still active.
func (s *Service) Member(ctx context.Context, roomID, memberID uuid.UUID) (Member, error) {
	return s.repo.GetMember(ctx, roomID, memberID)
}
```

Add `Label string \`json:"label"\`` to `MemberView` (after `Role`) and set `Label: m.Label` in `Members`.

- [ ] **Step 7: Carry the label on messages**

`chat/message.go` — add to `Message` after `DisplayName`:

```go
	Label       string    `json:"label"` // sender's role in service rooms, e.g. "เจ้าหน้าที่ตรวจสอบ"; "" otherwise
```

`chat/repository.go` — `messageSelect` selects `rm.label` right after `rm.display_name`, and `scanMessage` scans into `&m.Label` right after `&m.DisplayName`:

```go
const messageSelect = `SELECT m.id, m.room_id, m.member_id, rm.display_name, rm.label, m.client_msg_id, m.body, m.created_at,
		i.id, i.content_type, i.width, i.height
	FROM messages m
	JOIN room_members rm ON rm.id = m.member_id
	LEFT JOIN images i ON i.id = m.image_id `
```

```go
	err := row.Scan(&m.ID, &m.RoomID, &m.MemberID, &m.DisplayName, &m.Label, &m.ClientMsgID, &m.Body, &m.CreatedAt,
		&imgID, &imgType, &imgW, &imgH)
```

`chat/service.go` — in `Send`, add `Label: member.Label,` after `DisplayName: member.DisplayName,`.

- [ ] **Step 8: Document the new fields**

In `docs/openapi.yaml`:
- `ErrorCode.enum`: add `- member_removed` after `- kicked`.
- `Member`: `required: [id, display_name, label, role, joined_at, online]` and add property

```yaml
        label:
          type: string
          maxLength: 40
          description: Role shown beside the name in service rooms (e.g. `เจ้าหน้าที่ตรวจสอบ`); empty in normal rooms.
```

- `Message`: add `label` to `required` after `display_name` and the property

```yaml
        label:
          type: string
          description: Sender's role label; see `Member.label`.
```

- [ ] **Step 9: Run tests**

Run: `gofmt -l . && go vet ./... && go test -count=1 ./...`
Expected: all PASS; the migration applies on the test DB; existing chat tests still pass (label is `""` for normal rooms).

- [ ] **Step 10: Commit**

```bash
git add backend/internal/platform/db/migrations/000006_service_rooms.*.sql backend/internal/apperr backend/internal/platform/httpx/httpx.go backend/internal/room backend/internal/chat backend/internal/app/service_rooms_test.go docs/openapi.yaml
git commit -m "feat(backend): add external refs, external members and labels"
```

---

### Task 3: `/service/v1` rooms, sessions and close

**Files:**
- Modify: `backend/internal/auth/jwt.go` (`IssueFor`)
- Create: `backend/internal/auth/jwt_test.go`
- Create: `backend/internal/service/service.go`
- Create: `backend/internal/service/handler.go`
- Create: `backend/internal/service/service_test.go` (unit)
- Modify: `backend/internal/app/app.go` (wire)
- Modify: `backend/internal/app/app_test.go` (`session` gets `ExpiresAt`)
- Create: `backend/internal/app/service_api_test.go`
- Modify: `docs/openapi.yaml` (3 paths, `service` tag, `serviceKey` scheme, schemas)

**Interfaces:**
- Consumes (Task 2): `room.Service.EnsureByRef`, `GetByRef`, `JoinExternal`, `Close`; `room.StatusClosed`, `room.RoleMember`; `apperr.MemberRemoved`. (Task 1): `config.Config.ServiceAPIKey`, `ServiceJWTTTL`; test helpers `newAPIWith`, `testServiceKey`, `testRef` (from `service_rooms_test.go`).
- Produces:
  - `func (i *auth.Issuer) IssueFor(c auth.Claims, ttl time.Duration) (auth.Session, error)`
  - `service.RequireKey(key string) gin.HandlerFunc`
  - `service.ValidRef(ref string) error`
  - `service.NewService(rooms *room.Service, issuer *auth.Issuer, ttl time.Duration) *service.Service` (Task 4 adds a `*media.Service` parameter)
  - `service.RoomState{RoomID uuid.UUID "room_id"; Status string "status"}`
  - `func (s *service.Service) EnsureRoom(ctx, ref, name string) (RoomState, error)`
  - `func (s *service.Service) Session(ctx, ref, externalUserID, displayName, label string) (auth.Session, error)`
  - `func (s *service.Service) Close(ctx, ref string) error`
  - `service.NewHandler(svc *service.Service, key string) *service.Handler` with `Register(r gin.IRouter)`; routes group `/service/v1/rooms/:ref`

- [ ] **Step 1: Write the failing JWT TTL test**

`backend/internal/auth/jwt_test.go`:

```go
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
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/auth/`
Expected: FAIL — `i.IssueFor undefined`.

- [ ] **Step 3: Implement `IssueFor`**

In `jwt.go` replace `Issue` with:

```go
func (i *Issuer) Issue(c Claims) (Session, error) { return i.IssueFor(c, i.ttl) }

// IssueFor is Issue with a lifetime other than the issuer's default.
func (i *Issuer) IssueFor(c Claims, ttl time.Duration) (Session, error) {
	now := i.now().UTC()
	exp := now.Add(ttl)
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
```

Run: `go test ./internal/auth/` → PASS.

- [ ] **Step 4: Write the failing service unit tests**

`backend/internal/service/service_test.go`:

```go
package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRequireKey(t *testing.T) {
	const key = "0123456789abcdef0123456789abcdef"
	cases := []struct {
		name, configured, header string
		want                     int
	}{
		{"disabled", "", "Bearer " + key, http.StatusNotFound},
		{"missing", key, "", http.StatusUnauthorized},
		{"wrong", key, "Bearer " + strings.Repeat("x", 32), http.StatusUnauthorized},
		{"not bearer", key, key, http.StatusUnauthorized},
		{"correct", key, "Bearer " + key, http.StatusOK},
	}
	gin.SetMode(gin.TestMode)
	for _, tc := range cases {
		r := gin.New()
		r.GET("/x", RequireKey(tc.configured), func(c *gin.Context) { c.Status(http.StatusOK) })
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		if tc.header != "" {
			req.Header.Set("Authorization", tc.header)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != tc.want {
			t.Errorf("%s: status %d, want %d", tc.name, w.Code, tc.want)
		}
	}
}

func TestValidRef(t *testing.T) {
	good := []string{"claims:ticket:123:public", "a", "x_y-z:0", strings.Repeat("a", 128)}
	bad := []string{"", "Claims:ticket", "ticket 1", "ticket/1", "ตั๋ว", strings.Repeat("a", 129)}
	for _, r := range good {
		if err := ValidRef(r); err != nil {
			t.Errorf("ValidRef(%q) = %v", r, err)
		}
	}
	for _, r := range bad {
		if err := ValidRef(r); err == nil {
			t.Errorf("ValidRef(%q) accepted", r)
		}
	}
}
```

Run: `go test ./internal/service/` → FAIL (package does not exist / undefined).

- [ ] **Step 5: Implement the service**

`backend/internal/service/service.go`:

```go
// Package service is the API for trusted backends, such as the medical claims
// system, that decide who may chat themselves and ask smalltalk for rooms and
// member sessions on their users' behalf. It is served under /service/v1 and
// must only be reachable from the internal network.
package service

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/google/uuid"

	"smalltalk/internal/apperr"
	"smalltalk/internal/auth"
	"smalltalk/internal/room"
)

var (
	refPattern    = regexp.MustCompile(`^[a-z0-9:_-]{1,128}$`)
	userIDPattern = regexp.MustCompile(`^[A-Za-z0-9:_-]{1,128}$`)
)

// ValidRef checks a caller's room reference, e.g. claims:ticket:123:public.
func ValidRef(ref string) error {
	if !refPattern.MatchString(ref) {
		return apperr.Invalid("ref must match ^[a-z0-9:_-]{1,128}$")
	}
	return nil
}

type Service struct {
	rooms  *room.Service
	issuer *auth.Issuer
	ttl    time.Duration
}

func NewService(rooms *room.Service, issuer *auth.Issuer, ttl time.Duration) *Service {
	return &Service{rooms: rooms, issuer: issuer, ttl: ttl}
}

type RoomState struct {
	RoomID uuid.UUID `json:"room_id"`
	Status string    `json:"status"`
}

// EnsureRoom creates the room for ref on first use; later calls return it
// unchanged, keeping its original name.
func (s *Service) EnsureRoom(ctx context.Context, ref, name string) (RoomState, error) {
	if err := ValidRef(ref); err != nil {
		return RoomState{}, err
	}
	rm, err := s.rooms.EnsureByRef(ctx, ref, name)
	if err != nil {
		return RoomState{}, fmt.Errorf("ensure room: %w", err)
	}
	return RoomState{RoomID: rm.ID, Status: rm.Status}, nil
}

// Session issues a member JWT for an external user. It works on closed rooms
// too so history stays readable; posting is refused by room.CanPost.
func (s *Service) Session(ctx context.Context, ref, externalUserID, displayName, label string) (auth.Session, error) {
	if err := ValidRef(ref); err != nil {
		return auth.Session{}, err
	}
	if !userIDPattern.MatchString(externalUserID) {
		return auth.Session{}, apperr.Invalid("external_user_id must match ^[A-Za-z0-9:_-]{1,128}$")
	}
	rm, err := s.rooms.GetByRef(ctx, ref)
	if err != nil {
		return auth.Session{}, fmt.Errorf("service session: %w", err)
	}
	m, err := s.rooms.JoinExternal(ctx, rm.ID, externalUserID, displayName, label)
	if err != nil {
		return auth.Session{}, fmt.Errorf("service session: %w", err)
	}
	return s.issuer.IssueFor(auth.Claims{RoomID: rm.ID, MemberID: m.ID, Role: room.RoleMember}, s.ttl)
}

// Close closes the room and drops its live connections. Closing a closed room
// does nothing.
func (s *Service) Close(ctx context.Context, ref string) error {
	if err := ValidRef(ref); err != nil {
		return err
	}
	rm, err := s.rooms.GetByRef(ctx, ref)
	if err != nil {
		return fmt.Errorf("close service room: %w", err)
	}
	if rm.Status == room.StatusClosed {
		return nil
	}
	return s.rooms.Close(ctx, rm.ID)
}
```

`backend/internal/service/handler.go`:

```go
package service

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"smalltalk/internal/apperr"
	"smalltalk/internal/auth"
	"smalltalk/internal/platform/httpx"
)

type Handler struct {
	svc *Service
	key string
}

func NewHandler(svc *Service, key string) *Handler {
	return &Handler{svc: svc, key: key}
}

func (h *Handler) Register(r gin.IRouter) {
	g := r.Group("/service/v1/rooms/:ref", RequireKey(h.key))
	g.PUT("", h.ensureRoom)
	g.POST("/sessions", h.session)
	g.POST("/close", h.close)
}

// RequireKey checks the shared service key in constant time. An empty key
// turns the API off: every route answers 404 as if it did not exist.
func RequireKey(key string) gin.HandlerFunc {
	hash := auth.HashToken(key)
	return func(c *gin.Context) {
		if key == "" {
			httpx.Error(c, apperr.NotFound)
			return
		}
		token, ok := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer ")
		if !ok || !auth.TokenMatches(token, hash) {
			httpx.Error(c, apperr.Unauthorized)
			return
		}
		c.Next()
	}
}

func (h *Handler) ensureRoom(c *gin.Context) {
	var req struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, apperr.Invalid("invalid JSON body"))
		return
	}
	st, err := h.svc.EnsureRoom(c.Request.Context(), c.Param("ref"), req.Name)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, st)
}

func (h *Handler) session(c *gin.Context) {
	var req struct {
		ExternalUserID string `json:"external_user_id"`
		DisplayName    string `json:"display_name"`
		Label          string `json:"label"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, apperr.Invalid("invalid JSON body"))
		return
	}
	sess, err := h.svc.Session(c.Request.Context(), c.Param("ref"), req.ExternalUserID, req.DisplayName, req.Label)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, sess)
}

func (h *Handler) close(c *gin.Context) {
	if err := h.svc.Close(c.Request.Context(), c.Param("ref")); err != nil {
		httpx.Error(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
```

Run: `go test ./internal/service/` → PASS.

- [ ] **Step 6: Wire into the router**

In `app.go` import `"smalltalk/internal/service"`; after `chatSvc := ...` add:

```go
	serviceSvc := service.NewService(roomSvc, issuer, cfg.ServiceJWTTTL)
```

and after `chat.NewHandler(...).Register(r)`:

```go
	service.NewHandler(serviceSvc, cfg.ServiceAPIKey).Register(r)
```

- [ ] **Step 7: Write the failing integration tests**

In `app_test.go` add `ExpiresAt time.Time \`json:"expires_at"\`` and `Role string \`json:"role"\`` to the `session` struct.

`backend/internal/app/service_api_test.go`:

```go
package app

import (
	"context"
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
```

Run: `go test -count=1 ./internal/app/ -run TestService`
Expected: PASS for the new tests once Steps 5–6 are in (if written before Step 5/6, they fail to compile or get 404s). `go test ./cmd/server/` now FAILS: `route PUT /service/v1/rooms/{ref} is served but missing from docs/openapi.yaml` — fixed next.

- [ ] **Step 8: Document the service API in OpenAPI**

In `docs/openapi.yaml`:

1. `info.description`: append a paragraph:

```yaml
    **Service API** (`/service/v1`) is for trusted backends on the internal network, e.g. the medical
    claims system. They check their own users' permissions, then create a room per external
    reference and ask for member sessions on their users' behalf. It uses the `serviceKey` scheme,
    not room JWTs, and is disabled (404) unless `SERVICE_API_KEY` is set. Sessions it issues last
    `SERVICE_JWT_TTL` (default 15 minutes) instead of 12 hours.
```

2. `tags`: add

```yaml
  - name: service
    description: Server-to-server API for trusted backends (internal network only)
```

3. Under `paths`, add (before `components`):

```yaml
  /service/v1/rooms/{ref}:
    parameters:
      - $ref: "#/components/parameters/ServiceRef"
    put:
      tags: [service]
      operationId: ensureServiceRoom
      summary: Create the room for an external reference if it does not exist
      description: Idempotent. A later call returns the same room and keeps its original name.
      security:
        - serviceKey: []
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: "#/components/schemas/EnsureServiceRoomRequest"
            example:
              name: "เรื่อง #123"
      responses:
        "200":
          description: The room for this reference.
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/ServiceRoom"
        "400":
          $ref: "#/components/responses/InvalidInput"
        "401":
          $ref: "#/components/responses/ServiceUnauthorized"
        "404":
          $ref: "#/components/responses/NotFound"

  /service/v1/rooms/{ref}/sessions:
    parameters:
      - $ref: "#/components/parameters/ServiceRef"
    post:
      tags: [service]
      operationId: createServiceSession
      summary: Get a member session for an external user
      description: |
        Adds the user as a member (role `member`) on first call; later calls return the same
        `member_id` and refresh `display_name` and `label`. Works on closed rooms so history stays
        readable; sending is refused there. The caller hands the returned JWT to the user's browser.
      security:
        - serviceKey: []
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: "#/components/schemas/ServiceSessionRequest"
            example:
              external_user_id: "user:42"
              display_name: สมชาย ใจดี
              label: ผู้ยื่น
      responses:
        "200":
          description: Session for this user in this room.
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/Session"
        "400":
          $ref: "#/components/responses/InvalidInput"
        "401":
          $ref: "#/components/responses/ServiceUnauthorized"
        "404":
          $ref: "#/components/responses/NotFound"
        "409":
          description: "`member_removed`: this user was kicked or banned from the room."
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/Error"

  /service/v1/rooms/{ref}/close:
    parameters:
      - $ref: "#/components/parameters/ServiceRef"
    post:
      tags: [service]
      operationId: closeServiceRoom
      summary: Close the room
      description: Idempotent. Live connections receive `room_closed` and close with 4004.
      security:
        - serviceKey: []
      responses:
        "204":
          description: The room is closed.
        "400":
          $ref: "#/components/responses/InvalidInput"
        "401":
          $ref: "#/components/responses/ServiceUnauthorized"
        "404":
          $ref: "#/components/responses/NotFound"
```

4. `components.securitySchemes`: add

```yaml
    serviceKey:
      type: http
      scheme: bearer
      description: The shared `SERVICE_API_KEY` (at least 32 characters). Only for `/service/v1`.
```

5. `components.parameters`: add

```yaml
    ServiceRef:
      name: ref
      in: path
      required: true
      description: The caller's own id for the room, e.g. `claims:ticket:123:public`.
      schema:
        type: string
        pattern: "^[a-z0-9:_-]{1,128}$"
```

6. `components.responses`: add

```yaml
    ServiceUnauthorized:
      description: "`unauthorized`: missing or wrong service key."
      content:
        application/json:
          schema:
            $ref: "#/components/schemas/Error"
          example:
            error: { code: unauthorized, message: missing or invalid credentials }
```

7. `components.schemas`: add

```yaml
    EnsureServiceRoomRequest:
      type: object
      required: [name]
      properties:
        name:
          type: string
          minLength: 1
          maxLength: 80

    ServiceRoom:
      type: object
      required: [room_id, status]
      properties:
        room_id:
          type: string
          format: uuid
        status:
          type: string
          enum: [active, closed]

    ServiceSessionRequest:
      type: object
      required: [external_user_id, display_name]
      properties:
        external_user_id:
          type: string
          pattern: "^[A-Za-z0-9:_-]{1,128}$"
          description: The caller's id for this user; the same id always maps to the same member.
        display_name:
          type: string
          minLength: 1
          maxLength: 64
        label:
          type: string
          maxLength: 40
          description: Role shown beside the name, e.g. `เจ้าหน้าที่ตรวจสอบ`. Optional.
```

- [ ] **Step 9: Run everything**

Run: `gofmt -l . && go vet ./... && go test -count=1 ./...`
Expected: all PASS, including `TestOpenAPIMatchesRouter`.

- [ ] **Step 10: Commit**

```bash
git add backend/internal/auth backend/internal/service backend/internal/app docs/openapi.yaml
git commit -m "feat(backend): add /service/v1 rooms, sessions and close"
```

---

### Task 4: `/service/v1` sent-image download

**Files:**
- Modify: `backend/internal/media/repository.go` (`GetSent`)
- Modify: `backend/internal/media/service.go` (`OpenSent`)
- Modify: `backend/internal/service/service.go` (`SentImage`, `NewService` signature)
- Modify: `backend/internal/service/handler.go` (route)
- Modify: `backend/internal/app/app.go` (pass `mediaSvc`)
- Modify: `backend/internal/app/service_api_test.go` (test)
- Modify: `docs/openapi.yaml` (path)

**Interfaces:**
- Consumes (Task 2): `room.Service.GetByRef`, `room.Service.Member`, `room.Member.ExternalUserID`. (Task 3): `service.Service`, `service.Handler`, `ValidRef`, test helpers `ensureServiceRoom`, `serviceSession`. Existing: `testAPI.upload`, `testAPI.getImage`, `pngBytes`.
- Produces:
  - `func (r *media.Repository) GetSent(ctx, roomID, id uuid.UUID) (media.Image, error)`
  - `func (s *media.Service) OpenSent(ctx, roomID, id uuid.UUID) (media.Image, io.ReadCloser, error)`
  - `service.NewService(rooms *room.Service, images *media.Service, issuer *auth.Issuer, ttl time.Duration) *service.Service` (**signature change**)
  - `func (s *service.Service) SentImage(ctx, ref string, imageID uuid.UUID) (media.Image, io.ReadCloser, string, error)` — third return is the sender's `external_user_id`
  - Route `GET /service/v1/rooms/{ref}/images/{imageId}` → bytes, `Content-Type`, `X-Sender-External-Id`

- [ ] **Step 1: Write the failing integration test**

Append to `service_api_test.go` (add imports `"bytes"`, `"io"`, `"net/http"`):

```go
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
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test -count=1 ./internal/app/ -run TestServiceSentImage`
Expected: FAIL — `sent image = 404` (route missing).

- [ ] **Step 3: Implement media lookups**

In `media/repository.go`, replace `Get` with a shared query and add `GetSent`:

```go
const imageSelect = `SELECT id, room_id, member_id, content_type, size_bytes, width, height, storage_key, created_at
	FROM images WHERE room_id = $1 AND id = $2`

func (r *Repository) Get(ctx context.Context, roomID, id uuid.UUID) (Image, error) {
	return r.get(ctx, imageSelect, roomID, id)
}

// GetSent is Get restricted to images that a message in the room carries.
func (r *Repository) GetSent(ctx context.Context, roomID, id uuid.UUID) (Image, error) {
	return r.get(ctx, imageSelect+` AND EXISTS (SELECT 1 FROM messages m WHERE m.image_id = images.id)`, roomID, id)
}

func (r *Repository) get(ctx context.Context, sql string, roomID, id uuid.UUID) (Image, error) {
	var img Image
	err := r.db.QueryRow(ctx, sql, roomID, id).
		Scan(&img.ID, &img.RoomID, &img.MemberID, &img.ContentType, &img.SizeBytes,
			&img.Width, &img.Height, &img.StorageKey, &img.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Image{}, apperr.NotFound
	}
	if err != nil {
		return Image{}, fmt.Errorf("get image: %w", err)
	}
	return img, nil
}
```

In `media/service.go`, replace `Open` with:

```go
func (s *Service) Open(ctx context.Context, roomID, id uuid.UUID) (Image, io.ReadCloser, error) {
	img, err := s.repo.Get(ctx, roomID, id)
	return s.open(ctx, img, err)
}

// OpenSent is Open for images already sent in a message; uploads still
// waiting for their message are reported as not found.
func (s *Service) OpenSent(ctx context.Context, roomID, id uuid.UUID) (Image, io.ReadCloser, error) {
	img, err := s.repo.GetSent(ctx, roomID, id)
	return s.open(ctx, img, err)
}

func (s *Service) open(ctx context.Context, img Image, err error) (Image, io.ReadCloser, error) {
	if err != nil {
		return Image{}, nil, fmt.Errorf("open image: %w", err)
	}
	rc, err := s.store.Open(ctx, img.StorageKey)
	if err != nil {
		return Image{}, nil, fmt.Errorf("open image: %w", err)
	}
	return img, rc, nil
}
```

- [ ] **Step 4: Implement `SentImage` and the route**

In `service/service.go`: import `"io"` and `"smalltalk/internal/media"`; add `images *media.Service` to `Service`; change the constructor:

```go
func NewService(rooms *room.Service, images *media.Service, issuer *auth.Issuer, ttl time.Duration) *Service {
	return &Service{rooms: rooms, images: images, issuer: issuer, ttl: ttl}
}
```

and add:

```go
// SentImage opens an image sent in the room and reports who sent it, so the
// caller can apply its own rules (e.g. only the claimant's receipts count).
func (s *Service) SentImage(ctx context.Context, ref string, imageID uuid.UUID) (media.Image, io.ReadCloser, string, error) {
	if err := ValidRef(ref); err != nil {
		return media.Image{}, nil, "", err
	}
	rm, err := s.rooms.GetByRef(ctx, ref)
	if err != nil {
		return media.Image{}, nil, "", fmt.Errorf("sent image: %w", err)
	}
	img, rc, err := s.images.OpenSent(ctx, rm.ID, imageID)
	if err != nil {
		return media.Image{}, nil, "", fmt.Errorf("sent image: %w", err)
	}
	sender, err := s.rooms.Member(ctx, rm.ID, img.MemberID)
	if err != nil {
		_ = rc.Close()
		return media.Image{}, nil, "", fmt.Errorf("sent image: %w", err)
	}
	var senderID string
	if sender.ExternalUserID != nil {
		senderID = *sender.ExternalUserID
	}
	return img, rc, senderID, nil
}
```

In `service/handler.go` import `"io"`, `"strconv"`, `"github.com/google/uuid"`; register `g.GET("/images/:imageId", h.image)` and add:

```go
func (h *Handler) image(c *gin.Context) {
	id, err := uuid.Parse(c.Param("imageId"))
	if err != nil {
		httpx.Error(c, apperr.NotFound)
		return
	}
	img, rc, sender, err := h.svc.SentImage(c.Request.Context(), c.Param("ref"), id)
	if err != nil {
		httpx.Error(c, err)
		return
	}
	defer func() { _ = rc.Close() }()

	hdr := c.Writer.Header()
	hdr.Set("Content-Type", img.ContentType)
	hdr.Set("Content-Length", strconv.Itoa(img.SizeBytes))
	hdr.Set("X-Sender-External-Id", sender)
	hdr.Set("X-Content-Type-Options", "nosniff")
	hdr.Set("Cache-Control", "no-store")
	c.Status(http.StatusOK)
	_, _ = io.Copy(c.Writer, rc)
}
```

In `app.go` change the construction to `service.NewService(roomSvc, mediaSvc, issuer, cfg.ServiceJWTTTL)`.

- [ ] **Step 5: Document the route**

Add to `docs/openapi.yaml` `paths` after `/service/v1/rooms/{ref}/close`:

```yaml
  /service/v1/rooms/{ref}/images/{imageId}:
    parameters:
      - $ref: "#/components/parameters/ServiceRef"
      - name: imageId
        in: path
        required: true
        schema:
          type: string
          format: uuid
    get:
      tags: [service]
      operationId: getServiceImage
      summary: Download an image sent in the room
      description: |
        Only images already carried by a message in this room; uploads not yet sent, and images of
        other rooms, are `404`. `X-Sender-External-Id` tells the caller who sent it.
      security:
        - serviceKey: []
      responses:
        "200":
          description: The image bytes.
          headers:
            X-Sender-External-Id:
              description: "`external_user_id` of the sender; empty if the sender is not an external user."
              schema:
                type: string
          content:
            image/jpeg:
              schema:
                type: string
                contentMediaType: image/jpeg
            image/png:
              schema:
                type: string
                contentMediaType: image/png
            image/gif:
              schema:
                type: string
                contentMediaType: image/gif
        "400":
          $ref: "#/components/responses/InvalidInput"
        "401":
          $ref: "#/components/responses/ServiceUnauthorized"
        "404":
          $ref: "#/components/responses/NotFound"
```

- [ ] **Step 6: Run everything**

Run: `gofmt -l . && go vet ./... && go test -count=1 ./...`
Expected: all PASS, including `TestServiceSentImage`, `TestImages` and `TestOpenAPIMatchesRouter`.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/media backend/internal/service backend/internal/app docs/openapi.yaml
git commit -m "feat(backend): let trusted backends download sent images"
```

---

### Task 5: Refuse invites on service rooms

**Files:**
- Modify: `backend/internal/invite/service.go` (`Create`)
- Modify: `backend/internal/invite/repository.go` (`Join`)
- Modify: `backend/internal/app/service_api_test.go` (test)

**Interfaces:**
- Consumes (Task 2): `room.Room.ExternalRef`. (Task 3): `ensureServiceRoom`, `serviceSession`. Existing: `testAPI.joinErr`, `errBody`.
- Produces: no new API; `POST /rooms/{id}/invites` and `POST /join` answer `404 not_found` for service rooms.

- [ ] **Step 1: Write the failing test**

Append to `service_api_test.go`:

```go
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
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test -count=1 ./internal/app/ -run TestServiceRoomsRejectInvites`
Expected: FAIL — `create invite = 201 ""`.

- [ ] **Step 3: Implement**

In `invite/service.go` `Create`, right after loading `rm`:

```go
	if rm.ExternalRef != nil {
		// Service rooms admit only users their backend vouches for.
		return Created{}, apperr.NotFound
	}
```

In `invite/repository.go` `Join`, change the room lock query and add the check before the closed check:

```go
	var rm room.Room
	if err := tx.QueryRow(ctx,
		`SELECT id, name, status, external_ref, created_at FROM rooms WHERE id = $1 FOR UPDATE`, inv.RoomID).
		Scan(&rm.ID, &rm.Name, &rm.Status, &rm.ExternalRef, &rm.CreatedAt); err != nil {
		return room.Room{}, fmt.Errorf("lock room: %w", err)
	}
	if rm.ExternalRef != nil {
		return room.Room{}, apperr.NotFound
	}
```

- [ ] **Step 4: Run everything**

Run: `gofmt -l . && go vet ./... && go test -count=1 ./...`
Expected: all PASS (existing `TestInviteRules` unaffected).

- [ ] **Step 5: Commit**

```bash
git add backend/internal/invite backend/internal/app/service_api_test.go
git commit -m "feat(backend): refuse invite links on service rooms"
```

---

### Task 6: Deploy config and docs

**Files:**
- Modify: `docker-compose.yml`
- Modify: `deploy/nginx.conf`
- Modify: `README.md`

**Interfaces:**
- Consumes: env vars from Task 1 (`SERVICE_API_KEY`, `SERVICE_JWT_TTL`, `CORS_ORIGINS`) and existing `ALLOWED_ORIGINS`.
- Produces: nothing code-level. Plan 2 (claims fork) relies on: internal URL `http://api:8080` inside this compose network (or the fork's own service name), public browser URL through Nginx `/api` + `/ws`.

- [ ] **Step 1: Block `/service` at Nginx**

In `deploy/nginx.conf`, inside `server`, **before** `location /api/`:

```nginx
  # /service/v1 is for trusted backends on the internal network; they call
  # api:8080 directly. Never expose it through the public entry point.
  location ^~ /api/service/ {
    return 404;
  }
```

- [ ] **Step 2: Pass the new settings in Compose**

In `docker-compose.yml`, under `api.environment` after `APP_BASE_URL`:

```yaml
      # Trusted backends (e.g. the claims system) call /service/v1 with this key
      # (at least 32 characters). Empty turns the service API off.
      SERVICE_API_KEY: ${SERVICE_API_KEY:-}
      SERVICE_JWT_TTL: ${SERVICE_JWT_TTL:-15m}
      # Other web apps that call the API from the browser, e.g. https://claims.example.com.
      # CORS_ORIGINS is for REST, ALLOWED_ORIGINS for the WebSocket handshake.
      CORS_ORIGINS: ${CORS_ORIGINS:-}
      ALLOWED_ORIGINS: ${ALLOWED_ORIGINS:-}
```

- [ ] **Step 3: Verify against the running stack**

This rebuilds the local compose stack (`chat_system-*` containers) with a service key. Run from repo root:

```bash
export SERVICE_API_KEY=local-service-key-0123456789abcdef
docker compose config --quiet
docker compose up -d --build api nginx
# From the public entry point: must be blocked.
curl -s -o /dev/null -w 'public: %{http_code}\n' -X PUT -H "Authorization: Bearer $SERVICE_API_KEY" \
  -H 'Content-Type: application/json' -d '{"name":"x"}' http://localhost:8000/api/service/v1/rooms/test:nginx
curl -s -o /dev/null -w 'encoded: %{http_code}\n' 'http://localhost:8000/api/%73ervice/v1/rooms/test:nginx'
# From inside the compose network: must work.
docker run --rm --network chat_system_default curlimages/curl -s -X PUT \
  -H "Authorization: Bearer $SERVICE_API_KEY" -H 'Content-Type: application/json' \
  -d '{"name":"x"}' http://api:8080/service/v1/rooms/test:nginx
```

Expected: `public: 404`, `encoded: 404`, and the last command prints `{"room_id":"…","status":"active"}`. (If the network name differs, find it with `docker network ls | grep chat_system`.)

- [ ] **Step 4: Document in README**

Add a section to `README.md` after the use-case section:

```markdown
## Service API สำหรับระบบภายนอก

ระบบอื่น (เช่นระบบเบิกค่ารักษาพยาบาล) ใช้ smalltalk เป็น chat service ได้ผ่าน `/service/v1`
ระบบนั้นตรวจสิทธิ์ผู้ใช้เอง แล้วขอห้องและ session แทนผู้ใช้ — browser ของผู้ใช้ต่อ REST/WebSocket ของ smalltalk ตรงด้วย JWT ที่ได้มา

- ห้องผูกกับ `ref` ของระบบภายนอก เช่น `claims:ticket:123:public`; เรียก `PUT` ซ้ำได้ห้องเดิม
- สมาชิกผูกกับ `external_user_id`; ขอ session ซ้ำได้ `member_id` เดิม และมี `label` บอกบทบาท
- ห้องแบบนี้ไม่มีลิงก์เชิญและไม่มี owner
- เรียกได้จากเครือข่ายภายในเท่านั้น — Nginx ตอบ 404 ให้ `/api/service/`

| ตัวแปร | ค่าเริ่มต้น | ความหมาย |
| --- | --- | --- |
| `SERVICE_API_KEY` | ว่าง (ปิด) | key ที่ระบบภายนอกส่งใน `Authorization: Bearer`, ยาว ≥ 32 ตัวอักษร |
| `SERVICE_JWT_TTL` | `15m` | อายุ JWT ที่ออกผ่าน service API |
| `CORS_ORIGINS` | ว่าง | origin ของเว็บอื่นที่เรียก REST ได้ เช่น `https://claims.example.com` |
| `ALLOWED_ORIGINS` | ว่าง | origin/host ที่เปิด WebSocket ได้ นอกจาก host ของตัวเอง |

รายละเอียด endpoint อยู่ใน [docs/openapi.yaml](docs/openapi.yaml) (tag `service`)
```

- [ ] **Step 5: Lint the OpenAPI file and run the full suite**

Run (repo root): `npx --yes @redocly/cli@latest lint docs/openapi.yaml`
Expected: `Woohoo! Your API description is valid.` (warnings acceptable only if they already existed before this branch — compare with `git stash` if unsure).

Run (`backend/`): `gofmt -l . && go vet ./... && go test -count=1 ./... && (command -v golangci-lint >/dev/null && golangci-lint run ./... || true)`
Expected: all PASS, no lint findings.

- [ ] **Step 6: Commit**

```bash
git add docker-compose.yml deploy/nginx.conf README.md
git commit -m "chore: expose service API settings and block /service at nginx"
```

---

## After this plan

Plan 2 covers the claims fork (`guy127/pea-medical-claims`, branch `feat/smalltalk-chat`): `chat.go` endpoints, attach-from-chat, closing rooms on `paid`/`rejected`, the ported chat frontend, and the fork's compose. Write it once this plan is merged, against the fork's actual code.

# smalltalk

ระบบแชทแบบห้อง: Host สร้างห้องแล้วแชร์ QR ให้คนสแกนเข้ามาคุยได้ทันทีผ่านเว็บ ไม่ต้องลงแอปและไม่ต้องสมัครสมาชิก

- `server/` — Go (Gin + coder/websocket + PostgreSQL)
- `web/` — Next.js 16 (App Router, TypeScript strict)
- `deploy/nginx.conf` — รวมเว็บ, `/api` และ `/ws` ไว้ใต้ origin เดียว

## เริ่มใช้งาน (Docker Compose)

```bash
docker compose up -d --build
```

เปิด http://localhost:8000 แล้วสร้างห้อง ระบบจะออก QR ให้ทันที

**ทดสอบกับมือถือในวง LAN:** เปิดหน้า Host ผ่าน IP ของเครื่อง เช่น `http://192.168.1.10:8000`
เพราะลิงก์ใน QR จะใช้ origin เดียวกับที่ Host เปิดอยู่ (หรือกำหนด `APP_BASE_URL` ให้ชัดเจน)

ตั้งค่าผ่านไฟล์ `.env` ที่ root (ไม่ถูก commit):

| ตัวแปร | ค่าเริ่มต้น | ความหมาย |
| --- | --- | --- |
| `JWT_SECRET` | ค่า dev (ห้ามใช้จริง) | อย่างน้อย 32 ตัวอักษร |
| `POSTGRES_PASSWORD` | `smalltalk` | รหัสผ่านฐานข้อมูล |
| `APP_BASE_URL` | ว่าง | prefix ของลิงก์ใน QR เช่น `https://chat.example.com` |
| `PORT` | `8000` | port ของ Nginx |

Production ต้องมี TLS ด้านหน้า Nginx เพื่อให้ใช้ HTTPS/WSS

## พัฒนาแบบไม่ใช้ Docker

```bash
# Postgres ชั่วคราว
docker run -d --name smalltalk-db -e POSTGRES_PASSWORD=dev -e POSTGRES_DB=smalltalk -p 5432:5432 postgres:17-alpine

# API (migration รันอัตโนมัติตอนเริ่ม)
cd server
DATABASE_URL='postgres://postgres:dev@localhost:5432/smalltalk?sslmode=disable' \
JWT_SECRET='dev-secret-at-least-32-characters-long' \
ALLOWED_ORIGINS='localhost:3000' \
go run ./cmd/server

# Web (proxy /api ไปที่ :8080, WebSocket ต่อ ws://localhost:8080/ws)
cd web && npm install && npm run dev
```

## ทดสอบ

```bash
cd server
go test ./...                       # unit tests
TEST_DATABASE_URL='postgres://postgres:dev@localhost:5432/smalltalk_test?sslmode=disable' \
  go test ./...                     # + integration/WebSocket tests กับ Postgres จริง
golangci-lint run ./...

cd web
npm run lint && npm run format:check && npx tsc --noEmit && npm run build
```

CI (`.gitlab-ci.yml`) รัน lint + test (มี Postgres service) + build ทั้งสองฝั่ง

## API

REST (ผ่าน Nginx อยู่ใต้ `/api`) — JSON snake_case, เวลาเป็น RFC 3339 UTC, error รูปแบบ `{"error": {"code", "message"}}`

| Method | Path | ใคร | หมายเหตุ |
| --- | --- | --- | --- |
| POST | `/rooms` | Host | `{name, display_name}` → `room_id, owner_token, jwt, member_id` |
| POST | `/rooms/{id}/owner-session` | Host | `{owner_token}` → JWT ใหม่ เมื่อ JWT เดิมหมดอายุ |
| GET | `/rooms/{id}` | สมาชิก | ข้อมูลห้อง |
| POST | `/rooms/{id}/invites` | Host | `{expires_in_minutes?, max_uses?}` → `token, invite_url, expires_at` |
| GET | `/rooms/{id}/invites` | Host | invite ที่ยังใช้ได้ |
| DELETE | `/rooms/{id}/invites/{inviteId}` | Host | ยกเลิก QR |
| POST | `/join` | Guest | `{token, display_name}` → `jwt, room_id, member_id` |
| GET | `/rooms/{id}/messages?before={ulid}&limit=50` | สมาชิก | เรียงเก่า→ใหม่ + `next_cursor` |
| GET | `/rooms/{id}/members` | สมาชิก | รายชื่อ + `online` |
| POST | `/rooms/{id}/members/{mid}/kick` | Host | `{ban?: bool}` ตัด connection ทันที |
| POST | `/rooms/{id}/close` | Host | ปิดห้อง |

WebSocket: `/ws?room={id}` ส่ง JWT เป็น subprotocol ตัวที่สอง
`new WebSocket(url, ["chat", jwt])` เพื่อไม่ให้ token ไปอยู่ใน URL/log

- client → server: `send {client_msg_id, body}`, `resume {last_message_id}`, `ping`
- server → client: `message`, `ack {client_msg_id, id}`, `presence {member_id, online}`, `error {code, message, client_msg_id?}`, `kicked {reason}`, `room_closed`, `pong`
- close code 4001 unauthorized, 4003 ถูกเตะ/แบน, 4004 ห้องปิด — client จะไม่ reconnect

## สิ่งที่ต่างจากแผนเล็กน้อย

- `room_members` มีคอลัมน์ `kicked_at` เพิ่ม เพื่อแยก "เตะ" (สแกน QR เข้าใหม่ได้) กับ "แบน"
- `messages.id` ใช้ `COLLATE "C"` ให้ ULID เรียงแบบ byte เสมอไม่ขึ้นกับ locale
- Integration test ใช้ Postgres จาก `TEST_DATABASE_URL` (CI ใช้ service) แทน testcontainers-go
- เพิ่ม `POST /rooms/{id}/owner-session`, `GET /rooms/{id}`, `GET /rooms/{id}/invites` และ frame `pong`
- มี Nginx ตั้งแต่ตอนนี้ เพื่อให้มือถือเข้าผ่าน origin เดียว

## ยังไม่ได้ทำ (Phase 3 / หลัง MVP)

- Redis Pub/Sub + presence ข้าม instance (มี interface `chat.Broker` รอไว้แล้ว)
- Metrics, load test k6 1,000 connections
- ข้อจำกัด: การแบนผูกกับ member ไม่ใช่ตัวบุคคล เพราะไม่มีบัญชีผู้ใช้ คนที่ถูกแบนยังสแกน QR ที่ยังใช้ได้เข้ามาใหม่ได้ — ให้ Host ยกเลิก QR แล้วออกใหม่

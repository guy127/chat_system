# smalltalk

ระบบแชทแบบห้อง: Host สร้างห้องแล้วส่งลิงก์เชิญ (หรือให้สแกน QR) ให้คนเข้ามาคุยได้ทันทีผ่านเว็บ ไม่ต้องลงแอปและไม่ต้องสมัครสมาชิก

- หน้าแรกรวม **ห้องของฉัน** ทุกห้องที่ browser นี้เคยเข้า พร้อมข้อความล่าสุดและจำนวนที่ยังไม่อ่าน
- เข้าห้องด้วยลิงก์เชิญ (คัดลอก / แชร์ / วางลิงก์) หรือ QR
- ส่งข้อความและ **รูปภาพไม่เกิน 10 MB** (JPEG, PNG, GIF; WebP/HEIC ถูกแปลงเป็น JPEG ในเครื่องก่อนส่ง)
- Host เตะ/แบนสมาชิก ยกเลิกลิงก์เชิญ และปิดห้องได้

- `backend/` — Go (Gin + coder/websocket + PostgreSQL)
- `frontend/` — Next.js 16 (App Router, TypeScript strict)
- `deploy/nginx.conf` — รวมเว็บ, `/api` และ `/ws` ไว้ใต้ origin เดียว

## Use case

```mermaid
flowchart LR
    Guest["👤 Guest<br/>(คนที่ได้ลิงก์เชิญ)"]
    Member["👤 Member<br/>(สมาชิกในห้อง)"]
    Host["👤 Host<br/>(เจ้าของห้อง)"]
    Job["⚙️ System<br/>(งานเบื้องหลัง)"]

    subgraph smalltalk["ระบบ smalltalk"]
        direction TB
        UC1(["สร้างห้อง"])
        UC2(["ขอ session ใหม่ด้วย owner token"])
        UC3(["สร้างลิงก์เชิญ / QR"])
        UC4(["ดูลิงก์เชิญที่ยังใช้ได้"])
        UC5(["ยกเลิกลิงก์เชิญ"])
        UC6(["เตะสมาชิก"])
        UC7(["แบนสมาชิก"])
        UC8(["ปิดห้อง"])

        UC10(["เข้าห้องด้วยลิงก์เชิญ / QR"])

        UC20(["ดูรายการห้องของฉัน<br/>(ข้อความล่าสุด + unread)"])
        UC21(["ดูข้อมูลห้อง"])
        UC22(["ดูประวัติข้อความ"])
        UC23(["ส่งข้อความ"])
        UC24(["ส่งรูปภาพ (≤ 10 MB)"])
        UC25(["อัปโหลดรูป<br/>(ย่อรูป + ลบ EXIF)"])
        UC26(["ดูรูปภาพ"])
        UC27(["ดูรายชื่อสมาชิก + สถานะออนไลน์"])
        UC28(["รับข้อความแบบ realtime"])
        UC29(["เชื่อมต่อใหม่แล้วดึงข้อความที่พลาด"])

        UC30(["ลบรูปที่ไม่ถูกส่งภายใน 1 ชม."])
    end

    Guest --> UC10
    Member --> UC20 & UC21 & UC22 & UC23 & UC24 & UC26 & UC27 & UC28
    Host --> UC1 & UC2 & UC3 & UC4 & UC5 & UC6 & UC8
    Job --> UC30

    Guest == "เข้าห้องแล้วเป็น" ==> Member
    Host == "เป็น (generalization)" ==> Member

    UC1 -. "«include»" .-> UC3
    UC24 -. "«include»" .-> UC25
    UC23 -. "«include»" .-> UC28
    UC7 -. "«extend»" .-> UC6
    UC29 -. "«extend»" .-> UC28
```

- **Guest** เข้าห้องด้วยลิงก์เชิญ แล้วกลายเป็น Member
- **Host** ทำได้ทุกอย่างที่ Member ทำได้ (ตอนสร้างห้องได้ `member_id` ด้วย) และจัดการห้องได้
- **System** ลบรูปที่อัปโหลดแล้วแต่ไม่ถูกส่งภายใน 1 ชั่วโมง

## เริ่มใช้งาน (Docker Compose)

```bash
docker compose up -d --build
```

เปิด http://localhost:8000 แล้วสร้างห้อง ระบบจะสร้างลิงก์เชิญให้ทันที

**ทดสอบกับมือถือในวง LAN:** เปิดหน้า Host ผ่าน IP ของเครื่อง เช่น `http://192.168.1.10:8000`
เพราะลิงก์เชิญ/QR จะใช้ origin เดียวกับที่ Host เปิดอยู่ (หรือกำหนด `APP_BASE_URL` ให้ชัดเจน)

รูปภาพเก็บใน Docker volume `media` (ใน container คือ `/data/images`)

ตั้งค่าผ่านไฟล์ `.env` ที่ root (ไม่ถูก commit):

| ตัวแปร | ค่าเริ่มต้น | ความหมาย |
| --- | --- | --- |
| `JWT_SECRET` | ค่า dev (ห้ามใช้จริง) | อย่างน้อย 32 ตัวอักษร |
| `POSTGRES_PASSWORD` | `smalltalk` | รหัสผ่านฐานข้อมูล |
| `APP_BASE_URL` | ว่าง | prefix ของลิงก์เชิญ เช่น `https://chat.example.com` |
| `PORT` | `8000` | port ของ Nginx |
| `MEDIA_DIR` | `/data/images` (ใน Docker) | ที่เก็บไฟล์รูป |

Production ต้องมี TLS ด้านหน้า Nginx เพื่อให้ใช้ HTTPS/WSS

## พัฒนาแบบไม่ใช้ Docker

```bash
# Postgres ชั่วคราว
docker run -d --name smalltalk-db -e POSTGRES_PASSWORD=dev -e POSTGRES_DB=smalltalk -p 5432:5432 postgres:17-alpine

# API (migration รันอัตโนมัติตอนเริ่ม)
cd backend
DATABASE_URL='postgres://postgres:dev@localhost:5432/smalltalk?sslmode=disable' \
JWT_SECRET='dev-secret-at-least-32-characters-long' \
ALLOWED_ORIGINS='localhost:3000' \
go run ./cmd/server

# Web (proxy /api ไปที่ :8080, WebSocket ต่อ ws://localhost:8080/ws)
cd frontend && npm install && npm run dev
```

## ทดสอบ

```bash
cd backend
go test ./...                       # unit tests
TEST_DATABASE_URL='postgres://postgres:dev@localhost:5432/smalltalk_test?sslmode=disable' \
  go test ./...                     # + integration/WebSocket tests กับ Postgres จริง
golangci-lint run ./...

cd frontend
npm run lint && npm run format:check && npx tsc --noEmit && npm run build
```

CI (`.gitlab-ci.yml`) รัน lint + test (มี Postgres service) + build ทั้งสองฝั่ง

## โครงสร้าง package ของ Backend

ลูกศรชี้จาก package ที่ import ไปยัง package ที่ถูก import (ไม่มี import cycle)

```mermaid
graph TD
    main["cmd/server"]
    app["internal/app"]
    chat["internal/chat"]
    invite["internal/invite"]
    media["internal/media"]
    room["internal/room"]
    httpx["platform/httpx"]
    auth["internal/auth"]
    apperr["internal/apperr"]
    config["platform/config"]
    db["platform/db"]

    main --> app
    main --> chat
    main --> config
    main --> db

    app --> chat
    app --> invite
    app --> media
    app --> room
    app --> auth
    app --> httpx
    app --> config

    chat --> media
    chat --> room
    chat --> auth
    chat --> httpx
    chat --> apperr

    invite --> room
    invite --> auth
    invite --> httpx
    invite --> apperr

    media --> room
    media --> httpx
    media --> apperr

    room --> auth
    room --> httpx
    room --> apperr

    httpx --> apperr

    classDef entry fill:#fde68a,stroke:#b45309,color:#111
    classDef domain fill:#bfdbfe,stroke:#1d4ed8,color:#111
    classDef platform fill:#d1fae5,stroke:#047857,color:#111
    classDef leaf fill:#e5e7eb,stroke:#4b5563,color:#111
    class main,app entry
    class chat,invite,media,room domain
    class httpx,config,db platform
    class auth,apperr leaf
```

- `app` เป็นจุดประกอบ dependency ทั้งหมดและผูก route ส่วน `cmd/server` เปิด DB, รัน migration และจัดการ graceful shutdown
- `room` เป็น domain หลักที่ `chat`, `invite` และ `media` พึ่งพา (ตรวจสมาชิกภาพของห้อง)
- `auth`, `apperr`, `platform/config` และ `platform/db` ไม่ import package ภายในตัวอื่น ควรรักษาไว้แบบนี้เพื่อไม่ให้เกิด cycle

## API

สเปกฉบับเต็ม (OpenAPI 3.1) อยู่ที่ [docs/openapi.yaml](docs/openapi.yaml) ครอบคลุมทั้ง REST, error code และ WebSocket frame
เปิดดูแบบเอกสารได้ด้วย `npx @redocly/cli preview-docs docs/openapi.yaml`
ถ้าเพิ่ม ลบ หรือเปลี่ยน route โดยไม่แก้สเปก test `TestOpenAPIMatchesRouter` จะไม่ผ่าน

REST (ผ่าน Nginx อยู่ใต้ `/api`) — JSON snake_case, เวลาเป็น RFC 3339 UTC, error รูปแบบ `{"error": {"code", "message"}}`

| Method | Path | ใคร | หมายเหตุ |
| --- | --- | --- | --- |
| POST | `/rooms` | Host | `{name, display_name}` → `room_id, owner_token, jwt, member_id` |
| POST | `/rooms/{id}/owner-session` | Host | `{owner_token}` → JWT ใหม่ เมื่อ JWT เดิมหมดอายุ |
| GET | `/rooms/{id}` | สมาชิก | ข้อมูลห้อง |
| POST | `/rooms/{id}/invites` | Host | `{expires_in_minutes?, max_uses?}` → `token, invite_url, expires_at` |
| GET | `/rooms/{id}/invites` | Host | invite ที่ยังใช้ได้ |
| DELETE | `/rooms/{id}/invites/{inviteId}` | Host | ยกเลิกลิงก์เชิญ |
| POST | `/join` | Guest | `{token, display_name}` → `jwt, room_id, member_id` |
| GET | `/rooms/{id}/messages?before={ulid}&limit=50` | สมาชิก | เรียงเก่า→ใหม่ + `next_cursor` |
| GET | `/rooms/{id}/members` | สมาชิก | รายชื่อ + `online` |
| POST | `/rooms/{id}/members/{mid}/kick` | Host | `{ban?: bool}` ตัด connection ทันที |
| POST | `/rooms/{id}/close` | Host | ปิดห้อง |
| POST | `/rooms/{id}/images` | สมาชิก | multipart field `image` ≤ 10 MB → `id, width, height, content_type` |
| GET | `/rooms/{id}/images/{imageId}` | สมาชิก | ไฟล์รูป (ต้องส่ง JWT; client ใช้ fetch → object URL) |
| POST | `/rooms/summaries` | ใครก็ได้ที่มี JWT | `{rooms: [{room_id, jwt, last_read_id}]}` → สถานะห้อง ข้อความล่าสุด และ `unread` ต่อห้อง |

WebSocket: `/ws?room={id}` ส่ง JWT เป็น subprotocol ตัวที่สอง
`new WebSocket(url, ["chat", jwt])` เพื่อไม่ให้ token ไปอยู่ใน URL/log

- client → server: `send {client_msg_id, body, image_id?}`, `resume {last_message_id}`, `ping`
- server → client: `message`, `ack {client_msg_id, id}`, `presence {member_id, online}`, `error {code, message, client_msg_id?}`, `kicked {reason}`, `room_closed`, `pong`
- close code 4001 unauthorized, 4003 ถูกเตะ/แบน, 4004 ห้องปิด — client จะไม่ reconnect

### รูปภาพ

1. Client ย่อรูปให้ด้านยาวไม่เกิน 2048 px และเข้ารหัสใหม่ ซึ่งจะหมุนรูปตาม EXIF และลบ metadata ทั้งหมดรวมถึงพิกัด GPS
2. อัปโหลดด้วย `POST /rooms/{id}/images` แล้วส่ง `send` พร้อม `image_id` ทาง WebSocket
3. Server ตรวจชนิดไฟล์จากเนื้อไฟล์จริง (รับเฉพาะ JPEG/PNG/GIF ไม่เกิน 8192 px ต่อด้าน) และลบ EXIF/XMP/IPTC รวมถึง text chunk ของ PNG ซ้ำอีกรอบ
4. รูปหนึ่งส่งได้ครั้งเดียว และส่งได้เฉพาะคนที่อัปโหลด รูปที่อัปโหลดแล้วไม่ถูกส่งภายใน 1 ชั่วโมงจะถูกลบ
5. ตอนดาวน์โหลดตอบพร้อม `nosniff` และ `Content-Security-Policy: sandbox` คนที่ถูกเตะ/แบนหรืออยู่ห้องอื่นเปิดรูปไม่ได้

## Service API สำหรับระบบภายนอก

ระบบอื่น (เช่นระบบเบิกค่ารักษาพยาบาล) ใช้ smalltalk เป็น chat service ได้ผ่าน `/service/v1`
ระบบนั้นตรวจสิทธิ์ผู้ใช้เอง แล้วขอห้องและ session แทนผู้ใช้ — browser ของผู้ใช้ต่อ REST/WebSocket ของ smalltalk ตรงด้วย JWT ที่ได้มา

- ห้องผูกกับ `ref` ของระบบภายนอก เช่น `claims:ticket:123:public`; เรียก `PUT` ซ้ำได้ห้องเดิม
- สมาชิกผูกกับ `external_user_id`; ขอ session ซ้ำได้ `member_id` เดิม และมี `label` บอกบทบาท
- ห้องแบบนี้ไม่มีลิงก์เชิญและไม่มี owner
- เรียกได้จากเครือข่ายภายในเท่านั้น — Nginx ตอบ 404 ให้ `/api/service/`

| Method | Path | หมายเหตุ |
| --- | --- | --- |
| PUT | `/service/v1/rooms/{ref}` | `{name}` → `room_id, status` สร้างห้องถ้ายังไม่มี |
| POST | `/service/v1/rooms/{ref}/sessions` | `{external_user_id, display_name, label?}` → session (JWT อายุ `SERVICE_JWT_TTL`) |
| POST | `/service/v1/rooms/{ref}/close` | ปิดห้อง เรียกซ้ำได้ |
| GET | `/service/v1/rooms/{ref}/images/{imageId}` | ไฟล์รูปที่ถูกส่งในห้องนั้นแล้ว + header `X-Sender-External-Id` |

| ตัวแปร | ค่าเริ่มต้น | ความหมาย |
| --- | --- | --- |
| `SERVICE_API_KEY` | ว่าง (ปิด) | key ที่ระบบภายนอกส่งใน `Authorization: Bearer`, ยาว ≥ 32 ตัวอักษร |
| `SERVICE_JWT_TTL` | `15m` | อายุ JWT ที่ออกผ่าน service API |
| `CORS_ORIGINS` | ว่าง | origin ของเว็บอื่นที่เรียก REST ได้ เช่น `https://claims.example.com` |
| `ALLOWED_ORIGINS` | ว่าง | origin/host ที่เปิด WebSocket ได้ นอกจาก host ของตัวเอง |

รายละเอียด endpoint อยู่ใน [docs/openapi.yaml](docs/openapi.yaml) (tag `service`)

## สิ่งที่ต่างจากแผนเล็กน้อย

- `room_members` มีคอลัมน์ `kicked_at` เพิ่ม เพื่อแยก "เตะ" (เข้าใหม่ด้วยลิงก์เชิญได้) กับ "แบน"
- `messages.id` ใช้ `COLLATE "C"` ให้ ULID เรียงแบบ byte เสมอไม่ขึ้นกับ locale
- Integration test ใช้ Postgres จาก `TEST_DATABASE_URL` (CI ใช้ service) แทน testcontainers-go
- เพิ่ม `POST /rooms/{id}/owner-session`, `GET /rooms/{id}`, `GET /rooms/{id}/invites` และ frame `pong`
- มี Nginx ตั้งแต่ตอนนี้ เพื่อให้มือถือเข้าผ่าน origin เดียว

- รูปภาพผ่าน API แทน S3 presigned URL และเก็บลงดิสก์ (interface `media.Storage` รอเปลี่ยนเป็น S3/R2)
- รายชื่อห้องผูกกับ browser เพราะไม่มีบัญชีผู้ใช้ ถ้าเปลี่ยนเครื่องหรือล้างข้อมูล browser ต้องใช้ลิงก์เชิญใหม่

## ยังไม่ได้ทำ (Phase 3 / หลัง MVP)

- Redis Pub/Sub + presence ข้าม instance (มี interface `chat.Broker` รอไว้แล้ว)
- เก็บรูปบน S3/R2 เมื่อรันหลาย instance
- Metrics, load test k6 1,000 connections
- ข้อจำกัด: การแบนผูกกับ member ไม่ใช่ตัวบุคคล เพราะไม่มีบัญชีผู้ใช้ คนที่ถูกแบนยังใช้ลิงก์เชิญที่ยังใช้ได้เข้ามาใหม่ได้ — ให้ Host ยกเลิกลิงก์เชิญแล้วสร้างใหม่

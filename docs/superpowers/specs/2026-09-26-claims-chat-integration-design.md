# เชื่อมแชท smalltalk เข้ากับระบบเบิกค่ารักษาพยาบาล — Design

วันที่: 2026-09-26
สถานะ: รอรีวิว

## เป้าหมาย

ระบบเบิกค่ารักษาพยาบาล ([pea-medical-claims](https://github.com/Kmipxy86/pea-medical-claims)) เป็นระบบหลัก
ทุก ticket มีแชท realtime จาก smalltalk ไว้ให้ผู้ยื่นคุยกับเจ้าหน้าที่ แทน `comments` เดิมที่ไม่ realtime (ต้องรีเฟรชทั้ง ticket ถึงจะเห็นข้อความใหม่)

smalltalk ยังเป็นผลิตภัณฑ์แยกของเรา ทำหน้าที่เป็น **chat service** ที่ระบบ customer service อื่นเรียกใช้ได้
ระบบเบิกเป็นลูกค้ารายแรก

### สำเร็จเมื่อ

1. พนักงานยื่นเรื่องแล้วคุยกับเจ้าหน้าที่แบบ realtime ในหน้า ticket ได้ ส่งทั้งข้อความและรูปภาพ
2. เจ้าหน้าที่มีช่อง "ความเห็นภายใน" แยกต่างหาก ผู้ยื่นไม่มีทางเข้าถึงช่องนี้
3. ผู้ยื่น หรือเจ้าหน้าที่ที่รับเรื่อง กดแนบรูปที่ผู้ยื่นส่งในแชทเข้า ticket ได้ รูปที่แนบผ่านการตรวจเดิมทุกอย่าง (SHA-256, เอกสารครบ)
4. เมื่อ ticket เป็น `paid` หรือ `rejected` แชทจะอ่านได้อย่างเดียว
5. ถ้า smalltalk ล่ม หน้า ticket ยังใช้งานได้ ยกเว้นส่วนแชท
6. คนที่ไม่มีสิทธิ์ใน ticket เข้าห้องแชทของ ticket นั้นไม่ได้ ไม่มีลิงก์เชิญหรือช่องทางแบบไม่ระบุตัวตน

### ไม่อยู่ในขอบเขต

- ย้ายข้อมูล `comments` เดิมเข้าแชท (ของเดิมยังแสดงแบบอ่านอย่างเดียว)
- เปิดห้องกลับหลังปิด (workflow ของระบบเบิกไม่มีทางย้อนจาก `paid`/`rejected`)
- แจ้งเตือนผ่าน LINE/อีเมลเมื่อมีข้อความใหม่
- ส่ง audit log ของ smalltalk กลับไประบบเบิก (ระบบเบิกบันทึกการเปิดแชทเองแล้ว)

## สถาปัตยกรรม

```
Browser (claims frontend)
   │ cookie session              │ JWT ของสมาชิกห้อง (REST + WebSocket)
   ▼                             ▼
claims backend ──service key──▶ smalltalk backend ──▶ smalltalk DB
   │  ตรวจสิทธิ์ + audit              ห้อง / ข้อความ / รูป
   ▼
claims DB
```

- claims backend เป็น **trusted client** ของ smalltalk: ตรวจสิทธิ์ผู้ใช้ด้วยกฎของตัวเอง แล้วขอ session แทนผู้ใช้
- browser ต่อ smalltalk ตรง (REST + WebSocket) ด้วย JWT ที่ได้จาก claims backend
- ห้องที่ผูกกับ ticket มี 2 ห้อง:
  - `claims:ticket:{id}:public` — ผู้ยื่นและเจ้าหน้าที่
  - `claims:ticket:{id}:internal` — เจ้าหน้าที่เท่านั้น

  แยกเป็นคนละห้องเพื่อให้ข้อความภายในแยกออกจากห้องของผู้ยื่นจริง ๆ ไม่ได้อาศัย flag กรองข้อความ

## ฝั่ง smalltalk (repo นี้)

### ข้อมูล

Migration `000006_service_rooms`:

| ตาราง | คอลัมน์ | ความหมาย |
| --- | --- | --- |
| `rooms` | `external_ref TEXT UNIQUE NULL` | อ้างอิงจากระบบภายนอก เช่น `claims:ticket:123:public`; NULL = ห้องปกติ |
| `room_members` | `external_user_id TEXT NULL` | id ผู้ใช้ในระบบภายนอก; `UNIQUE (room_id, external_user_id)` |
| `room_members` | `label TEXT NOT NULL DEFAULT ''` | ป้ายบอกบทบาท เช่น "เจ้าหน้าที่ตรวจสอบ" แสดงคู่กับชื่อ |

`owner_token_hash` ของห้อง service เก็บ hash ของ token สุ่มที่ไม่มีใครถือไว้ จึงไม่มีทางได้ owner session

### Package `internal/service`

Routes อยู่ใต้ `/service/v1` ต้องมี `Authorization: Bearer <SERVICE_API_KEY>` (เทียบแบบ constant-time)
nginx ไม่ส่งต่อ `/service` จึงเรียกได้จากเครือข่ายภายในเท่านั้น

| Method & path | Body | ผลลัพธ์ |
| --- | --- | --- |
| `PUT /service/v1/rooms/{ref}` | `{name}` | สร้างห้องถ้ายังไม่มี (idempotent) → `{room_id, status}` |
| `POST /service/v1/rooms/{ref}/sessions` | `{external_user_id, display_name, label}` | upsert สมาชิก → `Session` รูปแบบเดิม (`room_id, member_id, role:"member", jwt, expires_at`) |
| `POST /service/v1/rooms/{ref}/close` | — | ตั้งห้องเป็น `closed` (idempotent) |
| `GET /service/v1/rooms/{ref}/images/{image_id}` | — | ไฟล์รูป (bytes + Content-Type) เฉพาะรูปที่ถูกส่งในห้องนั้นแล้ว พร้อม header `X-Sender-External-Id` (ใครเป็นคนส่ง) |

- `ref` ต้องตรง `^[a-z0-9:_-]{1,128}$`
- ถ้าเรียก sessions กับห้องที่ปิดแล้ว ยังได้ JWT (ไว้อ่านประวัติ) ส่วนการส่งข้อความถูก `CanPost` ปฏิเสธตามเดิม
- ถ้าสมาชิกเคยถูก kick/ban ในห้อง service ให้ตอบ 409 (ตามปกติห้อง service ไม่มีการ kick เพราะไม่มี owner)

### การเปลี่ยนพฤติกรรมเดิม

- **Invite:** สร้างลิงก์เชิญหรือ join ห้องที่มี `external_ref` ไม่ได้ ตอบ `404 room_not_found` เหมือนห้องที่ไม่มีอยู่
- **JWT TTL:** session ห้อง service มีอายุ `SERVICE_JWT_TTL` (ค่าเริ่มต้น 15 นาที) ห้องปกติยังใช้ `JWT_TTL` เดิม
- **CORS:** เพิ่ม middleware สำหรับ REST สาธารณะ อนุญาตเฉพาะ origin ใน `CORS_ORIGINS`
  - header ที่อนุญาต: `Authorization`, `Content-Type`
  - ไม่ใช้ credentials เพราะ JWT อยู่ใน header
- **WebSocket:** เพิ่ม claims origin ใน `ALLOWED_ORIGINS`
- **ชื่อที่แสดง:** `MemberView` และข้อมูลผู้ส่งข้อความเพิ่ม `label`

### Config ใหม่

| ตัวแปร | ค่าเริ่มต้น | ความหมาย |
| --- | --- | --- |
| `SERVICE_API_KEY` | ว่าง = ปิด `/service/v1` ทั้งหมด | ต้องยาว ≥ 32 ตัวอักษร |
| `SERVICE_JWT_TTL` | `15m` | อายุ JWT ของห้อง service |
| `CORS_ORIGINS` | ว่าง | origin ที่เรียก REST ข้าม origin ได้ |

## ฝั่งระบบเบิก (fork ของ pea-medical-claims)

Fork: [guy127/pea-medical-claims](https://github.com/guy127/pea-medical-claims) (แยกจาก upstream ที่ `9df2902`)
ทำงานใน branch `feat/smalltalk-chat` แล้วเปิด PR เข้า `main` ของ fork

### Backend — `chat.go` ใหม่

Config: `SMALLTALK_INTERNAL_URL` (เช่น `http://smalltalk-backend:8080`), `SMALLTALK_PUBLIC_URL` (ที่ browser ใช้), `SMALLTALK_SERVICE_KEY`
ถ้าไม่ตั้ง `SMALLTALK_INTERNAL_URL` ระบบจะปิดแชท และ frontend กลับไปใช้ `Messages.tsx` เดิม

**`POST /api/tickets/{id}/chat-session?channel=public|internal`**
1. โหลด ticket แล้วตรวจสิทธิ์ด้วยกฎเดียวกับ `handleGetTicket`
   - `public`: เจ้าของเรื่อง หรือ staff
   - `internal`: staff เท่านั้น (employee ได้ 403)
2. `PUT` ห้อง (สร้างครั้งแรกที่มีคนเปิด)
3. ถ้าสถานะ ticket เป็น `paid`/`rejected` ให้เรียก `close` ด้วย (ตามเก็บกรณีที่ปิดหลัง transition ไม่สำเร็จ)
4. ขอ session ด้วย `external_user_id = "user:{users.id}"`, `display_name = users.name`, `label = roleLabel[role]`
5. เขียน `audit_log` action `chat.open` พร้อม detail `channel=…`
6. ตอบ `{session, baseUrl: SMALLTALK_PUBLIC_URL, closed}`

**`POST /api/tickets/{id}/chat-images/{imageId}/attach`** body `{doc_type}` (ใช้ได้เฉพาะรูปในห้อง `public`)

สิทธิ์ (กฎใหม่ในระบบเบิก เดิมมีแต่ผู้ยื่นที่แนบไฟล์ได้):

| ผู้กด | สถานะ ticket ที่อนุญาต | รูปที่แนบได้ |
| --- | --- | --- |
| ผู้ยื่น | `draft`, `need_info` (กฎเดิม) | รูปที่ตัวเองส่ง |
| staff ที่เป็น `assignee_id` ของเรื่อง หรือ `admin` | `in_review`, `need_info` | รูปที่ **ผู้ยื่น** ส่ง (ตรวจจาก `X-Sender-External-Id`) |

- นอกเหนือจากนี้ตอบ 409 พร้อมข้อความภาษาไทยแบบเดียวกับ `handleUpload`
- staff แนบรูปที่ staff ส่งเองไม่ได้ เอกสารประกอบการเบิกต้องมาจากผู้ยื่นเสมอ
- `attachments.uploaded_by` = ผู้กด ถ้าเป็น staff จะเห็นได้ในรายการไฟล์และ audit ว่าเจ้าหน้าที่เป็นคนแนบ
- ดึงไฟล์จาก `GET /service/v1/rooms/{ref}/images/{imageId}` แล้วส่งต่อเข้าฟังก์ชันบันทึกไฟล์แนบเดิม
  - แยกส่วนบันทึกไฟล์ออกจาก `handleUpload` เป็นฟังก์ชันที่รับ `io.Reader`
  - ไฟล์จึงผ่าน SHA-256, ตรวจไฟล์ซ้ำ, flags และ OCR ครบ
- เขียน `audit_log` action `attachment.from_chat`

**หลัง transition เป็น `paid`/`rejected`:** เรียก `close` ทั้งสองห้องแบบ best-effort
ถ้าล้มเหลวให้ log ไว้ transition ยังสำเร็จ และขั้นที่ 3 ของ chat-session จะปิดห้องให้ภายหลัง

**HTTP client:** timeout 5 วินาที ส่ง service key เฉพาะ `SMALLTALK_INTERNAL_URL`
smalltalk ตอบ error หรือ timeout → ตอบ `502 {"error":"chat_unavailable"}`

### Frontend

- `src/components/chat/` ใหม่ port จาก smalltalk: `useChatSocket`, `MessageList`, `Composer`, `ChatImage`, `ImageViewer`, การอัปโหลด/แปลงรูป
  - ปรับให้ใช้ `baseUrl` จาก chat-session
  - เก็บ JWT ใน React state เท่านั้น ไม่ใช้ localStorage
  - ใช้ CSS ของระบบเบิก
- `TicketChat.tsx`
  - ผู้ยื่นเห็นห้อง `public` ห้องเดียว
  - staff มีแท็บ "ข้อความถึงผู้ยื่น" / "ความเห็นภายใน" ใช้คำเดิมของ `Messages.tsx`
- ต่ออายุ JWT: ขอ chat-session ใหม่ก่อนหมดอายุ 1 นาที และทุกครั้งที่ socket ปิดด้วย 4001
- ห้องปิด (4004 หรือ `closed: true`): แสดงประวัติ ซ่อนช่องพิมพ์ และขึ้นข้อความ "เรื่องนี้ปิดแล้ว — ดูประวัติได้อย่างเดียว"
- chat-session ได้ 502 หรือเครือข่ายล้ม: แสดง "แชทใช้งานไม่ได้ชั่วคราว" พร้อมปุ่มลองใหม่ ส่วนอื่นของหน้าทำงานปกติ
- รูปในแชท `public`: ปุ่ม "แนบเข้า ticket" แสดงตามตารางสิทธิ์ด้านบน (frontend ซ่อนปุ่มเพื่อ UX ส่วน backend เป็นตัวตัดสินจริง) กดแล้วเปิด dialog เลือก `doc_type` และรีเฟรชรายการไฟล์แนบ
- ถ้ามี `comments` เดิม แสดงเหนือแชทแบบอ่านอย่างเดียวในหัวข้อ "ข้อความเดิม"

### Deploy

`docker-compose.yml` ใน fork เพิ่ม services:
- `smalltalk-db`
- `smalltalk-backend` (build จาก git submodule หรือ image)
- nginx หรือ port ที่เปิด REST/WS ของ smalltalk ให้ browser เรียกได้

ตั้งค่า `SERVICE_API_KEY` ให้ตรงกับ `SMALLTALK_SERVICE_KEY` และตั้ง `CORS_ORIGINS`/`ALLOWED_ORIGINS` เป็น origin ของ claims frontend

## Error handling สรุป

| สถานการณ์ | ผลลัพธ์ |
| --- | --- |
| employee ขอ internal session | 403 จาก claims |
| ขอ session ของ ticket คนอื่น | 404 (ตามกฎเดิมของ handleGetTicket) |
| service key ผิด หรือไม่ส่ง | 401 จาก smalltalk; claims ตอบ 502 |
| `SERVICE_API_KEY` ไม่ตั้ง | `/service/v1/*` ตอบ 404 |
| JWT หมดอายุระหว่างใช้ | socket 4001 → frontend ขอ session ใหม่ ถ้าสิทธิ์ถูกถอนแล้วจะได้ 403 และแชทปิด |
| smalltalk ล่ม | หน้า ticket ใช้ได้ แชทขึ้นข้อความพร้อมปุ่มลองใหม่ |
| ปิดห้องหลัง transition ไม่สำเร็จ | log ไว้ แล้วปิดตอนมีคนขอ chat-session ครั้งถัดไป |
| attach รูปที่ไม่อยู่ในห้องนั้น | 404 จาก smalltalk → claims ตอบ 404 |
| attach รูปซ้ำ | ใช้กฎไฟล์ซ้ำเดิมของระบบเบิก (ติด flag) |

## การทดสอบ

**smalltalk**
- Unit
  - middleware service key: ไม่มี key / key ผิด / key ถูก / ปิดใช้งาน
  - validate `ref`
  - คำนวณ TTL ตามชนิดห้อง
- Integration (Postgres จริง)
  - `PUT` ห้องซ้ำได้ห้องเดิม
  - upsert `external_user_id` เดิมได้ `member_id` เดิม
  - สร้างลิงก์เชิญหรือ join ห้อง service ไม่ได้
  - ส่งข้อความในห้องปิดไม่ได้ แต่อ่านประวัติได้
  - ดึงรูปของห้องอื่นได้ 404
  - CORS preflight: origin ที่อนุญาต / ไม่อนุญาต
- `openapi_test.go`: เพิ่ม spec ของ `/service/v1`

**ระบบเบิก (fork)** ใช้ `httptest` จำลอง smalltalk
- employee ขอ internal → 403; staff ขอ internal → 200
- ขอ ticket ของคนอื่น → 404
- เขียน `audit_log` `chat.open`
- ticket `paid` → เรียก close และตอบ `closed: true`
- smalltalk ตอบ 500 หรือ timeout → 502
- attach: ไฟล์เข้า `attachments` และได้ SHA-256; แนบรูปเดิมซ้ำแล้วติด flag ไฟล์ซ้ำ
- attach ตามตารางสิทธิ์:
  - assignee ใน `in_review` → 200
  - staff ที่ไม่ใช่ assignee → 409
  - staff แนบรูปที่ staff ส่งเอง → 409
  - ผู้ยื่นใน `in_review` → 409
- `workflow_test.go` เดิมต้องผ่าน

**End-to-end ด้วยมือ (docker compose ของ fork)**
1. employee ยื่นเรื่อง → เปิดแชท → ส่งข้อความและรูปใบเสร็จ
2. reviewer เห็นข้อความทันที ตอบกลับ และเขียนความเห็นภายใน (ตรวจว่า employee มองไม่เห็น)
3. reviewer รับเรื่อง (`in_review`) แล้วแนบรูปใบเสร็จที่ผู้ยื่นส่งในแชทเป็น `receipt` → รูปขึ้นในรายการเอกสาร และ checks อัปเดต
4. approve → paid → แชททั้งสองฝั่งอ่านได้อย่างเดียว
5. หยุด smalltalk-backend → หน้า ticket ยังใช้ได้ แชทขึ้นข้อความว่าใช้งานไม่ได้

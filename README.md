# Book Review Backend

Go REST API สำหรับ frontend React ที่ได้รับมา ใช้ PostgreSQL บน **Supabase**, deploy บน **Render** จาก GitHub และรับ request จาก frontend บน **Vercel**

โค้ด frontend ใช้งานได้ผ่าน `src/api/backend.js` เดิม ตั้งค่า `REACT_APP_API_URL` เป็น URL backend โดยไม่เติม `/api` และไม่ลงท้าย `/`

## สิ่งที่ทำได้

- สมัครสมาชิก / เข้าสู่ระบบด้วย JWT อายุ 24 ชั่วโมง รหัสผ่านเก็บด้วย bcrypt
- ดูและแก้โปรไฟล์ เปลี่ยนรหัสผ่านแล้วเพิกถอน token เดิมทุกอุปกรณ์
- รายการหนังสือ ค้นหาชื่อ/ผู้แต่ง/คำอธิบาย กรองหมวดหมู่ ดูรายละเอียด
- เพิ่ม/แก้/ลบรีวิวของตัวเอง จำกัดหนึ่งรีวิวต่อสมาชิกต่อหนังสือ
- แอดมินเพิ่ม/แก้/ลบหนังสือ ดูสถิติ และลบรีวิวได้ แต่แก้ข้อความแทนเจ้าของไม่ได้
- คะแนนเฉลี่ยและจำนวนรีวิวคำนวณจากฐานข้อมูลทุกครั้ง ไม่มีคะแนนจำลอง
- CORS ระบุ origin ที่อนุญาต, จำกัดขนาด JSON, parameterized SQL, จำกัดความถี่ login ต่ออีเมล
- ตารางอยู่ใน schema `bookreview` แยกจาก `public` และเปิด RLS โดยไม่มีนโยบายให้ browser เข้าถึงโดยตรง

## รันในเครื่อง

ต้องมี Go 1.25 ขึ้นไป และ PostgreSQL (ใช้ Supabase ได้)

เปิด PowerShell ในโฟลเดอร์ repo แล้วตั้ง environment variables:

```powershell
$env:DATABASE_URL = 'postgresql://postgres.PROJECT_REF:URL_ENCODED_PASSWORD@POOLER_HOST:5432/postgres?sslmode=require'
$env:JWT_SECRET = 'ใส่ค่าสุ่มยาวอย่างน้อย-32-bytes-ห้ามใช้ค่านี้จริง'
$env:ALLOWED_ORIGINS = 'http://localhost:3000'
$env:AUTO_MIGRATE = 'true'
$env:PORT = '8080'
go mod download
go run ./cmd/server
```

ตัวโปรแกรมอ่าน environment ของ process โดยตรง **ไม่ได้โหลด `.env` อัตโนมัติ**; `.env.example` เป็นรายการตัวแปรอ้างอิง ห้าม commit secret หรือ DATABASE_URL จริง

ตรวจ `http://localhost:8080/health` ต้องได้ `{"status":"ok"}`

คำสั่งจัดการ (ตั้ง DATABASE_URL ก่อน):

```powershell
go run ./cmd/server -migrate
go run ./cmd/server -seed
go run ./cmd/server -admin-email 'your-real-email@example.com'
```

`-seed` ใส่หนังสือตัวอย่าง 6 เล่มจาก frontend เฉพาะเมื่อ catalog ว่าง ไม่สร้างบัญชีหรือรีวิวจำลอง ไม่ใช้รหัสผ่านแอดมินที่อยู่ใน mock frontend ต้องสมัครอีเมลจริงก่อนจึงค่อย promote ด้วย `-admin-email` หรือ SQL ที่อธิบายด้านล่าง

## Deploy: Supabase → Render → Vercel

### 1. Supabase (ฐานข้อมูล)

1. สร้าง project และเก็บ database password ไว้ในที่ปลอดภัย
2. เปิด **Connect → Session pooler** คัดลอก PostgreSQL connection string (port 5432) ซึ่งรองรับ IPv4 เหมาะกับ backend แบบ server ที่รันค้าง
3. ใส่รหัสผ่านที่ URL-encode แล้วลงใน connection string และเติม `?sslmode=require` หากมี query อยู่แล้วใช้ `&sslmode=require` ห้ามปิด SSL
4. ใช้ connection string นี้เป็น `DATABASE_URL` บน Render ไม่ใช่ Supabase project URL หรือ anon key
5. schema จะสร้างให้เมื่อ backend เริ่มทำงานด้วย `AUTO_MIGRATE=true` หรือรัน SQL จาก `migrations/001_init.sql` ใน Supabase SQL Editor ก่อนก็ได้

เชื่อมด้วย database owner `postgres` ตามค่า Session pooler: owner เป็นผู้เข้าถึงตารางที่เปิด RLS; ไม่ใช้ anon/authenticated role และไม่เปิด schema `bookreview` ใน Data API ไม่ต้องตั้ง Supabase Auth เพราะระบบนี้จัดการสมาชิกเองให้ตรงกับ numeric ID ใน frontend

### 2. GitHub และ Render (backend)

1. ตรวจโค้ดแล้ว commit และ push ไปที่ `Sukphachai/Review-book-backend` branch `main`
2. เข้า Render → **New → Blueprint** แล้วเชื่อม GitHub เลือก repo นี้ ระบบจะอ่าน `render.yaml`
3. ตั้งค่าที่ Render ขอ:

| ตัวแปร | ค่า |
| --- | --- |
| `DATABASE_URL` | Session pooler URL ของ Supabase พร้อม SSL |
| `ALLOWED_ORIGINS` | เช่น `https://your-frontend.vercel.app,http://localhost:3000` ไม่มี `/` ท้าย URL |
| `JWT_SECRET` | Blueprint สร้างค่าสุ่มให้ เก็บค่านี้คงเดิมระหว่าง deploy |
| `AUTO_MIGRATE` | `true` |

Render ใช้ Dockerfile build Go และฟังบน `0.0.0.0:$PORT` แล้วตรวจสุขภาพที่ `/health` ไม่ต้องสร้าง Render PostgreSQL เพราะฐานข้อมูลอยู่ Supabase แล้ว

หากสร้าง Web Service เอง ให้เลือก runtime **Docker**, branch `main`, Dockerfile path `./Dockerfile`, health check `/health` และตั้ง environment ตามตาราง เมื่อเชื่อม GitHub แล้วให้เปิด auto-deploy ตามต้องการ

ทดสอบ `https://YOUR-SERVICE.onrender.com/health` และ `/api/books` ก่อนเปิดใช้งาน frontend ช่วงแรก `/api/books` อาจได้ `{"items":[]}` จนกว่าจะเพิ่มหนังสือหรือ seed

### 3. Vercel (ส่งให้เพื่อนที่ทำ frontend)

ในโปรเจกต์ frontend ของเพื่อน ตั้ง Environment Variable:

```text
REACT_APP_API_URL=https://YOUR-SERVICE.onrender.com
```

ใช้ตัวแปร `REACT_APP_` เพราะ frontend นี้ใช้ Create React App; ให้ตั้ง Production (และ Preview หากต้องการ) แล้ว **redeploy frontend** เพื่อให้ค่าเข้าไปใน build ไม่ต้องเปลี่ยน `src/api/backend.js` หรือเพิ่ม `/api` ท้ายค่า

นำ origin ของ frontend ที่ใช้งานจริงไปใส่ `ALLOWED_ORIGINS` บน Render แล้ว redeploy backend หากใช้ custom domain ให้เพิ่ม origin นั้นด้วย Preview domain ของ Vercel เปลี่ยนได้ ต้องเพิ่มเป็นราย origin ไม่มีการอนุญาต `*.vercel.app` ทั้งหมด

### 4. สร้างผู้ดูแลระบบ

สมัครผ่านหน้า frontend ปกติก่อน แล้วรันใน **Supabase SQL Editor** ด้วยอีเมลของคุณ:

```sql
UPDATE bookreview.members
SET role = 'admin'
WHERE email = lower('your-real-email@example.com');
```

ตรวจว่ามีแถวถูกแก้ไข จากนั้น logout/login ใหม่เพื่อให้ frontend รับบทบาทล่าสุด ไม่มีบัญชี admin หรือรหัสผ่านเริ่มต้นฝังไว้ในระบบ

เพิ่มหนังสือผ่านหน้า admin หรือรันคำสั่ง `-seed` ในเครื่องโดยตั้ง DATABASE_URL ไป Supabase (เลือกทำอย่างใดอย่างหนึ่ง ไม่ต้องเปิด public seed endpoint)

## API

รูปแบบ JSON ใช้ camelCase และ error รูปแบบ `{"error":"ข้อความ"}` ให้ตรง frontend รายการคืน `{ "items": [] }`, ลบสำเร็จคืน 204 ไม่มี body

| Method | Path | สิทธิ์ |
| --- | --- | --- |
| GET | `/health` | ทุกคน (ตรวจ DB ด้วย) |
| POST | `/api/auth/register` | ทุกคน |
| POST | `/api/auth/login` | ทุกคน |
| GET | `/api/me` | สมาชิก |
| PUT | `/api/me/profile` | สมาชิก |
| PUT | `/api/me/password` | สมาชิก |
| GET | `/api/books?search=&category=` | ทุกคน |
| GET | `/api/books/{id}` | ทุกคน |
| GET | `/api/categories` | ทุกคน |
| GET | `/api/books/{id}/reviews` | ทุกคน |
| POST | `/api/books/{id}/reviews` | สมาชิก |
| GET | `/api/me/reviews` | สมาชิก |
| PUT | `/api/reviews/{id}` | เจ้าของ |
| DELETE | `/api/reviews/{id}` | เจ้าของ / admin |
| GET | `/api/admin/stats` | admin |
| POST | `/api/admin/books` | admin |
| PUT, DELETE | `/api/admin/books/{id}` | admin |

API ที่ต้อง login ส่ง `Authorization: Bearer <token>` โครง request/response ดู [สัญญา API จาก frontend](docs/frontend-api-contract.md)

ข้อแตกต่างที่ควรแจ้งเพื่อน:

- หลังเปลี่ยนรหัสผ่าน token เดิมใช้ไม่ได้แล้ว ต้อง login ใหม่ แนะนำให้ frontend เรียก `logout()` แล้วไป `/login` หลัง `apiChangePassword` สำเร็จใน `AuthContext.jsx`
- รีวิวต้องเป็นคะแนนจำนวนเต็ม 1–5 และข้อความ 5–5000 ตัวอักษร ทั้งตอนเพิ่มและแก้ไข
- ราคาไม่ติดลบ, URL รูปปกต้องเป็น http/https; รูปปกเป็นลิงก์ ไม่ใช่ระบบอัปโหลดไฟล์
- บัญชีและ token จาก mock server ใช้กับ backend จริงไม่ได้ ต้องสมัครใหม่

## ทดสอบ

```powershell
go vet ./...
go test -v ./...
go build ./cmd/server
```

ทดสอบ integration ด้วย PostgreSQL **แยกสำหรับทดสอบเท่านั้น**:

```powershell
$env:TEST_DATABASE_URL = 'postgres://postgres:postgres@localhost:5432/bookreview_test?sslmode=disable'
go test -v ./...
```

Integration test จะ reset schema `bookreview` ในฐานข้อมูลทดสอบ และปฏิเสธ database ที่ชื่อไม่ได้ลงท้าย `_test` ห้ามชี้ไป production หากไม่ได้ตั้งตัวแปร test นี้ unit tests จะรัน แต่ integration test จะ skip

GitHub Actions เตรียม PostgreSQL ให้แล้วและรัน `go vet`, `go test -race -cover`, `go build` ทุก push/PR

## โครงสร้าง

```text
cmd/server/main.go        startup, migrations, seed, admin CLI, graceful shutdown
internal/api/api.go       routing, auth, validation, SQL, business logic
internal/api/api_test.go  unit + PostgreSQL integration tests
migrations/001_init.sql  private schema, tables, constraints, indexes
migrations/books.json    optional sample catalog from frontend
Dockerfile               Go build + runtime without root
render.yaml              GitHub-connected Render Blueprint
```

แหล่งอ้างอิงการ deploy: [Render Blueprint](https://render.com/docs/blueprint-spec), [Render Docker](https://render.com/docs/docker), [Supabase PostgreSQL connections](https://supabase.com/docs/guides/database/connecting-to-postgres)

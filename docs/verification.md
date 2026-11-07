# ผลการตรวจในเครื่อง — 7 พฤศจิกายน 2026

Environment: Windows amd64, Go 1.27.3, PostgreSQL 17.6 แบบ local ชั่วคราว (ไม่มีการใช้ Supabase production)

- `go build ./...` และ build executable `./cmd/server`: ผ่าน
- `go vet ./...`: ผ่าน
- Unit tests: validation, JWT tampering, CORS allow/deny, malformed/oversized JSON: ผ่าน
- PostgreSQL integration test: ผ่าน ครอบคลุม auth/profile/password, member/admin permissions, books CRUD/search, reviews CRUD/ownership/duplicates, aggregates, cascade deletion และอ่านข้อมูลผ่าน server instance ใหม่
- Coverage package `internal/api`: **81.8%** (ไม่ใช่ coverage ของทั้งโปรเจกต์)
- CLI `-migrate`, `-seed`, `-admin-email`: ผ่านกับฐานข้อมูลทดสอบ
- HTTP smoke test กับ executable จริง: `/health` สำเร็จ และ `/api/books` คืนหนังสือ seed ครบ 6 เล่ม
- `git diff --check`: ผ่าน

Sandbox จำกัด socket จึงรัน integration และ HTTP smoke test ด้วยสิทธิ์ที่อนุญาตนอก sandbox ฐานข้อมูลทดสอบผูกเฉพาะ localhost และแยกจากข้อมูลจริง

ยังไม่ได้ทดสอบ Docker image บนเครื่องนี้ (ไม่มี Docker), ยังไม่ได้รัน GitHub Actions และยังไม่ได้ deploy Render/Supabase/Vercel ตามขอบเขตที่ผู้ใช้ให้เตรียมโค้ดและคู่มือก่อน

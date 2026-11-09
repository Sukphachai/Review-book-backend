# สัญญา API (API Contract): Book Review System

เอกสารสัญญากลางระหว่างทีม **React (Frontend)** และทีม **Go (Backend)**
Base URL สำหรับ Go Backend: `http://localhost:8080` (Local) หรือ URL บน Render

---

## 1. Authentication & Member Profile

### 1.1 สมัครสมาชิก (Register)
- **Method**: `POST`
- **Path**: `/api/auth/register`
- **Auth**: ไม่ต้องมี
- **Request Body**:
  ```json
  {
    "email": "user@example.com",
    "password": "password123",
    "displayName": "สมชาย รักอ่าน"
  }
  ```
- **Response**:
  - `201 Created`:
    ```json
    {
      "token": "jwt-token-string",
      "member": {
        "id": 1,
        "email": "user@example.com",
        "displayName": "สมชาย รักอ่าน",
        "role": "member",
        "bio": ""
      }
    }
    ```
  - `400 Bad Request`: `{ "error": "ข้อมูลไม่ครบถ้วน หรือรหัสผ่านสั้นกว่า 6 ตัวอักษร" }`
  - `409 Conflict`: `{ "error": "อีเมลนี้มีผู้ใช้งานแล้ว" }`

### 1.2 เข้าสู่ระบบ (Login)
- **Method**: `POST`
- **Path**: `/api/auth/login`
- **Auth**: ไม่ต้องมี
- **Request Body**:
  ```json
  {
    "email": "user@example.com",
    "password": "password123"
  }
  ```
- **Response**:
  - `200 OK`:
    ```json
    {
      "token": "jwt-token-string",
      "member": {
        "id": 1,
        "email": "user@example.com",
        "displayName": "สมชาย รักอ่าน",
        "role": "member"
      }
    }
    ```
  - `401 Unauthorized`: `{ "error": "อีเมลหรือรหัสผ่านไม่ถูกต้อง" }`

### 1.3 ดึงข้อมูลตนเอง (Get Profile)
- **Method**: `GET`
- **Path**: `/api/me`
- **Auth**: `Authorization: Bearer <token>`
- **Response**:
  - `200 OK`:
    ```json
    {
      "id": 1,
      "email": "user@example.com",
      "displayName": "สมชาย รักอ่าน",
      "role": "member",
      "bio": "ชอบอ่านวรรณกรรมและพัฒนาตนเอง"
    }
    ```
  - `401 Unauthorized`: `{ "error": "กรุณาเข้าสู่ระบบก่อน" }`

### 1.4 แก้ไขข้อมูลโปรไฟล์ (Update Profile)
- **Method**: `PUT`
- **Path**: `/api/me/profile`
- **Auth**: `Authorization: Bearer <token>`
- **Request Body**:
  ```json
  {
    "displayName": "สมชาย ชื่อใหม่",
    "bio": "คำอธิบายใหม่"
  }
  ```
- **Response**:
  - `200 OK`: `{ "id": 1, "displayName": "สมชาย ชื่อใหม่", "bio": "คำอธิบายใหม่", ... }`
  - `401 Unauthorized`: `{ "error": "กรุณาเข้าสู่ระบบก่อน" }`

### 1.5 เปลี่ยนรหัสผ่าน (Change Password)
- **Method**: `PUT`
- **Path**: `/api/me/password`
- **Auth**: `Authorization: Bearer <token>`
- **Request Body**:
  ```json
  {
    "currentPassword": "password123",
    "newPassword": "newpassword456"
  }
  ```
- **Response**:
  - `200 OK`: `{ "message": "เปลี่ยนรหัสผ่านเรียบร้อยแล้ว" }`
  - `400 Bad Request`: `{ "error": "รหัสผ่านใหม่ต้องมีความยาวอย่างน้อย 6 ตัวอักษร" }`
  - `401 Unauthorized`: `{ "error": "รหัสผ่านเดิมไม่ถูกต้อง" }`

---

## 2. Books (หนังสือ)

### 2.1 ดึงรายการหนังสือทั้งหมด (ค้นหา / กรอง)
- **Method**: `GET`
- **Path**: `/api/books?search=&category=`
- **Auth**: ไม่ต้องมี (Guest ดูได้)
- **Response**:
  - `200 OK`:
    ```json
    {
      "items": [
        {
          "id": 1,
          "title": "Atomic Habits เพราะชีวิตดีได้กว่าที่เป็น",
          "author": "James Clear",
          "category": "พัฒนาตนเอง",
          "coverImage": "https://images.unsplash.com/photo-1544947950-fa07a98d237f?auto=format&fit=crop&q=80&w=600",
          "description": "การเปลี่ยนแปลงเล็กๆ ที่ให้ผลลัพธ์มหาศาล",
          "price": 295,
          "rating": 4.8,
          "reviewCount": 12
        }
      ]
    }
    ```

### 2.2 ดูรายละเอียดหนังสือ
- **Method**: `GET`
- **Path**: `/api/books/:id`
- **Auth**: ไม่ต้องมี
- **Response**:
  - `200 OK`: `{ "id": 1, "title": "...", "author": "...", ... }`
  - `404 Not Found`: `{ "error": "ไม่พบหนังสือที่ต้องการ" }`

### 2.3 ดึงหมวดหมู่หนังสือ
- **Method**: `GET`
- **Path**: `/api/categories`
- **Auth**: ไม่ต้องมี
- **Response**:
  - `200 OK`:
    ```json
    {
      "items": ["ทั้งหมด", "พัฒนาตนเอง", "จิตวิทยา", "การเงินและการลงทุน", "เทคโนโลยี", "นิยายและวรรณกรรม"]
    }
    ```

---

## 3. Reviews (รีวิวหนังสือ)

### 3.1 ดึงรีวิวของหนังสือเล่มหนึ่ง
- **Method**: `GET`
- **Path**: `/api/books/:id/reviews`
- **Auth**: ไม่ต้องมี
- **Response**:
  - `200 OK`:
    ```json
    {
      "items": [
        {
          "id": 1,
          "bookId": 1,
          "memberId": 1,
          "memberName": "สมชาย รักอ่าน",
          "rating": 5,
          "comment": "เป็นหนังสือที่เปลี่ยนชีวิตจริงๆ แนะนำให้อ่าน!",
          "createdAt": "2026-11-01T10:00:00Z"
        }
      ]
    }
    ```

### 3.2 เพิ่มรีวิว (เขียนรีวิวใหม่)
- **Method**: `POST`
- **Path**: `/api/books/:id/reviews`
- **Auth**: `Authorization: Bearer <token>` (ต้อง Login)
- **Request Body**:
  ```json
  {
    "rating": 5,
    "comment": "อ่านแล้วเข้าใจง่าย นำไปปรับใช้ได้จริง"
  }
  ```
- **Response**:
  - `201 Created`: `{ "id": 2, "bookId": 1, "rating": 5, "comment": "...", "memberName": "สมชาย รักอ่าน", ... }`
  - `400 Bad Request`: `{ "error": "กรุณาให้คะแนน 1-5 ดาว และเขียนรีวิวอย่างน้อย 5 ตัวอักษร" }`
  - `401 Unauthorized`: `{ "error": "กรุณาเข้าสู่ระบบก่อนรีวิว" }`
  - `409 Conflict`: `{ "error": "คุณได้เขียนรีวิวหนังสือเล่มนี้ไปแล้ว" }`

### 3.3 ดูรีวิวทั้งหมดของตัวเอง
- **Method**: `GET`
- **Path**: `/api/me/reviews`
- **Auth**: `Authorization: Bearer <token>`
- **Response**:
  - `200 OK`:
    ```json
    {
      "items": [
        {
          "id": 1,
          "bookId": 1,
          "bookTitle": "Atomic Habits",
          "rating": 5,
          "comment": "...",
          "createdAt": "2026-11-01T10:00:00Z"
        }
      ]
    }
    ```

### 3.4 แก้ไขรีวิวของตนเอง
- **Method**: `PUT`
- **Path**: `/api/reviews/:id`
- **Auth**: `Authorization: Bearer <token>`
- **Request Body**:
  ```json
  {
    "rating": 4,
    "comment": "แก้ไขรีวิวใหม่"
  }
  ```
- **Response**:
  - `200 OK`: `{ "id": 1, "rating": 4, "comment": "แก้ไขรีวิวใหม่", ... }`
  - `403 Forbidden`: `{ "error": "คุณไม่มีสิทธิ์แก้ไขรีวิวของผู้อื่น" }`
  - `404 Not Found`: `{ "error": "ไม่พบรีวิว" }`

### 3.5 ลบรีวิว
- **Method**: `DELETE`
- **Path**: `/api/reviews/:id`
- **Auth**: `Authorization: Bearer <token>`
- **Response**:
  - `204 No Content`: (ไม่มี body)
  - `403 Forbidden`: `{ "error": "คุณไม่มีสิทธิ์ลบรีวิวนี้" }`
  - `404 Not Found`: `{ "error": "ไม่พบรีวิว" }`

---

## 4. Favorites (หนังสือเล่มโปรด)

### 4.1 ดูหนังสือเล่มโปรดทั้งหมดของตนเอง
- **Method**: `GET`
- **Path**: `/api/me/favorites`
- **Auth**: `Authorization: Bearer <token>`
- **Response**:
  - `200 OK`:
    ```json
    {
      "items": [
        {
          "id": 1,
          "title": "Atomic Habits",
          "author": "James Clear",
          "category": "พัฒนาตนเอง",
          "coverImage": "https://...",
          "price": 295,
          "rating": 4.9,
          "reviewCount": 3
        }
      ]
    }
    ```

### 4.2 ดู ID หนังสือเล่มโปรดทั้งหมดของตนเอง
- **Method**: `GET`
- **Path**: `/api/me/favorites/ids`
- **Auth**: `Authorization: Bearer <token>`
- **Response**:
  - `200 OK`: `{ "ids": [1, 3, 5] }`

### 4.3 เพิ่มหนังสือเข้าเล่มโปรด
- **Method**: `POST`
- **Path**: `/api/books/:id/favorite`
- **Auth**: `Authorization: Bearer <token>`
- **Response**:
  - `201 Created`: `{ "message": "เพิ่มหนังสือเล่มโปรดเรียบร้อยแล้ว", "bookId": 1 }`

### 4.4 ยกเลิกหนังสือเล่มโปรด
- **Method**: `DELETE`
- **Path**: `/api/books/:id/favorite`
- **Auth**: `Authorization: Bearer <token>`
- **Response**:
  - `200 OK`: `{ "message": "ยกเลิกหนังสือเล่มโปรดเรียบร้อยแล้ว", "bookId": 1 }`

---

## 5. Admin Management (เฉพาะบทบาท admin)

### 4.1 สถิติระบบหลังบ้าน (Dashboard Stats)
- **Method**: `GET`
- **Path**: `/api/admin/stats`
- **Auth**: `Authorization: Bearer <token>` (role: admin)
- **Response**:
  - `200 OK`:
    ```json
    {
      "totalBooks": 24,
      "totalReviews": 150,
      "totalUsers": 48,
      "avgRating": 4.6
    }
    ```
  - `403 Forbidden`: `{ "error": "เฉพาะผู้ดูแลระบบเท่านั้น" }`

### 4.2 เพิ่มหนังสือใหม่
- **Method**: `POST`
- **Path**: `/api/admin/books`
- **Auth**: `Authorization: Bearer <token>` (role: admin)
- **Request Body**:
  ```json
  {
    "title": "Clean Code",
    "author": "Robert C. Martin",
    "category": "เทคโนโลยี",
    "coverImage": "https://example.com/cover.jpg",
    "description": "คู่มือการเขียนโค้ดที่สะอาดและยั่งยืน",
    "price": 450
  }
  ```
- **Response**:
  - `201 Created`: `{ "id": 25, "title": "Clean Code", ... }`
  - `400 Bad Request`: `{ "error": "กรุณากรอกข้อมูลหนังสือให้ครบถ้วน" }`
  - `403 Forbidden`: `{ "error": "เฉพาะผู้ดูแลระบบเท่านั้น" }`

### 4.3 แก้ไขหนังสือ
- **Method**: `PUT`
- **Path**: `/api/admin/books/:id`
- **Auth**: `Authorization: Bearer <token>` (role: admin)
- **Response**:
  - `200 OK`: `{ "id": 25, ... }`
  - `403 Forbidden`: `{ "error": "เฉพาะผู้ดูแลระบบเท่านั้น" }`
  - `404 Not Found`: `{ "error": "ไม่พบหนังสือ" }`

### 4.4 ลบหนังสือ
- **Method**: `DELETE`
- **Path**: `/api/admin/books/:id`
- **Auth**: `Authorization: Bearer <token>` (role: admin)
- **Response**:
  - `204 No Content`
  - `403 Forbidden`: `{ "error": "เฉพาะผู้ดูแลระบบเท่านั้น" }`
  - `404 Not Found`: `{ "error": "ไม่พบหนังสือ" }`

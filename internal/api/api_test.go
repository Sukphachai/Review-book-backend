package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Sukphachai/Review-book-backend/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestValidation(t *testing.T) {
	for _, v := range []string{"12345", strings.Repeat("x", 73)} {
		if validPassword(v) {
			t.Fatalf("accepted invalid password")
		}
	}
	if !validPassword("รหัสผ่านดีๆ") {
		t.Fatal("Thai password rejected")
	}
	for _, v := range []reviewInput{{0, "hello"}, {6, "hello"}, {3, "abc"}} {
		if validReview(&v) == nil {
			t.Fatal("accepted invalid review")
		}
	}
	if e := validReview(&reviewInput{5, "หนังสือดีมาก"}); e != nil {
		t.Fatal(e)
	}
	for _, email := range []string{"x", "Name <a@b.com>", "a\nb@c.com"} {
		if _, e := emailValue(email); e == nil {
			t.Fatal("accepted bad email")
		}
	}
	b := Book{Title: "Title", Author: "Author", Category: "Cat", CoverImage: "javascript:alert(1)"}
	if validBook(&b) == nil {
		t.Fatal("accepted unsafe URL")
	}
}
func TestTokenValidation(t *testing.T) {
	s := &Server{cfg: Config{Secret: strings.Repeat("s", 32)}}
	token := s.sign(Member{ID: 1})
	parts := strings.Split(token, ".")
	sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
	sig[0] ^= 1
	tampered := parts[0] + "." + parts[1] + "." + base64.RawURLEncoding.EncodeToString(sig)
	for _, v := range []string{"", "garbage", tampered, "e30." + parts[1] + "." + parts[2]} {
		r := httptest.NewRequest("GET", "/api/me", nil)
		r.Header.Set("Authorization", "Bearer "+v)
		if _, e := s.authenticate(r); e == nil {
			t.Fatal("invalid token accepted")
		}
	}
	payload, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var c claims
	_ = json.Unmarshal(payload, &c)
	if c.Subject != 1 || c.Expires < time.Now().Add(23*time.Hour).Unix() {
		t.Fatal("incorrect token claims")
	}
}
func TestCORSAndInvalidJSON(t *testing.T) {
	h := New(nil, Config{Secret: strings.Repeat("s", 32), Origins: []string{"https://reader.vercel.app"}})
	for _, tc := range []struct {
		origin string
		status int
	}{{"https://reader.vercel.app", 204}, {"https://evil.example", 403}} {
		r := httptest.NewRequest("OPTIONS", "/api/books", nil)
		r.Header.Set("Origin", tc.origin)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("CORS: %d", w.Code)
		}
		if tc.status == 204 && w.Header().Get("Access-Control-Allow-Origin") != tc.origin {
			t.Fatal("missing allowed origin")
		}
	}
	for _, body := range []string{`{`, `{} {}`, strings.Repeat("x", 70000)} {
		r := httptest.NewRequest("POST", "/api/auth/register", strings.NewReader(body))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 400 {
			t.Fatalf("bad JSON returned %d", w.Code)
		}
	}
	r := httptest.NewRequest("GET", "/missing", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 404 || !strings.Contains(w.Body.String(), "error") {
		t.Fatal("missing JSON 404")
	}
}

// This test resets ONLY a dedicated database whose name ends with _test.
func TestIntegration(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to a disposable PostgreSQL database ending in _test")
	}
	cfg, e := pgxpool.ParseConfig(dsn)
	if e != nil {
		t.Fatal(e)
	}
	if !strings.HasSuffix(cfg.ConnConfig.Database, "_test") {
		t.Fatal("refusing to reset a database without the _test suffix")
	}
	ctx := context.Background()
	db, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	if _, e = db.Exec(ctx, "DROP SCHEMA IF EXISTS bookreview CASCADE"); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 2; i++ {
		if _, e = db.Exec(ctx, migrations.Initial); e != nil {
			t.Fatal(e)
		}
	}
	conf := Config{Secret: strings.Repeat("s", 32), Origins: []string{"http://localhost:3000"}}
	h := New(db, conf)
	call := func(method, path, token string, body any, status int) map[string]any {
		t.Helper()
		var buf bytes.Buffer
		if body != nil {
			if e := json.NewEncoder(&buf).Encode(body); e != nil {
				t.Fatal(e)
			}
		}
		r := httptest.NewRequest(method, path, &buf)
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("%s %s: got %d want %d: %s", method, path, w.Code, status, w.Body.String())
		}
		data := map[string]any{}
		if status != 204 {
			if e := json.Unmarshal(w.Body.Bytes(), &data); e != nil {
				t.Fatal(e)
			}
		}
		return data
	}
	reg := func(email string) string {
		t.Helper()
		v := call("POST", "/api/auth/register", "", map[string]any{"email": email, "password": "password123", "displayName": "นักอ่านทดสอบ", "role": "admin"}, 201)
		m := v["member"].(map[string]any)
		if m["role"] != "member" {
			t.Fatal("registration elevated role")
		}
		if _, ok := m["password_hash"]; ok {
			t.Fatal("hash leaked")
		}
		return v["token"].(string)
	}
	admin := reg("admin@example.com")
	member := reg("member@example.com")
	other := reg("other@example.com")
	call("POST", "/api/auth/register", "", map[string]any{"email": "MEMBER@example.com", "password": "password123", "displayName": "Duplicate"}, 409)
	call("POST", "/api/auth/login", "", map[string]any{"email": "member@example.com", "password": "wrong"}, 401)
	call("POST", "/api/auth/login", "", map[string]any{"email": "nobody@example.com", "password": "wrong"}, 401)
	login := call("POST", "/api/auth/login", "", map[string]any{"email": "MEMBER@example.com", "password": "password123"}, 200)
	member = login["token"].(string)
	call("GET", "/api/me", "", nil, 401)
	call("GET", "/api/admin/stats", member, nil, 403)
	if _, e = db.Exec(ctx, "UPDATE bookreview.members SET role='admin' WHERE email='admin@example.com'"); e != nil {
		t.Fatal(e)
	}
	bookBody := map[string]any{"title": "หนังสือทดสอบ", "author": "Test author", "category": "เทคโนโลยี", "description": "Description", "coverImage": "https://example.com/book.jpg", "price": 295}
	call("POST", "/api/admin/books", member, bookBody, 403)
	b := call("POST", "/api/admin/books", admin, bookBody, 201)
	bookID := int(b["id"].(float64))
	bookPath := fmt.Sprintf("/api/books/%d", bookID)
	call("GET", "/health", "", nil, 200)
	list := call("GET", "/api/books?search=author&category=เทคโนโลยี", "", nil, 200)
	if len(list["items"].([]any)) != 1 {
		t.Fatal("search mismatch")
	}
	empty := call("GET", "/api/books?search=missing", "", nil, 200)
	if len(empty["items"].([]any)) != 0 {
		t.Fatal("empty search mismatch")
	}
	call("GET", "/api/categories", "", nil, 200)
	call("GET", "/api/books/999999", "", nil, 404)
	call("GET", "/api/books/nope", "", nil, 400)
	call("POST", bookPath+"/reviews", member, map[string]any{"rating": 1.5, "comment": "hello"}, 400)
	call("POST", bookPath+"/reviews", member, map[string]any{"rating": 6, "comment": "hello"}, 400)
	call("POST", "/api/books/999999/reviews", member, map[string]any{"rating": 5, "comment": "hello"}, 404)
	rev := call("POST", bookPath+"/reviews", member, map[string]any{"rating": 5, "comment": "หนังสือดีมาก"}, 201)
	revPath := fmt.Sprintf("/api/reviews/%d", int(rev["id"].(float64)))
	call("POST", bookPath+"/reviews", member, map[string]any{"rating": 5, "comment": "duplicate"}, 409)
	second := call("POST", bookPath+"/reviews", other, map[string]any{"rating": 3, "comment": "อ่านได้เพลิน"}, 201)
	b = call("GET", bookPath, "", nil, 200)
	if b["rating"] != float64(4) || b["reviewCount"] != float64(2) {
		t.Fatalf("incorrect aggregates: %v", b)
	}
	call("PUT", revPath, other, map[string]any{"rating": 1}, 403)
	call("DELETE", revPath, other, nil, 403)
	call("PUT", revPath, member, map[string]any{"rating": 1}, 200)
	b = call("GET", bookPath, "", nil, 200)
	if b["rating"] != float64(2) {
		t.Fatal("rating not updated")
	}
	call("PUT", "/api/me/profile", member, map[string]any{"displayName": "ชื่อใหม่", "bio": "รักการอ่าน"}, 200)
	mine := call("GET", "/api/me/reviews", member, nil, 200)
	rv := mine["items"].([]any)[0].(map[string]any)
	if rv["memberName"] != "ชื่อใหม่" || rv["bookTitle"] != "หนังสือทดสอบ" {
		t.Fatal("review join mismatch")
	}
	call("GET", bookPath+"/reviews", "", nil, 200)
	stats := call("GET", "/api/admin/stats", admin, nil, 200)
	if stats["totalUsers"] != float64(3) || stats["avgRating"] != float64(2) {
		t.Fatal("stats mismatch")
	}
	call("PUT", fmt.Sprintf("/api/admin/books/%d", bookID), admin, map[string]any{"price": 300}, 200)
	call("DELETE", revPath, member, nil, 204)
	b = call("GET", bookPath, "", nil, 200)
	if b["rating"] != float64(3) {
		t.Fatal("rating after deletion mismatch")
	}
	call("DELETE", fmt.Sprintf("/api/reviews/%d", int(second["id"].(float64))), admin, nil, 204)
	b = call("GET", bookPath, "", nil, 200)
	if b["rating"] != float64(0) {
		t.Fatal("last review delete mismatch")
	}
	// A new server instance must still see data and accept the existing token.
	h = New(db, conf)
	call("GET", bookPath, "", nil, 200)
	call("GET", "/api/me", member, nil, 200)
	call("PUT", "/api/me/password", member, map[string]any{"currentPassword": "wrong", "newPassword": "newpassword123"}, 401)
	call("PUT", "/api/me/password", member, map[string]any{"currentPassword": "password123", "newPassword": "newpassword123"}, 200)
	call("GET", "/api/me", member, nil, 401)
	call("POST", "/api/auth/login", "", map[string]any{"email": "member@example.com", "password": "password123"}, 401)
	call("POST", "/api/auth/login", "", map[string]any{"email": "member@example.com", "password": "newpassword123"}, 200)
	call("POST", bookPath+"/reviews", other, map[string]any{"rating": 5, "comment": "another review"}, 201)
	call("DELETE", fmt.Sprintf("/api/admin/books/%d", bookID), admin, nil, 204)
	var n int
	if e = db.QueryRow(ctx, "SELECT count(*) FROM bookreview.reviews").Scan(&n); e != nil || n != 0 {
		t.Fatal("reviews not cascaded")
	}
	call("GET", bookPath, "", nil, 404)
}

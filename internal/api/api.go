package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type Config struct {
	Secret  string
	Origins []string
}
type Server struct {
	db       *pgxpool.Pool
	cfg      Config
	mu       sync.Mutex
	attempts map[string]attempt
}
type attempt struct {
	count int
	until time.Time
}
type Member struct {
	ID          int64  `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
	Role        string `json:"role"`
	Bio         string `json:"bio"`
	Hash        string `json:"-"`
	Version     int    `json:"-"`
}
type Book struct {
	ID          int64   `json:"id"`
	Title       string  `json:"title"`
	Author      string  `json:"author"`
	Category    string  `json:"category"`
	CoverImage  string  `json:"coverImage"`
	Description string  `json:"description"`
	Price       float64 `json:"price"`
	Rating      float64 `json:"rating"`
	ReviewCount int     `json:"reviewCount"`
}
type Review struct {
	ID         int64     `json:"id"`
	BookID     int64     `json:"bookId"`
	MemberID   int64     `json:"memberId"`
	MemberName string    `json:"memberName"`
	Rating     int       `json:"rating"`
	Comment    string    `json:"comment"`
	CreatedAt  time.Time `json:"createdAt"`
	BookTitle  string    `json:"bookTitle"`
	CoverImage string    `json:"coverImage"`
}
type problem struct {
	status  int
	message string
}

func (p problem) Error() string             { return p.message }
func fail(status int, message string) error { return problem{status, message} }

type endpoint func(http.ResponseWriter, *http.Request) error
type memberKey struct{}

const memberCols = `id,email,display_name,role,bio,password_hash,token_version`
const bookQuery = `SELECT b.id,b.title,b.author,b.category,b.cover_image,b.description,b.price::float8,
 COALESCE(round(avg(r.rating),1),0)::float8,count(r.id)::int
 FROM bookreview.books b LEFT JOIN bookreview.reviews r ON r.book_id=b.id `
const reviewQuery = `SELECT r.id,r.book_id,r.member_id,m.display_name,r.rating,r.comment,r.created_at,b.title,b.cover_image
 FROM bookreview.reviews r JOIN bookreview.members m ON m.id=r.member_id JOIN bookreview.books b ON b.id=r.book_id `

var categories = []string{"ทั้งหมด", "พัฒนาตนเอง", "จิตวิทยา", "การเงินและการลงทุน", "เทคโนโลยี", "นิยายและวรรณกรรม"}

func New(db *pgxpool.Pool, cfg Config) http.Handler {
	s := &Server{db: db, cfg: cfg, attempts: make(map[string]attempt)}
	mux := http.NewServeMux()
	add := func(pattern string, auth, admin bool, h endpoint) { mux.HandleFunc(pattern, s.wrap(auth, admin, h)) }
	add("GET /health", false, false, s.health)
	add("POST /api/auth/register", false, false, s.register)
	add("POST /api/auth/login", false, false, s.login)
	add("GET /api/me", true, false, func(w http.ResponseWriter, r *http.Request) error { return reply(w, 200, current(r)) })
	add("PUT /api/me/profile", true, false, s.profile)
	add("PUT /api/me/password", true, false, s.password)
	add("GET /api/books", false, false, s.books)
	add("GET /api/books/{id}", false, false, s.book)
	add("GET /api/categories", false, false, s.categoryList)
	add("GET /api/books/{id}/reviews", false, false, s.reviews)
	add("POST /api/books/{id}/reviews", true, false, s.createReview)
	add("GET /api/me/reviews", true, false, s.myReviews)
	add("PUT /api/reviews/{id}", true, false, s.updateReview)
	add("DELETE /api/reviews/{id}", true, false, s.deleteReview)
	add("GET /api/admin/stats", true, true, s.stats)
	add("POST /api/admin/books", true, true, s.createBook)
	add("PUT /api/admin/books/{id}", true, true, s.updateBook)
	add("DELETE /api/admin/books/{id}", true, true, s.deleteBook)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, fail(404, "ไม่พบ API ที่ต้องการ"))
	})
	return s.middleware(mux)
}
func reply(w http.ResponseWriter, status int, v any) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if status != 204 {
		return json.NewEncoder(w).Encode(v)
	}
	return nil
}
func writeError(w http.ResponseWriter, err error) {
	p := problem{500, "เกิดข้อผิดพลาดภายในระบบ"}
	if !errors.As(err, &p) {
		var pe *pgconn.PgError
		if errors.As(err, &pe) {
			switch pe.Code {
			case "23505":
				p = problem{409, "ข้อมูลนี้มีอยู่แล้ว"}
			case "23503":
				p = problem{404, "ไม่พบข้อมูลที่อ้างถึง"}
			case "23514", "22003":
				p = problem{400, "ข้อมูลไม่ถูกต้อง"}
			}
		}
		// Do not log SQL arguments, passwords, tokens or database connection strings.
		if p.status == 500 {
			log.Printf("request failed (%T)", err)
		}
	}
	_ = reply(w, p.status, map[string]string{"error": p.message})
}
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	d := json.NewDecoder(r.Body)
	if err := d.Decode(v); err != nil {
		return fail(400, "ข้อมูล JSON ไม่ถูกต้องหรือมีขนาดใหญ่เกินไป")
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return fail(400, "ส่ง JSON ได้ครั้งละหนึ่งรายการ")
	}
	return nil
}
func pathID(r *http.Request) (int64, error) {
	id, e := strconv.ParseInt(r.PathValue("id"), 10, 32)
	if e != nil || id <= 0 {
		return 0, fail(400, "รหัสข้อมูลไม่ถูกต้อง")
	}
	return id, nil
}
func current(r *http.Request) Member { return r.Context().Value(memberKey{}).(Member) }
func scanMember(row pgx.Row) (m Member, err error) {
	err = row.Scan(&m.ID, &m.Email, &m.DisplayName, &m.Role, &m.Bio, &m.Hash, &m.Version)
	return
}
func scanBook(row pgx.Row) (b Book, err error) {
	err = row.Scan(&b.ID, &b.Title, &b.Author, &b.Category, &b.CoverImage, &b.Description, &b.Price, &b.Rating, &b.ReviewCount)
	return
}
func scanReview(row pgx.Row) (v Review, err error) {
	err = row.Scan(&v.ID, &v.BookID, &v.MemberID, &v.MemberName, &v.Rating, &v.Comment, &v.CreatedAt, &v.BookTitle, &v.CoverImage)
	return
}

type claims struct {
	Subject int64 `json:"sub"`
	Version int   `json:"ver"`
	Expires int64 `json:"exp"`
}

var jwtHeader = base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))

func (s *Server) sign(m Member) string {
	payload, _ := json.Marshal(claims{m.ID, m.Version, time.Now().Add(24 * time.Hour).Unix()})
	msg := jwtHeader + "." + base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte(s.cfg.Secret))
	mac.Write([]byte(msg))
	return msg + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func (s *Server) authenticate(r *http.Request) (Member, error) {
	invalid := fail(401, "กรุณาเข้าสู่ระบบใหม่ Token ไม่ถูกต้องหรือหมดอายุ")
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") || len(header) > 2048 {
		return Member{}, invalid
	}
	parts := strings.Split(strings.TrimPrefix(header, "Bearer "), ".")
	if len(parts) != 3 || parts[0] != jwtHeader {
		return Member{}, invalid
	}
	signature, e := base64.RawURLEncoding.DecodeString(parts[2])
	if e != nil {
		return Member{}, invalid
	}
	mac := hmac.New(sha256.New, []byte(s.cfg.Secret))
	mac.Write([]byte(parts[0] + "." + parts[1]))
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return Member{}, invalid
	}
	payload, e := base64.RawURLEncoding.DecodeString(parts[1])
	if e != nil {
		return Member{}, invalid
	}
	var c claims
	if json.Unmarshal(payload, &c) != nil || c.Subject <= 0 || c.Expires <= time.Now().Unix() {
		return Member{}, invalid
	}
	m, e := scanMember(s.db.QueryRow(r.Context(), "SELECT "+memberCols+" FROM bookreview.members WHERE id=$1", c.Subject))
	if errors.Is(e, pgx.ErrNoRows) {
		return Member{}, invalid
	}
	if e != nil {
		return Member{}, e
	}
	if m.Version != c.Version {
		return Member{}, invalid
	}
	return m, nil
}
func (s *Server) wrap(auth, admin bool, h endpoint) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if auth {
			m, e := s.authenticate(r)
			if e != nil {
				writeError(w, e)
				return
			}
			if admin && m.Role != "admin" {
				writeError(w, fail(403, "เฉพาะผู้ดูแลระบบเท่านั้น"))
				return
			}
			r = r.WithContext(context.WithValue(r.Context(), memberKey{}, m))
		}
		if e := h(w, r); e != nil {
			writeError(w, e)
		}
	}
}
func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recover() != nil {
				writeError(w, fail(500, "เกิดข้อผิดพลาดภายในระบบ"))
			}
		}()
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Add("Vary", "Origin")
		origin := r.Header.Get("Origin")
		if origin != "" {
			allowed := false
			for _, v := range s.cfg.Origins {
				if origin == v {
					allowed = true
					break
				}
			}
			if !allowed {
				writeError(w, fail(403, "Origin ไม่ได้รับอนุญาต"))
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		}
		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
func (s *Server) limit(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for k, v := range s.attempts {
		if now.After(v.until) {
			delete(s.attempts, k)
		}
	}
	v := s.attempts[key]
	if v.until.IsZero() {
		v.until = now.Add(15 * time.Minute)
	}
	v.count++
	s.attempts[key] = v
	if v.count > 20 {
		return fail(429, "ลองหลายครั้งเกินไป กรุณารอ 15 นาที")
	}
	return nil
}
func validPassword(v string) bool { return utf8.RuneCountInString(v) >= 6 && len(v) <= 72 }
func validText(v string, min, max int) bool {
	n := utf8.RuneCountInString(strings.TrimSpace(v))
	return n >= min && n <= max
}
func emailValue(v string) (string, error) {
	v = strings.ToLower(strings.TrimSpace(v))
	a, e := mail.ParseAddress(v)
	if e != nil || a.Address != v || len(v) > 254 {
		return "", fail(400, "อีเมลไม่ถูกต้อง")
	}
	return v, nil
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) error {
	var in struct{ Email, Password, DisplayName string }
	if e := decode(w, r, &in); e != nil {
		return e
	}
	email, e := emailValue(in.Email)
	if e != nil {
		return e
	}
	if !validPassword(in.Password) || !validText(in.DisplayName, 1, 100) {
		return fail(400, "กรุณาระบุชื่อ และรหัสผ่านอย่างน้อย 6 ตัวอักษร (ไม่เกิน 72 bytes)")
	}
	if e = s.limit("register:" + email); e != nil {
		return e
	}
	hash, e := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if e != nil {
		return e
	}
	m, e := scanMember(s.db.QueryRow(r.Context(), `INSERT INTO bookreview.members(email,password_hash,display_name) VALUES($1,$2,$3) RETURNING `+memberCols, email, string(hash), strings.TrimSpace(in.DisplayName)))
	if e != nil {
		return e
	}
	return reply(w, 201, map[string]any{"token": s.sign(m), "member": m})
}

// Precomputed bcrypt hash makes an unknown-email login perform a password comparison too.
const dummyHash = "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"

func (s *Server) login(w http.ResponseWriter, r *http.Request) error {
	var in struct{ Email, Password string }
	if e := decode(w, r, &in); e != nil {
		return e
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if e := s.limit("login:" + email); e != nil {
		return e
	}
	if len(email) > 254 || len(in.Password) > 72 {
		return fail(401, "อีเมลหรือรหัสผ่านไม่ถูกต้อง")
	}
	m, e := scanMember(s.db.QueryRow(r.Context(), "SELECT "+memberCols+" FROM bookreview.members WHERE email=$1", email))
	if errors.Is(e, pgx.ErrNoRows) {
		_ = bcrypt.CompareHashAndPassword([]byte(dummyHash), []byte(in.Password))
		return fail(401, "อีเมลหรือรหัสผ่านไม่ถูกต้อง")
	}
	if e != nil {
		return e
	}
	if bcrypt.CompareHashAndPassword([]byte(m.Hash), []byte(in.Password)) != nil {
		return fail(401, "อีเมลหรือรหัสผ่านไม่ถูกต้อง")
	}
	return reply(w, 200, map[string]any{"token": s.sign(m), "member": m})
}
func (s *Server) profile(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		DisplayName string
		Bio         string
	}
	if e := decode(w, r, &in); e != nil {
		return e
	}
	if !validText(in.DisplayName, 1, 100) || !validText(in.Bio, 0, 2000) {
		return fail(400, "ชื่อหรือคำอธิบายไม่ถูกต้อง")
	}
	m, e := scanMember(s.db.QueryRow(r.Context(), `UPDATE bookreview.members SET display_name=$1,bio=$2 WHERE id=$3 RETURNING `+memberCols, strings.TrimSpace(in.DisplayName), strings.TrimSpace(in.Bio), current(r).ID))
	if e != nil {
		return e
	}
	return reply(w, 200, m)
}
func (s *Server) password(w http.ResponseWriter, r *http.Request) error {
	var in struct{ CurrentPassword, NewPassword string }
	if e := decode(w, r, &in); e != nil {
		return e
	}
	m := current(r)
	if e := s.limit(fmt.Sprintf("password:%d", m.ID)); e != nil {
		return e
	}
	if bcrypt.CompareHashAndPassword([]byte(m.Hash), []byte(in.CurrentPassword)) != nil {
		return fail(401, "รหัสผ่านเดิมไม่ถูกต้อง")
	}
	if !validPassword(in.NewPassword) {
		return fail(400, "รหัสผ่านใหม่ต้องมีอย่างน้อย 6 ตัวอักษร และไม่เกิน 72 bytes")
	}
	hash, e := bcrypt.GenerateFromPassword([]byte(in.NewPassword), bcrypt.DefaultCost)
	if e != nil {
		return e
	}
	tag, e := s.db.Exec(r.Context(), `UPDATE bookreview.members SET password_hash=$1,token_version=token_version+1 WHERE id=$2 AND token_version=$3`, string(hash), m.ID, m.Version)
	if e != nil {
		return e
	}
	if tag.RowsAffected() == 0 {
		return fail(401, "กรุณาเข้าสู่ระบบใหม่")
	}
	return reply(w, 200, map[string]string{"message": "เปลี่ยนรหัสผ่านเรียบร้อยแล้ว กรุณาเข้าสู่ระบบใหม่"})
}
func (s *Server) health(w http.ResponseWriter, r *http.Request) error {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if e := s.db.Ping(ctx); e != nil {
		return fail(503, "Database unavailable")
	}
	return reply(w, 200, map[string]string{"status": "ok"})
}
func (s *Server) books(w http.ResponseWriter, r *http.Request) error {
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	cat := r.URL.Query().Get("category")
	if cat == "ทั้งหมด" {
		cat = ""
	}
	if len(search) > 500 || len(cat) > 300 {
		return fail(400, "คำค้นหายาวเกินไป")
	}
	rows, e := s.db.Query(r.Context(), bookQuery+`WHERE ($1='' OR strpos(lower(b.title||' '||b.author||' '||b.description),lower($1))>0) AND ($2='' OR b.category=$2) GROUP BY b.id ORDER BY b.id DESC`, search, cat)
	if e != nil {
		return e
	}
	defer rows.Close()
	items := []Book{}
	for rows.Next() {
		b, e := scanBook(rows)
		if e != nil {
			return e
		}
		items = append(items, b)
	}
	if e = rows.Err(); e != nil {
		return e
	}
	return reply(w, 200, map[string]any{"items": items})
}
func (s *Server) getBook(ctx context.Context, id int64) (Book, error) {
	b, e := scanBook(s.db.QueryRow(ctx, bookQuery+" WHERE b.id=$1 GROUP BY b.id", id))
	if errors.Is(e, pgx.ErrNoRows) {
		return b, fail(404, "ไม่พบหนังสือที่ต้องการ")
	}
	return b, e
}
func (s *Server) book(w http.ResponseWriter, r *http.Request) error {
	id, e := pathID(r)
	if e != nil {
		return e
	}
	b, e := s.getBook(r.Context(), id)
	if e != nil {
		return e
	}
	return reply(w, 200, b)
}
func (s *Server) categoryList(w http.ResponseWriter, r *http.Request) error {
	rows, e := s.db.Query(r.Context(), "SELECT DISTINCT category FROM bookreview.books ORDER BY category")
	if e != nil {
		return e
	}
	defer rows.Close()
	items := append([]string{}, categories...)
	seen := map[string]bool{}
	for _, v := range items {
		seen[v] = true
	}
	for rows.Next() {
		var v string
		if e = rows.Scan(&v); e != nil {
			return e
		}
		if !seen[v] {
			items = append(items, v)
			seen[v] = true
		}
	}
	if e = rows.Err(); e != nil {
		return e
	}
	return reply(w, 200, map[string]any{"items": items})
}
func validBook(b *Book) error {
	b.Title = strings.TrimSpace(b.Title)
	b.Author = strings.TrimSpace(b.Author)
	b.Category = strings.TrimSpace(b.Category)
	if !validText(b.Title, 1, 300) || !validText(b.Author, 1, 200) || !validText(b.Category, 1, 100) || !validText(b.Description, 0, 10000) || b.Price < 0 || b.Price > 9999999999.99 {
		return fail(400, "ข้อมูลหนังสือไม่ถูกต้อง กรุณาระบุชื่อ ผู้แต่ง หมวดหมู่ และราคาไม่ติดลบ")
	}
	if len(b.CoverImage) > 2000 {
		return fail(400, "URL รูปภาพยาวเกินไป")
	}
	if b.CoverImage != "" {
		u, e := url.Parse(b.CoverImage)
		if e != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
			return fail(400, "URL รูปภาพต้องเป็น http หรือ https")
		}
	}
	return nil
}
func (s *Server) createBook(w http.ResponseWriter, r *http.Request) error {
	var b Book
	if e := decode(w, r, &b); e != nil {
		return e
	}
	if e := validBook(&b); e != nil {
		return e
	}
	e := s.db.QueryRow(r.Context(), `INSERT INTO bookreview.books(title,author,category,cover_image,description,price) VALUES($1,$2,$3,$4,$5,$6) RETURNING id`, b.Title, b.Author, b.Category, b.CoverImage, b.Description, b.Price).Scan(&b.ID)
	if e != nil {
		return e
	}
	b.Rating = 0
	b.ReviewCount = 0
	return reply(w, 201, b)
}
func (s *Server) updateBook(w http.ResponseWriter, r *http.Request) error {
	id, e := pathID(r)
	if e != nil {
		return e
	}
	b, e := s.getBook(r.Context(), id)
	if e != nil {
		return e
	}
	// Decode onto current values to preserve omitted fields. Identity is always from the URL.
	if e = decode(w, r, &b); e != nil {
		return e
	}
	b.ID = id
	if e = validBook(&b); e != nil {
		return e
	}
	tag, e := s.db.Exec(r.Context(), `UPDATE bookreview.books SET title=$1,author=$2,category=$3,cover_image=$4,description=$5,price=$6 WHERE id=$7`, b.Title, b.Author, b.Category, b.CoverImage, b.Description, b.Price, id)
	if e != nil {
		return e
	}
	if tag.RowsAffected() == 0 {
		return fail(404, "ไม่พบหนังสือ")
	}
	b, e = s.getBook(r.Context(), id)
	if e != nil {
		return e
	}
	return reply(w, 200, b)
}
func (s *Server) deleteBook(w http.ResponseWriter, r *http.Request) error {
	id, e := pathID(r)
	if e != nil {
		return e
	}
	tag, e := s.db.Exec(r.Context(), "DELETE FROM bookreview.books WHERE id=$1", id)
	if e != nil {
		return e
	}
	if tag.RowsAffected() == 0 {
		return fail(404, "ไม่พบหนังสือ")
	}
	return reply(w, 204, nil)
}
func (s *Server) listReviews(w http.ResponseWriter, r *http.Request, condition string, id int64) error {
	rows, e := s.db.Query(r.Context(), reviewQuery+condition+" ORDER BY r.created_at DESC,r.id DESC", id)
	if e != nil {
		return e
	}
	defer rows.Close()
	items := []Review{}
	for rows.Next() {
		v, e := scanReview(rows)
		if e != nil {
			return e
		}
		items = append(items, v)
	}
	if e = rows.Err(); e != nil {
		return e
	}
	return reply(w, 200, map[string]any{"items": items})
}
func (s *Server) reviews(w http.ResponseWriter, r *http.Request) error {
	id, e := pathID(r)
	if e != nil {
		return e
	}
	if _, e = s.getBook(r.Context(), id); e != nil {
		return e
	}
	return s.listReviews(w, r, "WHERE r.book_id=$1", id)
}
func (s *Server) myReviews(w http.ResponseWriter, r *http.Request) error {
	return s.listReviews(w, r, "WHERE r.member_id=$1", current(r).ID)
}

type reviewInput struct {
	Rating  int    `json:"rating"`
	Comment string `json:"comment"`
}

func validReview(v *reviewInput) error {
	v.Comment = strings.TrimSpace(v.Comment)
	if v.Rating < 1 || v.Rating > 5 || !validText(v.Comment, 5, 5000) {
		return fail(400, "กรุณาให้คะแนน 1-5 ดาว และเขียนรีวิว 5-5000 ตัวอักษร")
	}
	return nil
}
func (s *Server) getReview(ctx context.Context, id int64) (Review, error) {
	v, e := scanReview(s.db.QueryRow(ctx, reviewQuery+"WHERE r.id=$1", id))
	if errors.Is(e, pgx.ErrNoRows) {
		return v, fail(404, "ไม่พบรีวิว")
	}
	return v, e
}
func (s *Server) createReview(w http.ResponseWriter, r *http.Request) error {
	id, e := pathID(r)
	if e != nil {
		return e
	}
	var in reviewInput
	if e = decode(w, r, &in); e != nil {
		return e
	}
	if e = validReview(&in); e != nil {
		return e
	}
	if _, e = s.getBook(r.Context(), id); e != nil {
		return e
	}
	var rid int64
	e = s.db.QueryRow(r.Context(), `INSERT INTO bookreview.reviews(book_id,member_id,rating,comment) VALUES($1,$2,$3,$4) RETURNING id`, id, current(r).ID, in.Rating, in.Comment).Scan(&rid)
	if e != nil {
		return e
	}
	v, e := s.getReview(r.Context(), rid)
	if e != nil {
		return e
	}
	return reply(w, 201, v)
}
func (s *Server) updateReview(w http.ResponseWriter, r *http.Request) error {
	id, e := pathID(r)
	if e != nil {
		return e
	}
	v, e := s.getReview(r.Context(), id)
	if e != nil {
		return e
	}
	if v.MemberID != current(r).ID {
		return fail(403, "คุณไม่มีสิทธิ์แก้ไขรีวิวของผู้อื่น")
	}
	in := reviewInput{v.Rating, v.Comment}
	if e = decode(w, r, &in); e != nil {
		return e
	}
	if e = validReview(&in); e != nil {
		return e
	}
	tag, e := s.db.Exec(r.Context(), `UPDATE bookreview.reviews SET rating=$1,comment=$2 WHERE id=$3 AND member_id=$4`, in.Rating, in.Comment, id, current(r).ID)
	if e != nil {
		return e
	}
	if tag.RowsAffected() == 0 {
		return fail(404, "ไม่พบรีวิว")
	}
	v, e = s.getReview(r.Context(), id)
	if e != nil {
		return e
	}
	return reply(w, 200, v)
}
func (s *Server) deleteReview(w http.ResponseWriter, r *http.Request) error {
	id, e := pathID(r)
	if e != nil {
		return e
	}
	v, e := s.getReview(r.Context(), id)
	if e != nil {
		return e
	}
	m := current(r)
	if v.MemberID != m.ID && m.Role != "admin" {
		return fail(403, "คุณไม่มีสิทธิ์ลบรีวิวนี้")
	}
	tag, e := s.db.Exec(r.Context(), "DELETE FROM bookreview.reviews WHERE id=$1 AND (member_id=$2 OR $3)", id, m.ID, m.Role == "admin")
	if e != nil {
		return e
	}
	if tag.RowsAffected() == 0 {
		return fail(404, "ไม่พบรีวิว")
	}
	return reply(w, 204, nil)
}
func (s *Server) stats(w http.ResponseWriter, r *http.Request) error {
	var books, reviews, users int
	var avg float64
	e := s.db.QueryRow(r.Context(), `SELECT (SELECT count(*) FROM bookreview.books),(SELECT count(*) FROM bookreview.reviews),(SELECT count(*) FROM bookreview.members),(SELECT COALESCE(round(avg(rating),1),0)::float8 FROM bookreview.reviews)`).Scan(&books, &reviews, &users, &avg)
	if e != nil {
		return e
	}
	return reply(w, 200, map[string]any{"totalBooks": books, "totalReviews": reviews, "totalUsers": users, "avgRating": avg})
}

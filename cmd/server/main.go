package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Sukphachai/Review-book-backend/internal/api"
	"github.com/Sukphachai/Review-book-backend/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
func run() error {
	migrate := flag.Bool("migrate", false, "Apply migrations and exit")
	seed := flag.Bool("seed", false, "Insert sample books into an empty catalog and exit")
	admin := flag.String("admin-email", "", "Promote an existing registered member and exit")
	flag.Parse()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	cfg, e := pgxpool.ParseConfig(dsn)
	if e != nil {
		return fmt.Errorf("invalid DATABASE_URL (check connection string format)")
	}
	cfg.MaxConns = 5
	cfg.MinConns = 0
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.ConnConfig.ConnectTimeout = 10 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	db, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		return fmt.Errorf("cannot initialize database pool")
	}
	defer db.Close()
	if e = db.Ping(ctx); e != nil {
		return fmt.Errorf("cannot connect to database: check host, password, SSL and network access")
	}
	if *migrate || os.Getenv("AUTO_MIGRATE") == "true" {
		tx, e := db.Begin(ctx)
		if e != nil {
			return e
		}
		defer tx.Rollback(context.Background())
		if _, e = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(87202341)"); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, migrations.Initial); e != nil {
			return fmt.Errorf("schema migration failed (%T)", e)
		}
		if e = tx.Commit(ctx); e != nil {
			return e
		}
		log.Print("Database schema ready")
	}
	if *migrate {
		return nil
	}
	if *admin != "" {
		tag, e := db.Exec(ctx, `UPDATE bookreview.members SET role='admin' WHERE email=$1`, strings.ToLower(strings.TrimSpace(*admin)))
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 1 {
			return fmt.Errorf("member not found; register this email first")
		}
		log.Print("Member promoted to admin")
		return nil
	}
	if *seed {
		var books []api.Book
		if e = json.Unmarshal(migrations.SampleBooks, &books); e != nil {
			return e
		}
		tx, e := db.Begin(ctx)
		if e != nil {
			return e
		}
		defer tx.Rollback(context.Background())
		if _, e = tx.Exec(ctx, "LOCK TABLE bookreview.books IN EXCLUSIVE MODE"); e != nil {
			return e
		}
		var n int
		if e = tx.QueryRow(ctx, "SELECT count(*) FROM bookreview.books").Scan(&n); e != nil {
			return e
		}
		if n > 0 {
			return fmt.Errorf("catalog is not empty; seed skipped to protect existing data")
		}
		for _, b := range books {
			if _, e = tx.Exec(ctx, `INSERT INTO bookreview.books(title,author,category,cover_image,description,price) VALUES($1,$2,$3,$4,$5,$6)`, b.Title, b.Author, b.Category, b.CoverImage, b.Description, b.Price); e != nil {
				return e
			}
		}
		if e = tx.Commit(ctx); e != nil {
			return e
		}
		log.Print("Sample books inserted (ratings are computed from real reviews)")
		return nil
	}
	secret := os.Getenv("JWT_SECRET")
	if len(secret) < 32 {
		return fmt.Errorf("JWT_SECRET must contain at least 32 bytes")
	}
	origins := []string{}
	for _, v := range strings.Split(os.Getenv("ALLOWED_ORIGINS"), ",") {
		if v = strings.TrimSpace(v); v != "" {
			origins = append(origins, v)
		}
	}
	if len(origins) == 0 {
		return fmt.Errorf("ALLOWED_ORIGINS is required (e.g. http://localhost:3000)")
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	server := &http.Server{Addr: ":" + port, Handler: api.New(db, api.Config{Secret: secret, Origins: origins}), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 20 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	stop, done := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer done()
	go func() {
		<-stop.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	log.Printf("Listening on 0.0.0.0:%s", port)
	if e = server.ListenAndServe(); e != nil && e != http.ErrServerClosed {
		return e
	}
	return nil
}

CREATE SCHEMA IF NOT EXISTS bookreview;
CREATE TABLE IF NOT EXISTS bookreview.members (
 id serial PRIMARY KEY,
 email text NOT NULL UNIQUE CHECK(email = lower(email)),
 password_hash text NOT NULL,
 display_name text NOT NULL,
 role text NOT NULL DEFAULT 'member' CHECK(role IN ('member','admin')),
 bio text NOT NULL DEFAULT '',
 token_version integer NOT NULL DEFAULT 0,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS bookreview.books (
 id serial PRIMARY KEY,
 title text NOT NULL,
 author text NOT NULL,
 category text NOT NULL,
 cover_image text NOT NULL DEFAULT '',
 description text NOT NULL DEFAULT '',
 price numeric(12,2) NOT NULL DEFAULT 0 CHECK(price >= 0),
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS bookreview.reviews (
 id serial PRIMARY KEY,
 book_id integer NOT NULL REFERENCES bookreview.books(id) ON DELETE CASCADE,
 member_id integer NOT NULL REFERENCES bookreview.members(id) ON DELETE CASCADE,
 rating integer NOT NULL CHECK(rating BETWEEN 1 AND 5),
 comment text NOT NULL CHECK(char_length(trim(comment)) BETWEEN 5 AND 5000),
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(book_id, member_id)
);
CREATE TABLE IF NOT EXISTS bookreview.favorites (
  id serial PRIMARY KEY,
  book_id integer NOT NULL REFERENCES bookreview.books(id) ON DELETE CASCADE,
  member_id integer NOT NULL REFERENCES bookreview.members(id) ON DELETE CASCADE,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(book_id, member_id)
);
CREATE INDEX IF NOT EXISTS reviews_member_idx ON bookreview.reviews(member_id);
CREATE INDEX IF NOT EXISTS favorites_member_idx ON bookreview.favorites(member_id);
CREATE INDEX IF NOT EXISTS books_category_idx ON bookreview.books(category);
-- Private schema: no browser/Supabase anon access. API connects using DATABASE_URL.
REVOKE ALL ON SCHEMA bookreview FROM PUBLIC;
ALTER TABLE bookreview.members ENABLE ROW LEVEL SECURITY;
ALTER TABLE bookreview.books ENABLE ROW LEVEL SECURITY;
ALTER TABLE bookreview.reviews ENABLE ROW LEVEL SECURITY;
ALTER TABLE bookreview.favorites ENABLE ROW LEVEL SECURITY;

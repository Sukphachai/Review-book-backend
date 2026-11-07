package migrations

import _ "embed"

//go:embed 001_init.sql
var Initial string

//go:embed books.json
var SampleBooks []byte

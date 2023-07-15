//go:build fts5

package main

import (
	"database/sql"
	"log"
)

var (
	db         *sql.DB
	insertStmt *sql.Stmt
	coverStmt  *sql.Stmt
	searchStmt *sql.Stmt
	listStmt   *sql.Stmt
	existsStmt *sql.Stmt
)

// openDatabase initializes the db
func openDatabase(dataSourceName string) {
	if d, err := sql.Open("sqlite3", "file:"+dataSourceName); err == nil {
		db = d
	} else {
		log.Fatalf("can't open database %s: %s", dataSourceName, err)
	}

	if _, err := db.Exec(schemaSQL); err != nil {
		log.Fatalf("can't create schema: %s", err)
	}

	if stmt, err := db.Prepare(insertSQL); err == nil {
		insertStmt = stmt
	} else {
		log.Fatalf("can't prepare insert statement: %s", err)
	}

	if stmt, err := db.Prepare(coverSQL); err == nil {
		coverStmt = stmt
	} else {
		log.Fatalf("can't prepare cover statement: %s", err)
	}

	if stmt, err := db.Prepare(searchSQL); err == nil {
		searchStmt = stmt
	} else {
		log.Fatalf("can't prepare search statement: %s", err)
	}

	if stmt, err := db.Prepare(listSQL); err == nil {
		listStmt = stmt
	} else {
		log.Fatalf("can't prepare list statement: %s", err)
	}

	if stmt, err := db.Prepare(existsSQL); err == nil {
		existsStmt = stmt
	} else {
		log.Fatalf("can't prepare exists statement: %s", err)
	}
}

// closeDatabase closes the db
func closeDatabase() {
	if err := db.Close(); err != nil {
		log.Printf("can't close database: %s", err)
	}
}

const schemaSQL = `-- pdfs
CREATE TABLE IF NOT EXISTS pdfs(
	id       INTEGER PRIMARY KEY,
	path     TEXT,
	pages    INT,
	sig      TEXT,
	text     TEXT,
	cover    BLOB,
	added_at TEXT
);

CREATE INDEX IF NOT EXISTS pdfs_sig ON pdfs(sig);

CREATE VIRTUAL TABLE IF NOT EXISTS pdfs_fts USING fts5(text, content=pdfs, content_rowid=id);

CREATE TRIGGER IF NOT EXISTS pdfs_ai AFTER INSERT ON pdfs BEGIN
	INSERT INTO pdfs_fts(text) VALUES (new.text);
END;

CREATE TRIGGER IF NOT EXISTS pdfs_ad AFTER DELETE ON pdfs BEGIN
	INSERT INTO pdfs_fts(pdfs_fts, rowid, text) VALUES('delete', old.id, old.text);
END;`

const (
	insertSQL = `INSERT INTO pdfs(path, pages, sig, text, cover, added_at) VALUES(?, ?, ?, ?, ?, ?)`

	coverSQL = `SELECT cover FROM pdfs WHERE id = ?`

	searchSQL = `SELECT pdfs.id, pdfs.path, pdfs.pages, snippet(pdfs_fts, 0, '{{{', '}}}', '...', 16) ` +
		`FROM pdfs_fts, pdfs WHERE pdfs_fts MATCH ? AND pdfs_fts.rowid = pdfs.id ORDER BY rank LIMIT ?`

	listSQL = `SELECT pdfs.id, pdfs.path, pdfs.pages FROM pdfs WHERE path LIKE ?`

	existsSQL = `SELECT EXISTS (SELECT sig FROM pdfs WHERE sig = ?)`
)

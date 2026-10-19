package store

import (
	"context"
	"database/sql"
	"fmt"
)

// schemaStatements creates the application schema (mirrors
// src/main/resources/db/migration/V1__create_tables.sql) idempotently.
var schemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS users (
  id varchar(255) primary key,
  username varchar(255) UNIQUE,
  password varchar(255),
  email varchar(255) UNIQUE,
  bio text,
  image varchar(511)
)`,
	`CREATE TABLE IF NOT EXISTS articles (
  id varchar(255) primary key,
  user_id varchar(255),
  slug varchar(255) UNIQUE,
  title varchar(255),
  description text,
  body text,
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
)`,
	`CREATE TABLE IF NOT EXISTS article_favorites (
  article_id varchar(255) not null,
  user_id varchar(255) not null,
  primary key(article_id, user_id)
)`,
	`CREATE TABLE IF NOT EXISTS follows (
  user_id varchar(255) not null,
  follow_id varchar(255) not null
)`,
	`CREATE TABLE IF NOT EXISTS tags (
  id varchar(255) primary key,
  name varchar(255) not null
)`,
	`CREATE TABLE IF NOT EXISTS article_tags (
  article_id varchar(255) not null,
  tag_id varchar(255) not null
)`,
	`CREATE TABLE IF NOT EXISTS comments (
  id varchar(255) primary key,
  body text,
  article_id varchar(255),
  user_id varchar(255),
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
)`,
}

// EnsureSchema creates every application table that does not exist yet.
func EnsureSchema(ctx context.Context, db *sql.DB) error {
	for _, stmt := range schemaStatements {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("store: ensure schema: %w", err)
		}
	}
	return nil
}

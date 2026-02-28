package db

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

func setupTestDB(t *testing.T) (*DB, func()) {
	t.Helper()

	// Use a test-specific database URL or default to one based on the dev URL
	url := os.Getenv("MAILEROO_TEST_DATABASE_URL")
	if url == "" {
		devURL := os.Getenv("DATABASE_URL")
		if devURL == "" {
			t.Fatal("DATABASE_URL or MAILEROO_TEST_DATABASE_URL must be set for DB tests")
		}
		// Expand environment variables if present (e.g. ${DB_USER})
		devURL = os.ExpandEnv(devURL)
		// Replace the DB name 'maileroo' with 'maileroo_test'
		url = strings.Replace(devURL, "/maileroo?", "/maileroo_test?", 1)
	}

	// 1. Create the test database if it doesn't exist
	// Connect to default 'postgres' db to run CREATE DATABASE
	adminURL := strings.Replace(url, "/maileroo_test?", "/postgres?", 1)
	adminDB, err := sqlx.Connect("postgres", adminURL)
	if err != nil {
		t.Fatalf("failed to connect to postgres for test db creation: %v", err)
	}
	_, _ = adminDB.Exec("DROP DATABASE IF EXISTS maileroo_test")
	_, err = adminDB.Exec("CREATE DATABASE maileroo_test")
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	adminDB.Close()

	// 2. Run migrations using dbmate
	cmd := exec.Command("dbmate", "-u", url, "--no-dump-schema", "up")
	cmd.Dir = "../../" 
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("dbmate migration failed: %v\nOutput: %s", err, string(out))
	}

	// 3. Connect to the test DB
	db, err := Connect(url)
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}

	cleanup := func() {
		db.Close()
	}

	return db, cleanup
}

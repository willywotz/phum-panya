package db

import (
	"os"
	"testing"

	"phum-panya/internal/model"
)

func TestDialectorSelectsDriver(t *testing.T) {
	sq, err := Dialector("sqlite", "x.db", "")
	if err != nil {
		t.Fatalf("sqlite: %v", err)
	}
	if sq.Name() != "sqlite" {
		t.Errorf("sqlite Name() = %q", sq.Name())
	}
	pg, err := Dialector("postgres", "", "postgres://u:p@h:5432/db")
	if err != nil {
		t.Fatalf("postgres: %v", err)
	}
	if pg.Name() != "postgres" {
		t.Errorf("postgres Name() = %q", pg.Name())
	}
	if _, err := Dialector("mysql", "", ""); err == nil {
		t.Errorf("unknown driver: want error, got nil")
	}
}

// TestOpenWithPostgres round-trips against a live Postgres. It is skipped
// unless APP_DATABASE_URL is set (CI provides a postgres service).
func TestOpenWithPostgres(t *testing.T) {
	dsn := os.Getenv("APP_DATABASE_URL")
	if dsn == "" {
		t.Skip("APP_DATABASE_URL not set")
	}
	g, err := OpenWith("postgres", "", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := model.AutoMigrate(g); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	d := model.District{Name: "ทดสอบ"}
	if err := g.Create(&d).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	var got model.District
	if err := g.First(&got, d.ID).Error; err != nil {
		t.Fatalf("read: %v", err)
	}
	if got.Name != "ทดสอบ" {
		t.Errorf("Name = %q", got.Name)
	}
}

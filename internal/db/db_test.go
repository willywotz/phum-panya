package db_test

import (
	"path/filepath"
	"testing"

	"gorm.io/gorm"

	"phum-panya/internal/db"
)

func TestOpenPingAndTx(t *testing.T) {
	g, err := db.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := g.DB()
	if err := sqlDB.Ping(); err != nil {
		t.Fatal(err)
	}
	if err := g.Exec(`CREATE TABLE t (n INTEGER)`).Error; err != nil {
		t.Fatal(err)
	}
	err = db.Tx(g, func(tx *gorm.DB) error { return tx.Exec(`INSERT INTO t(n) VALUES(1)`).Error })
	if err != nil {
		t.Fatalf("tx: %v", err)
	}
	var n int64
	g.Raw(`SELECT count(*) FROM t`).Scan(&n)
	if n != 1 {
		t.Fatalf("rows = %d, want 1", n)
	}
}

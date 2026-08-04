package revision_test

import (
	"path/filepath"
	"testing"

	"phum-panya/internal/clock"
	"phum-panya/internal/db"
	"phum-panya/internal/model"
	"phum-panya/internal/revision"
)

func TestAppendThenListInOrder(t *testing.T) {
	g, err := db.Open(filepath.Join(t.TempDir(), "r.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := model.AutoMigrate(g); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := revision.NewRepo(g, clock.Real{})

	type snap struct {
		Name string
	}
	if err := repo.Append("doctor", 7, 1, model.ActionCreate, snap{"before"}); err != nil {
		t.Fatalf("append 1: %v", err)
	}
	if err := repo.Append("doctor", 7, 1, model.ActionUpdate, snap{"after"}); err != nil {
		t.Fatalf("append 2: %v", err)
	}

	got, err := repo.List("doctor", 7)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Action != model.ActionCreate || got[1].Action != model.ActionUpdate {
		t.Fatalf("wrong order: %q, %q", got[0].Action, got[1].Action)
	}
	if got[1].AfterJSON != `{"Name":"after"}` {
		t.Fatalf("after_json = %q", got[1].AfterJSON)
	}
}

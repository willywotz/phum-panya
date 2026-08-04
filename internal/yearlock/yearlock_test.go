package yearlock_test

import (
	"errors"
	"path/filepath"
	"testing"

	"phum-panya/internal/clock"
	"phum-panya/internal/db"
	"phum-panya/internal/model"
	"phum-panya/internal/yearlock"
)

func TestLockRefusedWhenPendingExists(t *testing.T) {
	g, err := db.Open(filepath.Join(t.TempDir(), "yl.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := model.AutoMigrate(g); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// A pending recipe in year 2567 (DoctorID 1 need not exist: no doctor is created,
	// but recipe.DoctorID has an FK — so create the doctor's district+doctor, OR insert a
	// recipe whose FK is satisfied. Simplest: create a district+doctor first.)
	dist := model.District{Name: "d", Province: "p"}
	g.Create(&dist)
	doc := model.Doctor{Code: "D1", Photo: "-", FullName: "x", Specialty: "y", Status: "active", FirstYear: 2560, DistrictID: dist.ID, ReviewState: model.ReviewApproved}
	g.Create(&doc)
	g.Create(&model.Recipe{Code: "R1", Name: "x", DoctorID: doc.ID, Indication: "-", Preparation: "-", Usage: "-", DataYear: 2567, ReviewState: model.ReviewPending})

	repo := yearlock.NewRepo(g, clock.Real{})
	if err := repo.Lock(2567, 1); !errors.Is(err, yearlock.ErrPendingInYear) {
		t.Fatalf("want ErrPendingInYear, got %v", err)
	}
}

func TestLockThenIsLocked(t *testing.T) {
	g, err := db.Open(filepath.Join(t.TempDir(), "yl.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := model.AutoMigrate(g); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := yearlock.NewRepo(g, clock.Real{})
	if err := repo.Lock(2567, 1); err != nil {
		t.Fatalf("lock: %v", err)
	}
	locked, err := repo.IsLocked(2567)
	if err != nil {
		t.Fatalf("islocked: %v", err)
	}
	if !locked {
		t.Fatalf("2567 should be locked")
	}
	open, _ := repo.IsLocked(2568)
	if open {
		t.Fatalf("2568 should be open")
	}
}

func TestUnlockThenList(t *testing.T) {
	g, err := db.Open(filepath.Join(t.TempDir(), "yl.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := model.AutoMigrate(g); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := yearlock.NewRepo(g, clock.Real{})
	repo.Lock(2566, 1)
	repo.Lock(2567, 1)
	if err := repo.Unlock(2566); err != nil {
		t.Fatalf("unlock: %v", err)
	}
	list, err := repo.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].DataYear != 2567 {
		t.Fatalf("list = %+v, want only 2567", list)
	}
	locked, _ := repo.IsLocked(2566)
	if locked {
		t.Fatalf("2566 should be unlocked")
	}
}

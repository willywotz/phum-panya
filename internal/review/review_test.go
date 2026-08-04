package review_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"gorm.io/gorm"

	"phum-panya/internal/clock"
	"phum-panya/internal/db"
	"phum-panya/internal/model"
	"phum-panya/internal/review"
	"phum-panya/internal/revision"
)

func setup(t *testing.T) (*review.Repo, *gorm.DB, uint) {
	t.Helper()
	g, err := db.Open(filepath.Join(t.TempDir(), "rev.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := model.AutoMigrate(g); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	dist := model.District{Name: "เมือง", Province: "จ"}
	if err := g.Create(&dist).Error; err != nil {
		t.Fatalf("district: %v", err)
	}
	return review.NewRepo(g, revision.NewRepo(g, clock.Real{})), g, dist.ID
}

func seedDoctor(t *testing.T, g *gorm.DB, districtID uint, code, name, state string) model.Doctor {
	t.Helper()
	d := model.Doctor{Code: code, Photo: "-", FullName: name, Specialty: "y", Status: "active", FirstYear: 2568, DistrictID: districtID, ReviewState: state}
	if err := g.Create(&d).Error; err != nil {
		t.Fatalf("seed doctor: %v", err)
	}
	return d
}

func TestApproveCreatePromotesAndLogs(t *testing.T) {
	repo, g, did := setup(t)
	d := seedDoctor(t, g, did, "D1", "x", model.ReviewPending)

	if err := repo.Approve("doctor", d.ID, 99); err != nil {
		t.Fatalf("approve: %v", err)
	}
	var got model.Doctor
	g.First(&got, d.ID)
	if got.ReviewState != model.ReviewApproved {
		t.Fatalf("state = %q, want approved", got.ReviewState)
	}
	var revs int64
	g.Model(&model.Revision{}).Where("entity_type = ? AND entity_id = ? AND action = ?", "doctor", d.ID, model.ActionCreate).Count(&revs)
	if revs != 1 {
		t.Fatalf("revisions = %d, want 1", revs)
	}
}

func TestApproveEditAppliesOverlay(t *testing.T) {
	repo, g, did := setup(t)
	d := seedDoctor(t, g, did, "D1", "old", model.ReviewApproved)
	overlay := fmt.Sprintf(`{"ID":%d,"Code":"D1","Photo":"-","FullName":"new","Specialty":"y","Status":"active","FirstYear":2568,"DistrictID":%d,"ReviewState":"approved"}`, d.ID, did)
	if err := g.Model(&model.Doctor{}).Where("id = ?", d.ID).Update("pending_json", overlay).Error; err != nil {
		t.Fatalf("stash: %v", err)
	}

	if err := repo.Approve("doctor", d.ID, 99); err != nil {
		t.Fatalf("approve: %v", err)
	}
	var got model.Doctor
	g.First(&got, d.ID)
	if got.FullName != "new" {
		t.Fatalf("overlay not applied: FullName = %q", got.FullName)
	}
	if got.PendingJSON != nil {
		t.Fatalf("pending_json must be cleared after approve")
	}
}

func TestRejectCreateSetsReasonKeepsHidden(t *testing.T) {
	repo, g, did := setup(t)
	d := seedDoctor(t, g, did, "D1", "x", model.ReviewPending)

	if err := repo.Reject("doctor", d.ID, 99, "รูปไม่ชัด"); err != nil {
		t.Fatalf("reject: %v", err)
	}
	var got model.Doctor
	g.First(&got, d.ID)
	if got.ReviewState != model.ReviewRejected || got.RejectionReason == nil || *got.RejectionReason != "รูปไม่ชัด" {
		t.Fatalf("reject state/reason wrong: %q / %v", got.ReviewState, got.RejectionReason)
	}
}

func TestApproveRecipeEditReplacesIngredients(t *testing.T) {
	repo, g, did := setup(t)
	d := seedDoctor(t, g, did, "D1", "x", model.ReviewApproved)
	h := model.Herb{ThaiName: "ขิง"}
	g.Create(&h)
	rec := model.Recipe{Code: "R1", Name: "old", DoctorID: d.ID, Indication: "-", Preparation: "-", Usage: "-", DataYear: 2568, ReviewState: model.ReviewApproved}
	g.Create(&rec)
	g.Create(&model.Ingredient{RecipeID: rec.ID, HerbID: &h.ID, Amount: "1", Unit: "g"})

	payload := fmt.Sprintf(`{"recipe":{"ID":%d,"Code":"R1","Name":"new","DoctorID":%d,"Indication":"-","Preparation":"-","Usage":"-","DataYear":2568,"ReviewState":"approved"},"ingredients":[{"RecipeID":%d,"HerbID":%d,"Amount":"2","Unit":"g"}]}`, rec.ID, d.ID, rec.ID, h.ID)
	g.Model(&model.Recipe{}).Where("id = ?", rec.ID).Update("pending_json", payload)

	if err := repo.Approve("recipe", rec.ID, 99); err != nil {
		t.Fatalf("approve: %v", err)
	}
	var got model.Recipe
	g.First(&got, rec.ID)
	if got.Name != "new" {
		t.Fatalf("recipe name = %q, want new", got.Name)
	}
	var ings []model.Ingredient
	g.Where("recipe_id = ?", rec.ID).Find(&ings)
	if len(ings) != 1 || ings[0].Amount != "2" {
		t.Fatalf("ingredients = %+v, want one with amount 2", ings)
	}
}

func TestQueueListsAcrossEntities(t *testing.T) {
	repo, g, did := setup(t)
	seedDoctor(t, g, did, "D2", "pending-create", model.ReviewPending)
	ad := seedDoctor(t, g, did, "D3", "approved", model.ReviewApproved)
	g.Create(&model.Recipe{Code: "R9", Name: "r", DoctorID: ad.ID, Indication: "-", Preparation: "-", Usage: "-", DataYear: 2568, ReviewState: model.ReviewPending})

	items, err := repo.Queue(nil)
	if err != nil {
		t.Fatalf("queue: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("queue len = %d, want 2 (%+v)", len(items), items)
	}
	for _, it := range items {
		if it.DistrictID != did {
			t.Fatalf("item %+v district = %d, want %d", it, it.DistrictID, did)
		}
	}
}

func TestApproveDoctorTreeBulk(t *testing.T) {
	repo, g, did := setup(t)
	d := seedDoctor(t, g, did, "D5", "x", model.ReviewPending)
	rec := model.Recipe{Code: "R5", Name: "r", DoctorID: d.ID, Indication: "-", Preparation: "-", Usage: "-", DataYear: 2568, ReviewState: model.ReviewPending}
	g.Create(&rec)
	g.Create(&model.Case{RecipeID: rec.ID, Condition: "c", Result: "r", DataYear: 2568, ReviewState: model.ReviewPending})

	n, err := repo.ApproveDoctorTree(d.ID, 99)
	if err != nil {
		t.Fatalf("bulk: %v", err)
	}
	if n != 3 {
		t.Fatalf("approved count = %d, want 3", n)
	}
	var pending int64
	g.Model(&model.Doctor{}).Where("review_state = ?", model.ReviewPending).Count(&pending)
	if pending != 0 {
		t.Fatalf("doctor still pending")
	}
	g.Model(&model.Recipe{}).Where("review_state = ?", model.ReviewPending).Count(&pending)
	if pending != 0 {
		t.Fatalf("recipe still pending")
	}
	g.Model(&model.Case{}).Where("review_state = ?", model.ReviewPending).Count(&pending)
	if pending != 0 {
		t.Fatalf("case still pending")
	}
}

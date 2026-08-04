package doctor_test

import (
	"net/http"
	"strconv"
	"testing"

	"phum-panya/internal/model"
)

// seedDoctorWithRecipeAndCase creates a doctor in districtID with one recipe
// and one case under that recipe, returning the doctor and recipe IDs.
func (env *doctorAPI) seedDoctorWithRecipeAndCase(t *testing.T, code string, districtID uint) (doctorID, recipeID uint) {
	t.Helper()
	d := model.Doctor{
		Code: code, Photo: "-", FullName: "H", DistrictID: districtID,
		Specialty: "herbal", Status: "active", FirstYear: 2565,
		ConsentObtained: true,
	}
	if err := env.g.Create(&d).Error; err != nil {
		t.Fatalf("seed doctor: %v", err)
	}
	r := model.Recipe{
		Code: code + "-R1", Name: "Recipe", DoctorID: d.ID,
		Indication: "x", Preparation: "x", Usage: "x", DataYear: 2565,
	}
	if err := env.g.Create(&r).Error; err != nil {
		t.Fatalf("seed recipe: %v", err)
	}
	c := model.Case{
		RecipeID: r.ID, Condition: "x", Result: "x", DataYear: 2565,
	}
	if err := env.g.Create(&c).Error; err != nil {
		t.Fatalf("seed case: %v", err)
	}
	return d.ID, r.ID
}

func TestDeleteCascadesToRecipesAndCases(t *testing.T) {
	env := newDoctorAPI(t)
	doctorID, recipeID := env.seedDoctorWithRecipeAndCase(t, "MUE-E1", env.district1)

	path := "/api/doctors/" + strconv.FormatUint(uint64(doctorID), 10)
	res := env.doAsAdmin("DELETE", path, "")
	if res.Code != http.StatusNoContent {
		t.Fatalf("delete = %d, want 204, body = %s", res.Code, res.Body.String())
	}

	var recipeCount, caseCount int64
	env.g.Model(&model.Recipe{}).Where("doctor_id = ?", doctorID).Count(&recipeCount)
	env.g.Model(&model.Case{}).Where("recipe_id = ?", recipeID).Count(&caseCount)
	if recipeCount != 0 {
		t.Errorf("recipe count = %d, want 0", recipeCount)
	}
	if caseCount != 0 {
		t.Errorf("case count = %d, want 0", caseCount)
	}
}

// TestEditorDeleteQueuesWithoutCascade proves an editor delete is queued
// (pending_delete=true) rather than executed: no rows are removed, so
// recipes/cases under the doctor survive untouched.
func TestEditorDeleteQueuesWithoutCascade(t *testing.T) {
	env := newDoctorAPI(t)
	doctorID, recipeID := env.seedDoctorWithRecipeAndCase(t, "MUE-E4", env.district1)

	path := "/api/doctors/" + strconv.FormatUint(uint64(doctorID), 10)
	res := env.doAsEditor("DELETE", path, "")
	if res.Code != http.StatusNoContent {
		t.Fatalf("queued delete = %d, want 204, body = %s", res.Code, res.Body.String())
	}

	var d model.Doctor
	if err := env.g.First(&d, doctorID).Error; err != nil {
		t.Fatalf("reload doctor: %v", err)
	}
	if !d.PendingDelete {
		t.Error("PendingDelete should be true after editor delete")
	}

	var recipeCount, caseCount int64
	env.g.Model(&model.Recipe{}).Where("doctor_id = ?", doctorID).Count(&recipeCount)
	env.g.Model(&model.Case{}).Where("recipe_id = ?", recipeID).Count(&caseCount)
	if recipeCount != 1 {
		t.Errorf("recipe count = %d, want 1 (nothing cascaded)", recipeCount)
	}
	if caseCount != 1 {
		t.Errorf("case count = %d, want 1 (nothing cascaded)", caseCount)
	}
}

func TestUnpublishClearsConsentWithoutDeleting(t *testing.T) {
	env := newDoctorAPI(t)
	doctorID, recipeID := env.seedDoctorWithRecipeAndCase(t, "MUE-E2", env.district1)

	path := "/api/doctors/" + strconv.FormatUint(uint64(doctorID), 10) + "/unpublish"
	res := env.doAsEditor("POST", path, "")
	if res.Code != http.StatusNoContent {
		t.Fatalf("unpublish = %d, want 204, body = %s", res.Code, res.Body.String())
	}

	var d model.Doctor
	if err := env.g.First(&d, doctorID).Error; err != nil {
		t.Fatalf("reload doctor: %v", err)
	}
	if d.ConsentObtained {
		t.Error("consent_obtained should be false after unpublish")
	}

	var recipeCount, caseCount int64
	env.g.Model(&model.Recipe{}).Where("doctor_id = ?", doctorID).Count(&recipeCount)
	env.g.Model(&model.Case{}).Where("recipe_id = ?", recipeID).Count(&caseCount)
	if recipeCount != 1 {
		t.Errorf("recipe count = %d, want 1 (rows must survive unpublish)", recipeCount)
	}
	if caseCount != 1 {
		t.Errorf("case count = %d, want 1 (rows must survive unpublish)", caseCount)
	}
}

func TestUnpublishEnforcesOwnDistrict(t *testing.T) {
	env := newDoctorAPI(t)
	doctorID, _ := env.seedDoctorWithRecipeAndCase(t, "MUE-E3", env.district2)

	path := "/api/doctors/" + strconv.FormatUint(uint64(doctorID), 10) + "/unpublish"
	res := env.doAsEditor("POST", path, "")
	if res.Code != http.StatusForbidden {
		t.Fatalf("editor unpublish other district = %d, want 403", res.Code)
	}
}

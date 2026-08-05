package importer_test

import (
	"bytes"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"

	"phum-panya/internal/caserec"
	"phum-panya/internal/clock"
	"phum-panya/internal/db"
	"phum-panya/internal/doctor"
	"phum-panya/internal/herb"
	"phum-panya/internal/importer"
	"phum-panya/internal/model"
	"phum-panya/internal/recipe"
	"phum-panya/internal/revision"
	"phum-panya/internal/yearlock"
)

func newImporterEnv(t *testing.T) (*importer.Importer, *gorm.DB, uint) {
	t.Helper()
	g, err := db.Open(filepath.Join(t.TempDir(), "imp.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.AutoMigrate(g); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	dist := model.District{Name: "d", Province: "p"}
	if err := g.Create(&dist).Error; err != nil {
		t.Fatalf("district: %v", err)
	}
	rev := revision.NewRepo(g, clock.Real{})
	lock := yearlock.NewRepo(g, clock.Real{})
	im := importer.NewImporter(
		g, clock.Real{},
		doctor.NewRepo(g, clock.Real{}, rev),
		recipe.NewRepo(g, clock.Real{}, rev, lock),
		caserec.NewRepo(g, clock.Real{}, rev, lock),
		herb.NewRepo(g),
		lock,
	)
	return im, g, dist.ID
}

func buildDupLockedFixture(t *testing.T, districtID uint) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()
	did := strconv.Itoa(int(districtID))

	f.SetSheetName("Sheet1", importer.SheetDoctors)
	writeRow(t, f, importer.SheetDoctors, 1, importer.DoctorColumns)
	writeRow(t, f, importer.SheetDoctors, 2, []string{"D1", "dup", "", "", "0", did, "", "", "spec", "0", "", "active", "2560"})
	writeRow(t, f, importer.SheetDoctors, 3, []string{"D2", "new", "", "", "0", did, "", "", "spec", "0", "", "active", "2560"})

	if _, err := f.NewSheet(importer.SheetRecipes); err != nil {
		t.Fatalf("sheet: %v", err)
	}
	writeRow(t, f, importer.SheetRecipes, 1, importer.RecipeColumns)
	writeRow(t, f, importer.SheetRecipes, 2, []string{"R1", "r", "D2", "ind", "prep", "use", "", "", "2567"}) // locked year

	if _, err := f.NewSheet(importer.SheetIngredients); err != nil {
		t.Fatalf("sheet: %v", err)
	}
	writeRow(t, f, importer.SheetIngredients, 1, importer.IngredientColumns)

	if _, err := f.NewSheet(importer.SheetCases); err != nil {
		t.Fatalf("sheet: %v", err)
	}
	writeRow(t, f, importer.SheetCases, 1, importer.CaseColumns)

	buf, err := f.WriteToBuffer()
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	return buf.Bytes()
}

func TestCommitWritesApprovedThenUndo(t *testing.T) {
	im, g, distID := newImporterEnv(t)
	if distID != 1 {
		t.Fatalf("fixture assumes district id 1, got %d", distID) // newImporterEnv seeds the first district
	}
	data := buildFixtureWorkbook(t) // D1 doctor (district_id 1), R1 recipe (2565), ingredient ขิง, one case — all new, unlocked

	rep, err := im.Run(bytes.NewReader(data), "f.xlsx", 1)
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	if rep.BatchID == nil {
		t.Fatalf("committed run must return a batch id, report=%+v", rep)
	}
	if rep.DryRun {
		t.Fatalf("committed report must have DryRun=false")
	}

	var d model.Doctor
	if err := g.Where("code = ?", "D1").First(&d).Error; err != nil {
		t.Fatalf("imported doctor not found: %v", err)
	}
	if d.ReviewState != model.ReviewApproved {
		t.Fatalf("imported doctor must be approved, got %q", d.ReviewState)
	}
	if d.ConsentObtained {
		t.Fatalf("imported doctor must default to consent=false (hidden until consent recorded)")
	}
	if d.BatchID == nil || *d.BatchID != *rep.BatchID {
		t.Fatalf("imported doctor must be tagged with the batch")
	}
	var rc, cc int64
	g.Model(&model.Recipe{}).Where("code = ?", "R1").Count(&rc)
	g.Model(&model.Case{}).Count(&cc)
	if rc != 1 || cc != 1 {
		t.Fatalf("expected 1 recipe + 1 case written, got recipe=%d case=%d", rc, cc)
	}
	// the ingredient took the pending-herb path (ขิง not in catalog)
	var ing model.Ingredient
	if err := g.First(&ing).Error; err != nil {
		t.Fatalf("ingredient not written: %v", err)
	}
	if ing.PendingHerbName == nil || *ing.PendingHerbName != "ขิง" {
		t.Fatalf("ingredient should be pending-herb ขิง, got %+v", ing)
	}

	if err := im.Undo(*rep.BatchID); err != nil {
		t.Fatalf("undo: %v", err)
	}
	var n int64
	g.Model(&model.Doctor{}).Where("code = ?", "D1").Count(&n)
	if n != 0 {
		t.Fatalf("undo must remove the imported doctor")
	}
	g.Model(&model.Recipe{}).Where("code = ?", "R1").Count(&rc)
	g.Model(&model.Case{}).Count(&cc)
	if rc != 0 || cc != 0 {
		t.Fatalf("undo must remove imported recipe/case, got recipe=%d case=%d", rc, cc)
	}
}

// buildSecondBatchFixture: D1 is a duplicate (already imported), R2 is a NEW recipe under D1,
// plus one case under R2. Reuses the shared writeRow helper and the exported columns.
func buildSecondBatchFixture(t *testing.T, districtID uint) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()
	did := strconv.Itoa(int(districtID))

	f.SetSheetName("Sheet1", importer.SheetDoctors)
	writeRow(t, f, importer.SheetDoctors, 1, importer.DoctorColumns)
	writeRow(t, f, importer.SheetDoctors, 2, []string{"D1", "หมอ A", "", "", "0", did, "", "", "ยาต้ม", "0", "", "active", "2560"})

	if _, err := f.NewSheet(importer.SheetRecipes); err != nil {
		t.Fatalf("sheet: %v", err)
	}
	writeRow(t, f, importer.SheetRecipes, 1, importer.RecipeColumns)
	writeRow(t, f, importer.SheetRecipes, 2, []string{"R2", "ยาที่สอง", "D1", "ไอ", "ต้ม", "ดื่ม", "", "", "2565"})

	if _, err := f.NewSheet(importer.SheetIngredients); err != nil {
		t.Fatalf("sheet: %v", err)
	}
	writeRow(t, f, importer.SheetIngredients, 1, importer.IngredientColumns)

	if _, err := f.NewSheet(importer.SheetCases); err != nil {
		t.Fatalf("sheet: %v", err)
	}
	writeRow(t, f, importer.SheetCases, 1, importer.CaseColumns)
	writeRow(t, f, importer.SheetCases, 2, []string{"R2", "female", "30-40", "ไอ", "", "หาย", "", "2565"})

	buf, err := f.WriteToBuffer()
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	return buf.Bytes()
}

func TestUndoDoesNotDeleteOtherBatchData(t *testing.T) {
	im, g, distID := newImporterEnv(t)
	if distID != 1 {
		t.Fatalf("fixture assumes district id 1, got %d", distID)
	}

	// batch 1: D1 + R1 + case
	rep1, err := im.Run(bytes.NewReader(buildFixtureWorkbook(t)), "b1.xlsx", 1)
	if err != nil || rep1.BatchID == nil {
		t.Fatalf("batch1: %v rep=%+v", err, rep1)
	}
	// batch 2: D1 duplicate (skipped) + R2 + case, added under the existing D1
	rep2, err := im.Run(bytes.NewReader(buildSecondBatchFixture(t, distID)), "b2.xlsx", 1)
	if err != nil || rep2.BatchID == nil {
		t.Fatalf("batch2: %v rep=%+v", err, rep2)
	}

	// undo batch 1 must NOT touch batch 2's rows, and must keep D1 (batch 2 depends on it)
	if err := im.Undo(*rep1.BatchID); err != nil {
		t.Fatalf("undo batch1: %v", err)
	}
	var r1, r2, dCount int64
	g.Model(&model.Recipe{}).Where("code = ?", "R1").Count(&r1)
	g.Model(&model.Recipe{}).Where("code = ?", "R2").Count(&r2)
	g.Model(&model.Doctor{}).Where("code = ?", "D1").Count(&dCount)
	if r1 != 0 {
		t.Fatalf("batch1 recipe R1 should be gone, got %d", r1)
	}
	if r2 != 1 {
		t.Fatalf("batch2 recipe R2 must survive undo of batch1, got %d", r2)
	}
	if dCount != 1 {
		t.Fatalf("D1 must survive (batch2 still depends on it), got %d", dCount)
	}
	var status string
	g.Model(&model.ImportBatch{}).Where("id = ?", *rep2.BatchID).Pluck("status", &status)
	if status != "committed" {
		t.Fatalf("batch2 must stay committed, got %q", status)
	}
}

func TestDryRunReportsInFileDuplicate(t *testing.T) {
	im, _, distID := newImporterEnv(t)
	f := excelize.NewFile()
	defer f.Close()
	did := strconv.Itoa(int(distID))
	f.SetSheetName("Sheet1", importer.SheetDoctors)
	writeRow(t, f, importer.SheetDoctors, 1, importer.DoctorColumns)
	writeRow(t, f, importer.SheetDoctors, 2, []string{"D1", "a", "", "", "0", did, "", "", "spec", "0", "", "active", "2560"})
	writeRow(t, f, importer.SheetDoctors, 3, []string{"D1", "b", "", "", "0", did, "", "", "spec", "0", "", "active", "2560"})
	for _, s := range []string{importer.SheetRecipes, importer.SheetIngredients, importer.SheetCases} {
		if _, err := f.NewSheet(s); err != nil {
			t.Fatalf("sheet: %v", err)
		}
	}
	writeRow(t, f, importer.SheetRecipes, 1, importer.RecipeColumns)
	writeRow(t, f, importer.SheetIngredients, 1, importer.IngredientColumns)
	writeRow(t, f, importer.SheetCases, 1, importer.CaseColumns)
	buf, err := f.WriteToBuffer()
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	rep, err := im.DryRun(bytes.NewReader(buf.Bytes()), "dup.xlsx")
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if rep.DoctorsNew != 1 {
		t.Fatalf("two D1 rows must count as 1 new doctor, got %d", rep.DoctorsNew)
	}
	if len(rep.Skipped) == 0 {
		t.Fatalf("the in-file duplicate D1 must be reported as skipped")
	}
}

func TestDryRunReportsDuplicatesAndLockedYears(t *testing.T) {
	im, g, distID := newImporterEnv(t)
	g.Create(&model.Doctor{Code: "D1", Photo: "-", FullName: "existing", Specialty: "y", Status: "active", FirstYear: 2560, DistrictID: distID, ReviewState: model.ReviewApproved})
	g.Create(&model.YearLock{DataYear: 2567, LockedBy: 1})
	data := buildDupLockedFixture(t, distID)

	rep, err := im.DryRun(bytes.NewReader(data), "f.xlsx")
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if len(rep.Skipped) == 0 {
		t.Fatalf("expected D1 reported as skipped duplicate, report=%+v", rep)
	}
	if len(rep.Errors) == 0 {
		t.Fatalf("expected a locked-year error for the 2567 recipe, report=%+v", rep)
	}
	if !rep.DryRun {
		t.Fatalf("report.DryRun must be true")
	}
	var n int64
	g.Model(&model.ImportBatch{}).Count(&n)
	if n != 0 {
		t.Fatalf("dry run must not create a batch, got %d", n)
	}
}

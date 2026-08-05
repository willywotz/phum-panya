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
	if err := model.AutoMigrate(g); err != nil {
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

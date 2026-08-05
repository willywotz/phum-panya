package importer_test

import (
	"bytes"
	"io"
	"testing"

	"github.com/xuri/excelize/v2"

	"phum-panya/internal/importer"
)

func bytesReader(data []byte) io.Reader {
	return bytes.NewReader(data)
}

func writeRow(t *testing.T, f *excelize.File, sheet string, rowNum int, values []string) {
	t.Helper()
	for i, v := range values {
		cellName, err := excelize.CoordinatesToCellName(i+1, rowNum)
		if err != nil {
			t.Fatalf("cell name: %v", err)
		}
		if err := f.SetCellValue(sheet, cellName, v); err != nil {
			t.Fatalf("set cell: %v", err)
		}
	}
}

func buildFixtureWorkbook(t *testing.T) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()

	f.SetSheetName("Sheet1", importer.SheetDoctors)
	writeRow(t, f, importer.SheetDoctors, 1, importer.DoctorColumns)
	writeRow(t, f, importer.SheetDoctors, 2, []string{"D1", "หมอ A", "", "", "2500", "1", "", "", "ยาต้ม", "10", "", "active", "2560"})

	if _, err := f.NewSheet(importer.SheetRecipes); err != nil {
		t.Fatalf("new sheet: %v", err)
	}
	writeRow(t, f, importer.SheetRecipes, 1, importer.RecipeColumns)
	writeRow(t, f, importer.SheetRecipes, 2, []string{"R1", "ยาแก้ไข้", "D1", "ไข้", "ต้ม", "ดื่ม", "", "", "2565"})

	if _, err := f.NewSheet(importer.SheetIngredients); err != nil {
		t.Fatalf("new sheet: %v", err)
	}
	writeRow(t, f, importer.SheetIngredients, 1, importer.IngredientColumns)
	writeRow(t, f, importer.SheetIngredients, 2, []string{"R1", "ขิง", "10", "g", ""})

	if _, err := f.NewSheet(importer.SheetCases); err != nil {
		t.Fatalf("new sheet: %v", err)
	}
	writeRow(t, f, importer.SheetCases, 1, importer.CaseColumns)
	writeRow(t, f, importer.SheetCases, 2, []string{"R1", "female", "30-40", "ไข้", "", "หาย", "", "2565"})

	buf, err := f.WriteToBuffer()
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	return buf.Bytes()
}

func TestParseReadsAllSheets(t *testing.T) {
	data := buildFixtureWorkbook(t)
	p, err := importer.ParseWorkbook(bytesReader(data))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(p.Doctors) != 1 || p.Doctors[0].Code != "D1" || p.Doctors[0].FirstYear != 2560 {
		t.Fatalf("doctors = %+v", p.Doctors)
	}
	if len(p.Recipes) != 1 || p.Recipes[0].DoctorCode != "D1" || p.Recipes[0].DataYear != 2565 {
		t.Fatalf("recipes = %+v", p.Recipes)
	}
	if len(p.Ingredients) != 1 || p.Ingredients[0].RecipeCode != "R1" || p.Ingredients[0].HerbName != "ขิง" {
		t.Fatalf("ingredients = %+v", p.Ingredients)
	}
	if len(p.Cases) != 1 || p.Cases[0].RecipeCode != "R1" {
		t.Fatalf("cases = %+v", p.Cases)
	}
}

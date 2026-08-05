package importer

import (
	"io"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

// DoctorRow / RecipeRow / IngredientRow / CaseRow are raw rows from the template,
// before any linking or validation.
type DoctorRow struct {
	Code, FullName, KnownAs, Gender string
	BirthYear                       int
	DistrictID                      uint
	Address, Phone, Specialty       string
	YearsExperience                 int
	Lineage, Status                 string
	FirstYear                       int
}

type RecipeRow struct {
	Code, Name, DoctorCode, Indication, Preparation, Usage, Caution, CareStage string
	DataYear                                                                   int
}

type IngredientRow struct {
	RecipeCode, HerbName, Amount, Unit, Note string
}

type CaseRow struct {
	RecipeCode, PatientGender, PatientAgeRange, Condition, Treatment, Result, Duration string
	DataYear                                                                           int
}

// Parsed is the whole workbook as flat typed rows.
type Parsed struct {
	Doctors     []DoctorRow
	Recipes     []RecipeRow
	Ingredients []IngredientRow
	Cases       []CaseRow
}

// ParseWorkbook reads the canonical four-sheet template.
func ParseWorkbook(r io.Reader) (*Parsed, error) {
	f, err := excelize.OpenReader(r)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	out := &Parsed{}
	if err := eachDataRow(f, SheetDoctors, func(row []string) {
		out.Doctors = append(out.Doctors, DoctorRow{
			Code: cell(row, 0), FullName: cell(row, 1), KnownAs: cell(row, 2), Gender: cell(row, 3),
			BirthYear: atoi(cell(row, 4)), DistrictID: uint(atoi(cell(row, 5))),
			Address: cell(row, 6), Phone: cell(row, 7), Specialty: cell(row, 8),
			YearsExperience: atoi(cell(row, 9)), Lineage: cell(row, 10), Status: cell(row, 11),
			FirstYear: atoi(cell(row, 12)),
		})
	}); err != nil {
		return nil, err
	}
	if err := eachDataRow(f, SheetRecipes, func(row []string) {
		out.Recipes = append(out.Recipes, RecipeRow{
			Code: cell(row, 0), Name: cell(row, 1), DoctorCode: cell(row, 2),
			Indication: cell(row, 3), Preparation: cell(row, 4), Usage: cell(row, 5),
			Caution: cell(row, 6), CareStage: cell(row, 7), DataYear: atoi(cell(row, 8)),
		})
	}); err != nil {
		return nil, err
	}
	if err := eachDataRow(f, SheetIngredients, func(row []string) {
		out.Ingredients = append(out.Ingredients, IngredientRow{
			RecipeCode: cell(row, 0), HerbName: cell(row, 1), Amount: cell(row, 2),
			Unit: cell(row, 3), Note: cell(row, 4),
		})
	}); err != nil {
		return nil, err
	}
	if err := eachDataRow(f, SheetCases, func(row []string) {
		out.Cases = append(out.Cases, CaseRow{
			RecipeCode: cell(row, 0), PatientGender: cell(row, 1), PatientAgeRange: cell(row, 2),
			Condition: cell(row, 3), Treatment: cell(row, 4), Result: cell(row, 5),
			Duration: cell(row, 6), DataYear: atoi(cell(row, 7)),
		})
	}); err != nil {
		return nil, err
	}
	return out, nil
}

// eachDataRow calls fn for every non-header, non-blank row of a sheet.
func eachDataRow(f *excelize.File, sheet string, fn func(row []string)) error {
	rows, err := f.GetRows(sheet)
	if err != nil {
		return err
	}
	for i, row := range rows {
		if i == 0 || isBlank(row) {
			continue
		}
		fn(row)
	}
	return nil
}

func isBlank(row []string) bool {
	for _, c := range row {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}

func cell(row []string, i int) string {
	if i < len(row) {
		return strings.TrimSpace(row[i])
	}
	return ""
}

func atoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

// Package export writes staff bulk exports (CSV and Excel) of doctors,
// recipes, and cases. Every export selects explicit public columns, so
// phone, consent_*, and audit fields never leave this package.
package export

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"

	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

// row is anything that can render itself as one exported record, in the
// same column order as its header.
type row interface {
	values() []string
}

// doctorHeader is the explicit public column list for Doctor exports. It
// must never include phone, consent_*, or updated_*.
var doctorHeader = []string{
	"id", "code", "full_name", "known_as", "gender", "birth_year",
	"district_id", "address", "specialty", "years_experience", "lineage",
	"status", "first_year",
}

type doctorRow struct {
	ID              uint
	Code            string
	FullName        string
	KnownAs         string
	Gender          string
	BirthYear       int
	DistrictID      uint
	Address         string
	Specialty       string
	YearsExperience int
	Lineage         string
	Status          string
	FirstYear       int
}

func (d doctorRow) values() []string {
	return []string{
		strconv.FormatUint(uint64(d.ID), 10), d.Code, d.FullName, d.KnownAs, d.Gender,
		strconv.Itoa(d.BirthYear), strconv.FormatUint(uint64(d.DistrictID), 10), d.Address,
		d.Specialty, strconv.Itoa(d.YearsExperience), d.Lineage, d.Status, strconv.Itoa(d.FirstYear),
	}
}

// recipeSelect is the qualified column list used to query Recipe exports;
// recipeHeader is the matching output header.
var (
	recipeSelect = []string{
		"recipes.id", "recipes.code", "recipes.name", "recipes.doctor_id",
		"recipes.indication", "recipes.preparation", "recipes.usage",
		"recipes.caution", "recipes.care_stage", "recipes.data_year",
	}
	recipeHeader = []string{
		"id", "code", "name", "doctor_id", "indication", "preparation",
		"usage", "caution", "care_stage", "data_year",
	}
)

type recipeRow struct {
	ID          uint
	Code        string
	Name        string
	DoctorID    uint
	Indication  string
	Preparation string
	Usage       string
	Caution     string
	CareStage   string
	DataYear    int
}

func (r recipeRow) values() []string {
	return []string{
		strconv.FormatUint(uint64(r.ID), 10), r.Code, r.Name, strconv.FormatUint(uint64(r.DoctorID), 10),
		r.Indication, r.Preparation, r.Usage, r.Caution, r.CareStage, strconv.Itoa(r.DataYear),
	}
}

// caseSelect is the qualified column list used to query Case exports;
// caseHeader is the matching output header.
var (
	caseSelect = []string{
		"cases.id", "cases.recipe_id", "cases.patient_gender", "cases.patient_age_range",
		"cases.condition", "cases.treatment", "cases.result", "cases.duration", "cases.data_year",
	}
	caseHeader = []string{
		"id", "recipe_id", "patient_gender", "patient_age_range",
		"condition", "treatment", "result", "duration", "data_year",
	}
)

type caseRow struct {
	ID              uint
	RecipeID        uint
	PatientGender   string
	PatientAgeRange string
	Condition       string
	Treatment       string
	Result          string
	Duration        string
	DataYear        int
}

func (c caseRow) values() []string {
	return []string{
		strconv.FormatUint(uint64(c.ID), 10), strconv.FormatUint(uint64(c.RecipeID), 10),
		c.PatientGender, c.PatientAgeRange, c.Condition, c.Treatment, c.Result, c.Duration,
		strconv.Itoa(c.DataYear),
	}
}

// Doctors writes every doctor's public fields to w in format, scoped to
// districtID (nil exports every district).
func Doctors(w io.Writer, g *gorm.DB, format string, districtID *uint) error {
	q := g.Table("doctors").Select(doctorHeader)
	if districtID != nil {
		q = q.Where("district_id = ?", *districtID)
	}
	var out []doctorRow
	if err := q.Find(&out).Error; err != nil {
		return err
	}
	return writeRows(w, format, doctorHeader, out)
}

// Recipes writes every recipe's public fields to w in format, scoped to its
// doctor's districtID (nil exports every district).
func Recipes(w io.Writer, g *gorm.DB, format string, districtID *uint) error {
	q := g.Table("recipes").Select(recipeSelect)
	if districtID != nil {
		q = q.Joins("JOIN doctors ON doctors.id = recipes.doctor_id").
			Where("doctors.district_id = ?", *districtID)
	}
	var out []recipeRow
	if err := q.Find(&out).Error; err != nil {
		return err
	}
	return writeRows(w, format, recipeHeader, out)
}

// Cases writes every case's public fields to w in format, scoped to its
// recipe's doctor's districtID (nil exports every district).
func Cases(w io.Writer, g *gorm.DB, format string, districtID *uint) error {
	q := g.Table("cases").Select(caseSelect)
	if districtID != nil {
		q = q.Joins("JOIN recipes ON recipes.id = cases.recipe_id").
			Joins("JOIN doctors ON doctors.id = recipes.doctor_id").
			Where("doctors.district_id = ?", *districtID)
	}
	var out []caseRow
	if err := q.Find(&out).Error; err != nil {
		return err
	}
	return writeRows(w, format, caseHeader, out)
}

// writeRows renders header and items to w as format ("csv" or "xlsx").
func writeRows[T row](w io.Writer, format string, header []string, items []T) error {
	rows := make([][]string, len(items))
	for i, item := range items {
		rows[i] = item.values()
	}
	switch format {
	case "csv":
		return writeCSV(w, header, rows)
	case "xlsx":
		return writeXLSX(w, header, rows)
	default:
		return fmt.Errorf("export: unsupported format %q", format)
	}
}

func writeCSV(w io.Writer, header []string, rows [][]string) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(header); err != nil {
		return err
	}
	for _, r := range rows {
		if err := cw.Write(r); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

func writeXLSX(w io.Writer, header []string, rows [][]string) error {
	f := excelize.NewFile()
	defer f.Close()
	const sheet = "Sheet1"
	for i, h := range header {
		cell, err := excelize.CoordinatesToCellName(i+1, 1)
		if err != nil {
			return err
		}
		if err := f.SetCellValue(sheet, cell, h); err != nil {
			return err
		}
	}
	for r, row := range rows {
		for c, v := range row {
			cell, err := excelize.CoordinatesToCellName(c+1, r+2)
			if err != nil {
				return err
			}
			if err := f.SetCellValue(sheet, cell, v); err != nil {
				return err
			}
		}
	}
	return f.Write(w)
}

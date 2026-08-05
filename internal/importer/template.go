package importer

// The canonical import template: four sheets, columns in the fixed order below,
// linked by code. Keep these in sync with the published .xlsx template and the
// standard paper form (data spec Sheets A/B/C).
const (
	SheetDoctors     = "Doctors"
	SheetRecipes     = "Recipes"
	SheetIngredients = "Ingredients"
	SheetCases       = "Cases"
)

var (
	DoctorColumns = []string{
		"code", "full_name", "known_as", "gender", "birth_year", "district_id",
		"address", "phone", "specialty", "years_experience", "lineage", "status", "first_year",
	}
	RecipeColumns = []string{
		"code", "name", "doctor_code", "indication", "preparation", "usage",
		"caution", "care_stage", "data_year",
	}
	IngredientColumns = []string{"recipe_code", "herb_name", "amount", "unit", "note"}
	CaseColumns       = []string{
		"recipe_code", "patient_gender", "patient_age_range", "condition",
		"treatment", "result", "duration", "data_year",
	}
)

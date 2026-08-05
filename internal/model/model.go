// Package model defines the pure domain entities for phum-panya.
package model

import (
	"time"
)

const (
	RoleCentralAdmin   = "central_admin"
	RoleDistrictEditor = "district_editor"

	ReviewPending  = "pending"
	ReviewApproved = "approved"
	ReviewRejected = "rejected"

	ActionCreate = "create"
	ActionUpdate = "update"
	ActionDelete = "delete"
	ActionReject = "reject"

	GenderMale   = "male"
	GenderFemale = "female"
	GenderOther  = "other"

	DoctorStatusActive   = "active"
	DoctorStatusInactive = "inactive"
	DoctorStatusDeceased = "deceased"

	CaseResultCured    = "cured"
	CaseResultBetter   = "better"
	CaseResultNoChange = "no_change"

	ImportBatchStatusCommitted = "committed"
	ImportBatchStatusUndone    = "undone"
)

// Every review_state column (Doctor, Recipe, Case) carries the identical
// `check:chk_review_state,review_state IN ('approved','pending','rejected')`
// gorm tag below. A Go struct tag must be a literal string, so it cannot
// reference the ReviewApproved/Pending/Rejected constants above directly;
// keep the three literals in sync with those constants by hand.

// District is อำเภอ, the unit that groups all data.
type District struct {
	ID       uint   `gorm:"primaryKey"`
	Name     string `gorm:"not null"`
	Province string `gorm:"not null"`
}

// User is a staff login (central admin or district editor).
type User struct {
	ID           uint   `gorm:"primaryKey"`
	FullName     string `gorm:"not null"`
	Email        string `gorm:"uniqueIndex;not null"`
	PasswordHash string `gorm:"not null" json:"-"`
	Role         string `gorm:"not null;check:chk_user_role,role IN ('central_admin','district_editor')"`
	DistrictID   *uint
	District     District `gorm:"constraint:OnDelete:SET NULL"`
	Active       *bool    `gorm:"not null;default:true"`
}

// Session is a server-side login session keyed by a hashed token.
type Session struct {
	TokenHash string `gorm:"primaryKey"`
	UserID    uint
	ExpiresAt time.Time
}

// Doctor is หมอพื้นบ้าน, the healer profile.
type Doctor struct {
	ID         uint   `gorm:"primaryKey"`
	Code       string `gorm:"uniqueIndex;not null"`
	Photo      string `gorm:"not null"`
	FullName   string `gorm:"not null"`
	KnownAs    string
	Gender     string `gorm:"check:chk_doctor_gender,gender IN ('','male','female','other')"`
	BirthYear  int
	DistrictID uint `gorm:"not null;index"`
	// District has no OnDelete:CASCADE: District is a fixed list (spec §2) and
	// must never wipe its doctors when a row is removed.
	District        District
	Address         string
	Phone           string
	Specialty       string `gorm:"not null"`
	YearsExperience int
	Lineage         string
	ConsentObtained bool `gorm:"not null;default:false"`
	ConsentDate     *time.Time
	Status          string `gorm:"not null;check:chk_doctor_status,status IN ('active','inactive','deceased')"`
	FirstYear       int    `gorm:"not null"`
	UpdatedBy       *uint
	UpdatedAt       time.Time
	ReviewState     string `gorm:"not null;default:approved;index;check:chk_review_state,review_state IN ('approved','pending','rejected')"`
	PendingJSON     *string
	PendingPhoto    *string
	PendingDelete   bool `gorm:"not null;default:false"`
	RejectionReason *string
	BatchID         *uint `gorm:"index"`
}

// Herb is สมุนไพร, the shared catalog for the whole province.
type Herb struct {
	ID             uint   `gorm:"primaryKey"`
	ThaiName       string `gorm:"not null"`
	LocalName      string
	ScientificName string
	Photo          string
	PartUsed       string
	Properties     string

	CreatedByDistrictID *uint `gorm:"index"`
	AliasOfID           *uint `gorm:"index"` // set => this herb is an alias of AliasOfID (canonical)
}

// Recipe is ตำรับยา, a formula belonging to one doctor.
type Recipe struct {
	ID              uint   `gorm:"primaryKey"`
	Code            string `gorm:"uniqueIndex;not null"`
	Name            string `gorm:"not null"`
	DoctorID        uint   `gorm:"not null;index"`
	Doctor          Doctor `gorm:"constraint:OnDelete:CASCADE"`
	Indication      string `gorm:"not null"`
	Preparation     string `gorm:"not null"`
	Usage           string `gorm:"not null"`
	Caution         string
	CareStage       string
	Photos          []RecipePhoto `json:"photos" gorm:"foreignKey:RecipeID;constraint:OnDelete:CASCADE"`
	DataYear        int           `gorm:"not null"`
	UpdatedBy       *uint
	UpdatedAt       time.Time
	ReviewState     string `gorm:"not null;default:approved;index;check:chk_review_state,review_state IN ('approved','pending','rejected')"`
	PendingJSON     *string
	PendingDelete   bool `gorm:"not null;default:false"`
	RejectionReason *string
	BatchID         *uint `gorm:"index"`
}

// RecipePhoto is one image attached to a Recipe. A recipe may hold many
// (data model §4.5); this replaces the single Recipe.Photo string column
// (issue #18). Display order is SortOrder, lowest first. The ON DELETE
// CASCADE constraint lives on Recipe.Photos (the has-many side): GORM
// builds the recipe_photos foreign key from whichever side declares it, so
// this belongs-to field carries none, keeping one source of truth.
type RecipePhoto struct {
	ID        uint `gorm:"primaryKey"`
	RecipeID  uint `gorm:"not null;index"`
	Recipe    Recipe
	Path      string `gorm:"not null"`
	SortOrder int    `gorm:"not null;default:0"`
	CreatedAt time.Time
}

// Ingredient is one row inside a Recipe. Exactly one of HerbID or
// PendingHerbName is set (enforced by the chk_herb_xor check constraint).
type Ingredient struct {
	ID              uint   `gorm:"primaryKey;check:chk_herb_xor,(herb_id IS NOT NULL) <> (pending_herb_name IS NOT NULL)"`
	RecipeID        uint   `gorm:"not null;index"`
	Recipe          Recipe `gorm:"constraint:OnDelete:CASCADE"`
	HerbID          *uint  `gorm:"index"`
	Herb            Herb
	PendingHerbName *string
	Amount          string
	Unit            string
	Note            string
}

// Case is เคส, an anonymous treatment result linked to one recipe.
type Case struct {
	ID              uint   `gorm:"primaryKey"`
	RecipeID        uint   `gorm:"not null;index"`
	Recipe          Recipe `gorm:"constraint:OnDelete:CASCADE"`
	PatientGender   string `gorm:"check:chk_case_patient_gender,patient_gender IN ('','male','female','other')"`
	PatientAgeRange string
	Condition       string `gorm:"not null"`
	Treatment       string
	Result          string `gorm:"not null"`
	Duration        string
	Photo           string
	DataYear        int `gorm:"not null"`
	UpdatedBy       *uint
	UpdatedAt       time.Time
	ReviewState     string `gorm:"not null;default:approved;index;check:chk_review_state,review_state IN ('approved','pending','rejected')"`
	PendingJSON     *string
	PendingPhoto    *string
	PendingDelete   bool `gorm:"not null;default:false"`
	RejectionReason *string
	BatchID         *uint `gorm:"index"`
}

// Revision is an append-only audit log entry for a create/update/delete/
// reject action on an entity, recording who made the change and its
// resulting state as JSON.
type Revision struct {
	ID         uint      `gorm:"primaryKey"`
	EntityType string    `gorm:"not null;index:idx_rev_entity"`
	EntityID   uint      `gorm:"not null;index:idx_rev_entity"`
	ChangedBy  uint      `gorm:"not null"`
	ChangedAt  time.Time `gorm:"not null"`
	Action     string    `gorm:"not null"`
	AfterJSON  string    `gorm:"not null"`
}

// YearLock marks a data year as locked, blocking further edits to that
// year's records once a district editor's data has been finalized.
type YearLock struct {
	DataYear int       `gorm:"primaryKey"`
	LockedAt time.Time `gorm:"not null"`
	LockedBy uint      `gorm:"not null"`
}

// ImportBatch records one bulk-import run, letting an admin undo it.
type ImportBatch struct {
	ID         uint      `gorm:"primaryKey"`
	ImportedBy uint      `gorm:"not null"`
	ImportedAt time.Time `gorm:"not null"`
	SourceFile string    `gorm:"not null"`
	RowCount   int
	Status     string `gorm:"not null;check:chk_import_batch_status,status IN ('committed','undone')"`
}

// LoginAttempt is one recorded failed login. The DB-backed rate limiter
// (auth.DBLimiter) reads these rows so throttle state is shared across api
// replicas. On Postgres the table is UNLOGGED (see internal/db.AutoMigrate).
type LoginAttempt struct {
	ID        uint      `gorm:"primaryKey"`
	Key       string    `gorm:"index;not null"`
	CreatedAt time.Time `gorm:"index;not null"`
}

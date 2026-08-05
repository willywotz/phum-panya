// Package model defines the GORM entities for phum-panya and the
// AutoMigrate function that creates their tables.
package model

import (
	"time"

	"gorm.io/gorm"
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
)

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
	Role         string `gorm:"not null"`
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
	Gender     string
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
	Status          string `gorm:"not null"`
	FirstYear       int    `gorm:"not null"`
	UpdatedBy       *uint
	UpdatedAt       time.Time
	ReviewState     string `gorm:"not null;default:approved;index"`
	PendingJSON     *string
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
	Photo           string
	DataYear        int `gorm:"not null"`
	UpdatedBy       *uint
	UpdatedAt       time.Time
	ReviewState     string `gorm:"not null;default:approved;index"`
	PendingJSON     *string
	PendingDelete   bool `gorm:"not null;default:false"`
	RejectionReason *string
	BatchID         *uint `gorm:"index"`
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
	PatientGender   string
	PatientAgeRange string
	Condition       string `gorm:"not null"`
	Treatment       string
	Result          string `gorm:"not null"`
	Duration        string
	Photo           string
	DataYear        int `gorm:"not null"`
	UpdatedBy       *uint
	UpdatedAt       time.Time
	ReviewState     string `gorm:"not null;default:approved;index"`
	PendingJSON     *string
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
	Status     string `gorm:"not null"` // committed | undone
}

// AutoMigrate creates or updates the tables for every model.
func AutoMigrate(g *gorm.DB) error {
	return g.AutoMigrate(
		&District{}, &User{}, &Session{}, &Doctor{},
		&Herb{}, &Recipe{}, &Ingredient{}, &Case{},
		&Revision{}, &YearLock{}, &ImportBatch{},
	)
}

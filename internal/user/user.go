// Package user provides CRUD access to staff login accounts.
package user

import (
	"gorm.io/gorm"

	"phum-panya/internal/model"
)

// Repo provides CRUD access to users backed by GORM.
type Repo struct {
	g *gorm.DB
}

// NewRepo returns a Repo backed by g.
func NewRepo(g *gorm.DB) *Repo {
	return &Repo{g: g}
}

// List returns every user.
func (r *Repo) List() ([]model.User, error) {
	var users []model.User
	err := r.g.Find(&users).Error
	return users, err
}

// Get returns the user with id, or gorm.ErrRecordNotFound if none exists.
func (r *Repo) Get(id uint) (model.User, error) {
	var u model.User
	err := r.g.First(&u, id).Error
	return u, err
}

// Create inserts u and sets its ID. The caller must set PasswordHash.
func (r *Repo) Create(u *model.User) error {
	return r.g.Create(u).Error
}

// Update saves the profile fields of u (full name, email, role, district).
// It does not change the password. It returns gorm.ErrRecordNotFound if no
// user with u.ID exists. Existence is checked first because Save, given a
// primary key with no matching row, inserts rather than reports zero rows
// affected.
func (r *Repo) Update(u *model.User) error {
	existing, err := r.Get(u.ID)
	if err != nil {
		return err
	}
	existing.FullName = u.FullName
	existing.Email = u.Email
	existing.Role = u.Role
	existing.DistrictID = u.DistrictID
	return r.g.Save(&existing).Error
}

// SetActive sets the active flag for the user with id. A column update is
// used because Active is a *bool: a struct Save would treat a false value
// the same as an unset zero value and drop it. It returns
// gorm.ErrRecordNotFound if no user with id exists.
func (r *Repo) SetActive(id uint, active bool) error {
	res := r.g.Model(&model.User{}).Where("id = ?", id).Update("active", active)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// SetPassword sets the password hash for the user with id. It returns
// gorm.ErrRecordNotFound if no user with id exists.
func (r *Repo) SetPassword(id uint, hash string) error {
	res := r.g.Model(&model.User{}).Where("id = ?", id).Update("password_hash", hash)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

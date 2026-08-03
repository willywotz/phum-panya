// Package bootstrap seeds the first central admin user.
package bootstrap

import (
	"gorm.io/gorm"

	"phum-panya/internal/auth"
	"phum-panya/internal/model"
)

const roleCentralAdmin = "central_admin"

// EnsureAdmin creates the first central_admin user if none exists.
// It is idempotent: it is a no-op if a central_admin already exists,
// or if email or password is empty.
func EnsureAdmin(g *gorm.DB, email, password string) (created bool, err error) {
	var n int64
	if err := g.Model(&model.User{}).Where("role = ?", roleCentralAdmin).Count(&n).Error; err != nil {
		return false, err
	}
	if n > 0 || email == "" || password == "" {
		return false, nil
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		return false, err
	}
	fullName := email
	if fullName == "" {
		fullName = "Administrator"
	}
	active := true
	user := model.User{
		FullName:     fullName,
		Email:        email,
		PasswordHash: hash,
		Role:         roleCentralAdmin,
		Active:       &active,
	}
	if err := g.Create(&user).Error; err != nil {
		return false, err
	}
	return true, nil
}

package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"phum-panya/internal/httpx"
	"phum-panya/internal/model"
)

// userContextKey is the gin context key holding the *CurrentUser.
const userContextKey = "user"

// CurrentUser is the authenticated user attached to a gin request.
type CurrentUser struct {
	ID         uint
	FullName   string
	Role       string
	DistrictID *uint
}

// LoadUser reads the session cookie and, if it names an active user,
// attaches a *CurrentUser to the gin context. It never aborts: missing
// or invalid sessions simply leave the context without a user.
func LoadUser(store *SessionStore, g *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw, err := c.Cookie(sessionCookieName)
		if err != nil || raw == "" {
			c.Next()
			return
		}
		userID, err := store.Lookup(raw)
		if err != nil {
			c.Next()
			return
		}
		var user model.User
		if err := g.First(&user, userID).Error; err != nil {
			c.Next()
			return
		}
		if user.Active == nil || !*user.Active {
			c.Next()
			return
		}
		c.Set(userContextKey, &CurrentUser{
			ID:         user.ID,
			FullName:   user.FullName,
			Role:       user.Role,
			DistrictID: user.DistrictID,
		})
		c.Next()
	}
}

// UserFrom returns the *CurrentUser attached to c, if any.
func UserFrom(c *gin.Context) (*CurrentUser, bool) {
	v, ok := c.Get(userContextKey)
	if !ok {
		return nil, false
	}
	user, ok := v.(*CurrentUser)
	return user, ok
}

// RequireAuth aborts with 401 unless a user is attached to the context.
func RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, ok := UserFrom(c); !ok {
			httpx.Err(c, http.StatusUnauthorized, "unauthorized", "authentication required")
			c.Abort()
			return
		}
		c.Next()
	}
}

// RequireRole aborts with 401 if no user is authenticated, or 403 if the
// authenticated user's role does not match role.
func RequireRole(role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := UserFrom(c)
		if !ok {
			httpx.Err(c, http.StatusUnauthorized, "unauthorized", "authentication required")
			c.Abort()
			return
		}
		if user.Role != role {
			httpx.Err(c, http.StatusForbidden, "forbidden", "insufficient role")
			c.Abort()
			return
		}
		c.Next()
	}
}

// CanWriteDistrict reports whether u may write data for districtID.
// A central admin may write any district; a district editor may write
// only their own; any other role may write none.
func CanWriteDistrict(u *CurrentUser, districtID uint) bool {
	switch u.Role {
	case "central_admin":
		return true
	case "district_editor":
		return u.DistrictID != nil && *u.DistrictID == districtID
	default:
		return false
	}
}

package httpx

import "github.com/gin-gonic/gin"

// OK writes v as JSON with the given status.
func OK(c *gin.Context, status int, v any) { c.JSON(status, v) }

// Err writes a JSON error body {"error":{"code","message"}}.
func Err(c *gin.Context, status int, code, msg string) {
	c.JSON(status, gin.H{"error": gin.H{"code": code, "message": msg}})
}

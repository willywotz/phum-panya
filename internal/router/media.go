package router

import (
	"errors"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"phum-panya/internal/media"
)

// mediaHandler streams a stored image at /media/<key> from the media store.
// Keys are content hashes, so responses are immutable and cacheable forever.
func mediaHandler(store media.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := strings.TrimPrefix(c.Param("key"), "/")
		if key == "" || strings.Contains(key, "..") || path.Clean(key) != key {
			c.Status(http.StatusNotFound)
			return
		}
		obj, err := store.Open(key)
		if err != nil {
			if errors.Is(err, media.ErrNotFound) {
				c.Status(http.StatusNotFound)
				return
			}
			c.Status(http.StatusInternalServerError)
			return
		}
		defer obj.Body.Close()
		h := c.Writer.Header()
		h.Set("Content-Type", obj.ContentType)
		if obj.Size > 0 {
			h.Set("Content-Length", strconv.FormatInt(obj.Size, 10))
		}
		h.Set("Cache-Control", "public, max-age=31536000, immutable")
		c.Status(http.StatusOK)
		_, _ = io.Copy(c.Writer, obj.Body)
	}
}

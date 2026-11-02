package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

func (a *API) mountStatic(r *gin.Engine) {
	dir := a.Cfg.WebDir
	if dir == "" {
		dir = "../web/dist"
	}
	index := filepath.Join(dir, "index.html")
	assets := filepath.Join(dir, "assets")
	// Register the assets route even when the bundle is not built yet so a
	// missing file never falls through to the SPA index.html handler.
	r.Static("/assets", assets)
	for _, f := range []string{"favicon.ico", "favicon.svg", "logo.svg"} {
		r.StaticFile("/"+f, filepath.Join(dir, f))
	}

	r.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		if strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/t/") || strings.HasPrefix(path, "/.well-known/") {
			c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "not_found", "message": "Route not found"}})
			return
		}
		if _, err := os.Stat(index); err != nil {
			c.JSON(http.StatusNotFound, gin.H{
				"error": gin.H{
					"code":    "frontend_not_built",
					"message": "Frontend bundle not found. Run `pnpm build` in web/ or use the Vite dev server.",
				},
			})
			return
		}
		c.File(index)
	})
}

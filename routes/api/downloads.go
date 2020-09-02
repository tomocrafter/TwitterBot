package api

import (
	"TwitterBot/controllers/api/downloads"

	"github.com/gin-gonic/gin"
)

// SetDownloadsRoutes sets routes of downloads api to passed gin router.
func SetDownloadsRoutes(router *gin.RouterGroup) {
	router.GET("/:user", downloads.GetOne)
	router.GET("/suggestions", downloads.GetSuggestions)
}

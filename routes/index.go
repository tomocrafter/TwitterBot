package routes

import (
	"TwitterBot/controllers"

	"github.com/gin-gonic/gin"
)

// SetIndexRoute sets a router to index(/) page.
func SetIndexRoute(g *gin.RouterGroup) {
	g.GET("/", controllers.Index)
}

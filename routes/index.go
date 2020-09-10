package routes

import (
	"github.com/tomocrafter/TwitterBot/controllers"

	"github.com/gin-gonic/gin"
)

// SetIndexRoute sets a router to index(/) page.
func SetIndexRoute(g *gin.RouterGroup) {
	g.GET("/", controllers.Index)
}

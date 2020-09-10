package routes

import (
	"github.com/tomocrafter/TwitterBot/config"
	"github.com/tomocrafter/TwitterBot/middlewares/cors"
	"github.com/tomocrafter/TwitterBot/routes/api"
	"github.com/tomocrafter/TwitterBot/routes/webhook"

	"github.com/gin-gonic/gin"
)

// InitRoutes initializes gin routes.
func InitRoutes(g *gin.Engine, c *config.Config) {
	g.Use(gin.Logger())
	g.Use(cors.Middleware())

	index := g.Group("/")

	webhook.SetTwitterWebhookRoutes(index, c)

	apiGroup := g.Group("/api")

	downloadsGroup := apiGroup.Group("/downloads")
	api.SetDownloadsRoutes(downloadsGroup)
}

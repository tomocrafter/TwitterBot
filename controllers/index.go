package controllers

import "github.com/gin-gonic/gin"

// Index handles requests of index page that just returnes loggined user name.
func Index(context *gin.Context) {
	context.String(200, user.ScreenName+" is online!")
}

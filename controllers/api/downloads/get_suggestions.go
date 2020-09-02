package downloads

import (
	"fmt"
	"net/http"

	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"
)

// GetSuggestions handles request that retrive usernames inferred from the username being type.
func GetSuggestions(context *gin.Context) {
	query, ok := context.GetQuery("query")
	if !ok {
		context.JSON(http.StatusBadRequest, []interface{}{})
		return
	}

	var screenNames []string
	_, err := dbMap.Select(&screenNames, "SELECT DISTINCT screen_name FROM download WHERE screen_name LIKE ? LIMIT 10", "%"+escape(query)+"%")
	if err != nil {
		context.JSON(http.StatusInternalServerError, []string{})
		fmt.Printf("Error on requesting to MySQL: %+v", err)
		sentry.CaptureException(err)
		return
	}

	context.JSON(http.StatusOK, screenNames)
}

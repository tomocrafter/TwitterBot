package downloads

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"
)

// GetOne handles request that retrive download data of specified user id.
func GetOne(context *gin.Context) {
	user, ok := context.Params.Get("user")
	if !ok {
		context.JSON(http.StatusBadRequest, []interface{}{})
		return
	}

	var downloads []Download
	_, err := dbMap.Select(&downloads, "SELECT video_url,video_thumbnail,tweet_id FROM download WHERE screen_name = ?", user)
	if err != nil {
		context.JSON(http.StatusInternalServerError, []interface{}{})
		fmt.Printf("error occurred while requesting to MySQL: %+v", err)
		sentry.CaptureException(err)
		return
	}

	n := len(downloads)
	var res = make([]DownloadResponse, n)
	for i := 0; i < n; i++ {
		d := downloads[i]
		res[i] = DownloadResponse{ // TODO
			ScreenName:     d.ScreenName,
			VideoURL:       d.VideoURL,
			VideoThumbnail: d.VideoThumbnail,
			TweetID:        strconv.FormatInt(d.TweetID, 10),
		}
	}
	context.JSON(http.StatusOK, res)
}

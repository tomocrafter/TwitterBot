package debug

import (
	"bytes"
	"io/ioutil"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm/logger"
)

// Middleware initializes new twitter debug handler.
func Middleware() gin.HandlerFunc {
	// Make a debug handler to prints body of webhook.
	return func(context *gin.Context) {
		reader := &context.Request.Body
		buf, _ := ioutil.ReadAll(*reader)
		s := string(buf)
		*reader = ioutil.NopCloser(bytes.NewBuffer(buf))
		logger.Silent("webhook debug", zap.String("body", s))
	}
}

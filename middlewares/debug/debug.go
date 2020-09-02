package debug

import (
	"bytes"
	"io/ioutil"
	"log"

	"github.com/gin-gonic/gin"
)

// Middleware initializes new twitter debug handler.
func Middleware() gin.HandlerFunc {
	// Make a debug handler to prints body of webhook.
	return func(context *gin.Context) {
		if context
		reader := &context.Request.Body
		buf, _ := ioutil.ReadAll(*reader)
		s := string(buf)
		*reader = ioutil.NopCloser(bytes.NewBuffer(buf))
		log.Printf("webhook debug:\n%s\n", s)
	}
}

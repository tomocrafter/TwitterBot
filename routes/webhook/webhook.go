package webhook

import (
	"TwitterBot/config"
	"log"

	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"
	"github.com/tomocrafter/go-twitter/twitter"
)

// SetTwitterWebhookRoutes sets routers of twitter webhook.
// TODO:
//  Move handler create to separated controller.
//  Think how handle config, because only this router uses config. It shouldn't be.
func SetTwitterWebhookRoutes(router *gin.RouterGroup, c *config.Config) {
	router.GET(c.Path.Webhook, twitter.CreateCRCHandler(c.Twitter.ConsumerSecret))

	// Routing to POST /webhook for handling webhook payload!
	payloads := make(chan interface{})

	// Start listening payloads from webhook
	go listen(payloads)

	handler, err := twitter.CreateWebhookHandler(payloads)
	if err != nil {
		sentry.CaptureException(err)
		log.Fatal("Error while creating webhook handler", err)
	}
	router.POST(c.Path.Webhook, twitter.CreateTwitterAuthHandler(c.Twitter.ConsumerSecret), handler)

}

package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"github.com/dghubble/go-twitter/twitter"
	"github.com/gin-gonic/gin"
	"log"
	"net/http"
	"regexp"
	"strings"
	"unicode"
)

var r = regexp.MustCompile(`<a href=".*?" rel="nofollow">(.*?)</a>`)

type WebhookRequest struct {
	TweetCreateEvents   []twitter.Tweet              `json:"tweet_create_events"`
	DirectMessageEvents []twitter.DirectMessageEvent `json:"direct_message_events"`
	Users               map[string]FixedUser         `json:"users"`
}

func IsBlank(str string) bool {
	for _, char := range str {
		if !unicode.IsSpace(char) {
			return false
		}
	}
	return true
}

func createCRCToken(crcToken string) string {
	mac := hmac.New(sha256.New, []byte(botConfig.Twitter.ConsumerSecret))
	mac.Write([]byte(crcToken))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func HandleCRC(context *gin.Context) {
	if token, ok := context.GetQuery("crc_token"); ok {
		context.JSON(http.StatusOK, gin.H{
			"response_token": "sha256=" + createCRCToken(token),
		})
	} else {
		context.JSON(http.StatusBadRequest, gin.H{
			"error": "Missing crc_token in the request.",
		})
	}
}

func AuthTwitter(context *gin.Context) {
	signature := context.Request.Header.Get("X-Twitter-Webhooks-Signature")

	// 違ったらゼロを返す。
	if subtle.ConstantTimeCompare([]byte(signature), []byte("sha256="+createCRCToken(readRequestBody(&context.Request.Body)))) != 1 {
		context.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "The webhook signature is not correct."})
	}
}

func HandleTwitter(context *gin.Context) {
	req := new(WebhookRequest)
	if err := context.BindJSON(req); err != nil {
		context.JSON(http.StatusBadRequest, err)
		fmt.Printf("Error on parsing json from twitter: %+v\n", err)
		return
	}

	switch {
	case req.TweetCreateEvents != nil: // If Timeline
		n := len(req.TweetCreateEvents)
		for i := 0; i < n; i++ {
			tweet := &req.TweetCreateEvents[i]

			var builder strings.Builder
			var isReply bool

			// TODO: もっと効率いい方法があるはず。 @tomobotter @tomocrafter test | {0, 11}, {12, 23} | i >= e.Start() && i <= e.End() to @tomobotter @tomocrafter
			var isDeleting bool
			end := -1
			for i, r := range tweet.Text {
				for _, e := range tweet.Entities.UserMentions {
					if id == e.ID {
						isReply = true
					}
					if i == e.Indices.Start() {
						isDeleting = true
						end = e.Indices.End()
						break
					}
				}
				if isDeleting && i <= end {
					continue
				}
				isDeleting = false
				builder.WriteRune(r)
			}

			log.Println("TL @" + tweet.User.ScreenName + ": " + tweet.Text)

			// if not retweet or not reply or black listed.
			via := r.FindStringSubmatch(tweet.Source)
			if tweet.RetweetedStatus != nil && !isReply && len(via) != 0 && isBlackListed(via[1]) {
				return
			}

			body := builder.String()

			Dispatch(TimelineSender{
				Client: client,
				Tweet:  tweet,
			}, body)
		}
	case req.DirectMessageEvents != nil: // If DirectMessage
		n := len(req.DirectMessageEvents)
		for i := 0; i < n; i++ {
			e := &req.DirectMessageEvents[i]

			if string(id) == e.Message.SenderID {
				return
			}

			text := e.Message.Data.Text
			if e.Message.Data.QuickReplyResponse != nil {
				text = e.Message.Data.QuickReplyResponse.Metadata
			}

			user := req.Users[e.Message.SenderID]

			Dispatch(DirectMessageSender{
				Client:             client,
				User:               &user,
				DirectMessageEvent: e,
			}, text)
		}
	}
}

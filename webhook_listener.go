package main

import (
	"log"
	"strings"

	"github.com/tomocrafter/go-twitter/twitter"
)

func listen(payloads chan interface{}) {
	for event := range payloads {
		switch t := event.(type) {
		case twitter.TweetCreateEvent:
			var builder strings.Builder
			var isReply bool

			// TODO: もっと効率いい方法があるはず。 @tomobotter @tomocrafter test | {0, 11}, {12, 23} | i >= e.Start() && i <= e.End() to @tomobotter @tomocrafter
			var isTruncating bool
			end := -1
			for i, r := range t.Text {
				for _, e := range t.Entities.UserMentions {
					if id == e.ID {
						isReply = true
					}
					if i == e.Indices.Start() {
						isTruncating = true
						end = e.Indices.End()
						break
					}
				}
				if isTruncating && i <= end {
					continue
				}
				isTruncating = false
				builder.WriteRune(r)
			}

			// if not retweet or not replyId or black listed.
			via := r.FindStringSubmatch(t.Source)
			if t.RetweetedStatus != nil || !isReply || (len(via) != 0 && isDeniedClient(via[1])) {
				return
			}

			log.Println("TL @" + t.User.ScreenName + ": " + t.Text)

			body := builder.String()

			tweet := event.(twitter.Tweet)
			go Dispatch(TimelineSender{
				Tweet: &tweet,
			}, body)
		case twitter.DMEvent:
			if string(id) == t.Message.SenderID {
				return
			}

			text := t.Message.Data.Text
			if t.Message.Data.QuickReplyResponse != nil {
				text = t.Message.Data.QuickReplyResponse.Metadata
			}

			user := t.Users[t.Message.SenderID]

			go Dispatch(DirectMessageSender{
				User:               &user,
				DirectMessageEvent: &t.DirectMessageEvent,
			}, text)
		}
	}
}

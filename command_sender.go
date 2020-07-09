package main

import (
	"fmt"
	"strconv"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/go-redis/redis"
	"github.com/tomocrafter/go-twitter/twitter"
)

var (
	sendMessageQueue = make(chan Message)
)

type Message struct {
	m       string
	replyID *int64
}

// メッセージは、キューイングする必要があります。
// 1リクエストごとにRedisとコミュニケーションしツイートが制限されているか確かめる必要があるからです。
func MessageSendTicker() {
	changeName := func(name string, client *twitter.Client) {
		_, _, _ = client.Accounts.UpdateProfile(&twitter.AccountProfileParams{
			Name:            name,
			IncludeEntities: twitter.Bool(false),
			SkipStatus:      twitter.Bool(true),
		})
	}

	for message := range sendMessageQueue {
		canReply, isUnlocked := checkCanReply()
		if !canReply {
			return
		}

		params := &twitter.StatusUpdateParams{}
		if message.replyID != nil {
			params.TrimUser = twitter.Bool(true)
			params.InReplyToStatusID = *message.replyID
		}

		_, _, e := client.Statuses.Update(message.m, params)

		if isUnlocked && e == nil {
			changeName("tomobotter", client)
		} else if e != nil {
			if e.(twitter.APIError).Errors[0].Code == 185 { // User is over daily status update limit
				fmt.Println("\x1b[31m        Rate Limit Reached!        \x1b[0m")
				changeName("tomobotter@ツイート制限中", client)
				redisClient.Set(NoReply, time.Now().Add(10*time.Minute).Unix(), 0)
			}
		}
	}
}

/*
checkCanReply はツイートができるか、そしてこのリクエストが制限が解除されてから一度目かをRedisに問い合わせて返します。

必要:
送信するときに、API制限中かどうか。
API制限が終わってから一回目かどうか。

つまり:
no-reply-id:制限が解除される時間 をRedisに保存しておく。
キーが無かった場合は制限されていないのでAPIは制限されていない。
キーが合った場合は、その値をint64でパースする。
今の時間より解除される時間のが多かった=まだ解除されていない。
今の時間より解除される時間のが少なかった=解除された。さらに、キーがあるため解除されてから一回目のAPIコール。
*/
func checkCanReply() (canTweet, isUnlocked bool) {
	nextResetStr, err := redisClient.Get(NoReply).Result()
	if err == redis.Nil {
		nextResetStr = "0"
		redisClient.Set(NoReply, "0", 0)
		return true, false
	} else if err != nil {
		sentry.CaptureException(err)
		return true, false // If error occurred on Redis, Try to replyId.
	}

	if nextReset, err := strconv.ParseInt(nextResetStr, 10, 64); err == nil {
		if nextReset > time.Now().Unix() {
			return false, false
		} else if nextReset == 0 {
			return true, false
		} else {
			redisClient.Set(NoReply, "0", 0)
			return true, true
		}
	} else { // If redis returned no-replyId as not int.
		redisClient.Set(NoReply, "0", 0)
		return true, false
	}
}

func BroadcastMessage(message string) {
	sendMessageQueue <- Message{m: message}
}

type CommandSender interface {
	// SendMessage はコマンドの送信主に対し返信します。
	// このメソッドは現在のルーチンをブロックします。
	SendMessage(message string)

	// GetName はコマンドの送信主の名前を返します。
	GetName() string
}

type TwitterSender interface {
	GetUserId() int64
	GetScreenName() string
}

// Timeline Sender
type TimelineSender struct {
	Tweet      *twitter.Tweet
	ReplyCache *twitter.Tweet
}

func (s TimelineSender) GetUserId() int64 {
	return s.Tweet.User.ID
}

func (s TimelineSender) GetScreenName() string {
	return s.Tweet.User.ScreenName
}

func (s TimelineSender) SendMessage(message string) {
	sendMessageQueue <- Message{m: "@" + s.Tweet.User.ScreenName + " " + message, replyID: &s.Tweet.ID}
}

func (s TimelineSender) GetName() string {
	return s.Tweet.User.Name
}

// DirectMessage Sender
type DirectMessageSender struct {
	User               *twitter.User
	DirectMessageEvent *twitter.DirectMessageEvent
}

func (s DirectMessageSender) GetUserId() int64 {
	return s.User.ID
}

func (s DirectMessageSender) GetScreenName() string {
	return s.User.ScreenName
}

func (s DirectMessageSender) SendMessage(message string) {
	_, _, err := client.DirectMessages.EventsNew(&twitter.DirectMessageEventsNewParams{
		Event: &twitter.DirectMessageEvent{
			Type: "message_create",
			Message: &twitter.DirectMessageEventMessage{
				Target: &twitter.DirectMessageTarget{
					RecipientID: s.User.ScreenName,
				},
				Data: &twitter.DirectMessageData{
					Text: message,
				},
			},
		},
	})
	if err != nil {
		if err.(twitter.APIError).Errors[0].Code == 349 { // You cannot send messages to this user
			return
		}
		sentry.CaptureException(err)
	}
}

func (s DirectMessageSender) GetName() string {
	return s.User.Name
}

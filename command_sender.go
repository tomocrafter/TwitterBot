package main

import (
	"encoding/json"
	"fmt"
	"github.com/dghubble/go-twitter/twitter"
	"github.com/go-redis/redis"
	"strconv"
	"time"
)

var (
	sendMessageQueue []*Message
	updatedQueue     = make(chan bool)
)

type Message struct {
	m     string
	reply int64
}

type FixedUser struct {
	ContributorsEnabled            bool                  `json:"contributors_enabled"`
	CreatedAt                      string                `json:"created_at"`
	DefaultProfile                 bool                  `json:"default_profile"`
	DefaultProfileImage            bool                  `json:"default_profile_image"`
	Description                    string                `json:"description"`
	Email                          string                `json:"email"`
	Entities                       *twitter.UserEntities `json:"entities"`
	FavouritesCount                int                   `json:"favourites_count"`
	FollowRequestSent              bool                  `json:"follow_request_sent"`
	Following                      bool                  `json:"following"`
	FollowersCount                 int                   `json:"followers_count"`
	FriendsCount                   int                   `json:"friends_count"`
	GeoEnabled                     bool                  `json:"geo_enabled"`
	ID                             json.Number           `json:"id"`
	IDStr                          string                `json:"id_str"`
	IsTranslator                   bool                  `json:"is_translator"`
	Lang                           string                `json:"lang"`
	ListedCount                    int                   `json:"listed_count"`
	Location                       string                `json:"location"`
	Name                           string                `json:"name"`
	Notifications                  bool                  `json:"notifications"`
	ProfileBackgroundColor         string                `json:"profile_background_color"`
	ProfileBackgroundImageURL      string                `json:"profile_background_image_url"`
	ProfileBackgroundImageURLHttps string                `json:"profile_background_image_url_https"`
	ProfileBackgroundTile          bool                  `json:"profile_background_tile"`
	ProfileBannerURL               string                `json:"profile_banner_url"`
	ProfileImageURL                string                `json:"profile_image_url"`
	ProfileImageURLHttps           string                `json:"profile_image_url_https"`
	ProfileLinkColor               string                `json:"profile_link_color"`
	ProfileSidebarBorderColor      string                `json:"profile_sidebar_border_color"`
	ProfileSidebarFillColor        string                `json:"profile_sidebar_fill_color"`
	ProfileTextColor               string                `json:"profile_text_color"`
	ProfileUseBackgroundImage      bool                  `json:"profile_use_background_image"`
	Protected                      bool                  `json:"protected"`
	ScreenName                     string                `json:"screen_name"`
	ShowAllInlineMedia             bool                  `json:"show_all_inline_media"`
	Status                         *twitter.Tweet        `json:"status"`
	StatusesCount                  int                   `json:"statuses_count"`
	Timezone                       string                `json:"time_zone"`
	URL                            string                `json:"url"`
	UtcOffset                      int                   `json:"utc_offset"`
	Verified                       bool                  `json:"verified"`
	WithheldInCountries            []string              `json:"withheld_in_countries"`
	WithholdScope                  string                `json:"withheld_scope"`
}

func init() {
	go func() {
		for range updatedQueue {
			for len(sendMessageQueue) != 0 {
				message := sendMessageQueue[0]
				sendMessageQueue = sendMessageQueue[1:]
				sendMessage(message)
			}
		}
	}()
}

type CommandSender interface {
	SendMessage(message string)
	GetName() string
}

type TwitterSender interface {
	GetUserId() int64
	GetScreenName() string
}

// Timeline Sender
type TimelineSender struct {
	Tweet      *twitter.Tweet
	CacheReply *twitter.Tweet
}

func (s TimelineSender) GetUserId() int64 {
	return s.Tweet.User.ID
}

func (s TimelineSender) GetScreenName() string {
	return s.Tweet.User.ScreenName
}

/*
必要:
送信するときに、API制限中かどうか。
API制限が終わってから一回目かどうか。

つまり:
no-reply:制限が解除される時間 をRedisに保存しておく。
キーが無かった場合は制限されていないのでAPIは制限されていない。
キーが合った場合は、その値をint64でパースする。
今の時間より解除される時間のが多かった=まだ解除されていない。
今の時間より解除される時間のが少なかった=解除された。さらに、キーがあるため解除されてから一回目のAPIコール。

ツイートできるか, このリクエストで解除されたか
*/
func checkCanReply() (canTweet, isUnlocked bool) {
	nextResetStr, err := redisClient.Get(NoReply).Result()
	if err == redis.Nil {
		nextResetStr = "0"
		redisClient.Set(NoReply, "0", 0)
	} else if err != nil {
		HandleError(err)
		return true, false // If error occurred on Redis, Try to reply.
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
	} else { // If redis returned no-reply as not int.
		redisClient.Set(NoReply, "0", 0)
		return true, false
	}
}

func changeName(name string, client *twitter.Client) {
	_, _, _ = client.Accounts.UpdateProfile(&twitter.AccountProfileParams{
		Name:            name,
		IncludeEntities: twitter.Bool(false),
		SkipStatus:      twitter.Bool(true),
	})
}

func BroadcastMessage(message string) {
	sendMessageQueue = append(sendMessageQueue, &Message{m: message, reply: 0})
	updatedQueue <- true
}

func (s TimelineSender) SendMessage(message string) {
	sendMessageQueue = append(sendMessageQueue, &Message{m: "@" + s.Tweet.User.ScreenName + " " + message, reply: s.Tweet.ID})
	updatedQueue <- true
}

func sendMessage(message *Message) {
	canReply, isUnlocked := checkCanReply()
	if !canReply {
		return
	}

	var e error
	if message.reply != 0 {
		_, _, e = client.Statuses.Update(message.m, &twitter.StatusUpdateParams{
			TrimUser:          twitter.Bool(true),
			InReplyToStatusID: message.reply,
		})
	} else {
		_, _, e = client.Statuses.Update(message.m, nil)
	}
	if isUnlocked && e == nil {
		changeName("tomobotter", client)
	} else if e != nil && !e.(twitter.APIError).Empty() {
		if e.(twitter.APIError).Errors[0].Code == 185 { // User is over daily status update limit
			fmt.Println("\x1b[31m        Rate Limit Reached!        \x1b[0m")
			changeName("tomobotter@ツイート制限中", client)
			redisClient.Set("no-reply", time.Now().Add(10*time.Minute).Unix(), 0)
		}
	}
}

func (s TimelineSender) GetName() string {
	return s.Tweet.User.Name
}

// DirectMessage Sender
type DirectMessageSender struct {
	User               *FixedUser
	DirectMessageEvent *twitter.DirectMessageEvent
}

func (s DirectMessageSender) GetUserId() int64 {
	z, _ := s.User.ID.Int64()
	return z
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
					RecipientID: s.User.ID.String(),
				},
				Data: &twitter.DirectMessageData{
					Text: message,
				},
			},
		},
	})
	HandleError(err)
}

func (s DirectMessageSender) GetName() string {
	return s.User.Name
}

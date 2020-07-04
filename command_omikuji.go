package main

import (
	"github.com/getsentry/sentry-go"
	"github.com/go-redis/redis"
	"github.com/kyokomi/lottery"
	"github.com/tomocrafter/go-twitter/twitter"
	"log"
	"math/rand"
	"strconv"
	"sync"
	"time"
)

type Item struct {
	ItemName string
	DropProb int
}

func (d Item) Prob() int {
	return d.DropProb
}

var (
	mt    = new(sync.Mutex)
	lot   = lottery.New(rand.New(rand.NewSource(time.Now().Unix())))
	items = []lottery.Interface{
		Item{ItemName: "大吉", DropProb: 5},    // 0.05%
		Item{ItemName: "吉", DropProb: 4500},  // 45%
		Item{ItemName: "中吉", DropProb: 3600}, // 36%
		Item{ItemName: "小吉", DropProb: 1595}, // 15.95%
		Item{ItemName: "凶", DropProb: 300},   // 3%
	}
)

func OmikujiCommand(s CommandSender, _ []string) {
	mt.Lock()
	defer mt.Unlock()

	var screenName string
	var id int64

	switch s := s.(type) {
	case TimelineSender:
		screenName = s.Tweet.User.ScreenName
		id = s.Tweet.User.ID
	case DirectMessageSender:
		screenName = s.User.ScreenName
		id = s.User.ID
	default:
		return
	}

	// Check If today is in 1/1 - 1/7
	now := time.Now()
	if now.Month() != 1 || now.Day() > 7 {
		return
	}

	key := "lottery:" + strconv.FormatInt(id, 10)
	err := redisClient.Get(key).Err()
	if err == redis.Nil {
		index := lot.Lots(items...)
		if index == -1 {
			log.Fatalln("lot error")
		}

		if index == 0 {
			go func() {
				_, _, err := client.DirectMessages.EventsNew(&twitter.DirectMessageEventsNewParams{
					Event: &twitter.DirectMessageEvent{
						Type: "message_create",
						Message: &twitter.DirectMessageEventMessage{
							Target: &twitter.DirectMessageTarget{
								RecipientID: strconv.FormatInt(id, 10),
							},
							Data: &twitter.DirectMessageData{
								Text: "結果は 大吉 でした！おめでとうございます！\nあなたにとって素敵な一年になりますように。\nぜひまた明日も挑戦してください！\nなお、おみくじ機能は1/7まで利用可能です。",
							},
						},
					},
				})
				if err != nil {
					sentry.CaptureException(err)
				}
			}()
			BroadcastMessage(s.GetName() + " (@" + screenName + ") さんが確率 0.05% の大吉を当てました！")
		} else {
			go func() {
				_, _, err := client.DirectMessages.EventsNew(&twitter.DirectMessageEventsNewParams{
					Event: &twitter.DirectMessageEvent{
						Type: "message_create",
						Message: &twitter.DirectMessageEventMessage{
							Target: &twitter.DirectMessageTarget{
								RecipientID: strconv.FormatInt(id, 10),
							},
							Data: &twitter.DirectMessageData{
								Text: "結果は " + items[index].(Item).ItemName + " でした！ぜひまた明日も挑戦してください！\nなお、おみくじ機能は1/7まで利用可能です。",
							},
						},
					},
				})
				if err != nil {
					sentry.CaptureException(err)
				}
			}()
		}

		tomorrow := now.AddDate(0, 0, 1)
		ch := time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), 0, 0, 0, 0, location)

		redisClient.SetNX(key, "", ch.Sub(now))
	} else if err != nil {
		sentry.CaptureException(err)
	}
}

package main

import (
	"fmt"
	mapset "github.com/deckarep/golang-set"
	"github.com/dghubble/go-twitter/twitter"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	pattern = regexp.MustCompile("^https?:/*twitter\\.com/+(.+/+(status|statuses)|i/+web/+status|statuses)/+([2-9][0-9]|[1-9][0-9]{2,})(\\.json|\\.xml|\\.html|/)?(\\?.*)?$")
)

const (
	baseUnixTime         = 1288834974657
	InvalidArgumentError = "https://twitter. com/tomocrafter/status/829221788500553734 か 829221788500553734 のようなURLを指定してください。"
)

func statusIdToMillis(id int64) int64 {
	return (id >> 22) + baseUnixTime
}

func twitterIdToTime(id int64) time.Time {
	return time.Unix(0, statusIdToMillis(id)*int64(time.Millisecond)).In(location)
}

func roundDown(num, places float64) float64 {
	shift := math.Pow(10, places)
	return math.Trunc(num*shift) / shift
}

func formatTime(t time.Time) string {
	return t.Format("15:04:05.000")
}

func parseTweetURL(url string) (int64, bool) {
	match := pattern.FindStringSubmatch(url)
	if len(match) > 3 {
		id, _ := strconv.ParseInt(match[3], 10, 64)
		return id, true
	}
	return 0, false
}

func interfaceToInt64(i []interface{}) []int64 {
	f := make([]int64, len(i))
	for n := range i {
		f[n] = i[n].(int64)
	}
	return f
}

func TimeCommand(s CommandSender, args []string) (err error) {
	switch s := s.(type) {
	case TimelineSender:
		if s.Tweet.InReplyToStatusID == 0 { // Error
			s.SendMessage("時間を計測したいツイートにリプライしてください。")
			return
		}

		now := time.Now()

		t := twitterIdToTime(s.Tweet.InReplyToStatusID)
		just := time.Date(t.Year(), t.Month(), t.Day(), 3, 34, 0, 0, location)

		if t.Equal(just) {
			s.SendMessage("時間: 03:34:00.000 (ジャスト！おめでとうございます。)")
			return
		}

		if now.Hour() == 3 && now.Minute() >= 30 && now.Minute() <= 40 { // 3:30 ~ 3:40
			sTweet, _, e := client.Statuses.Show(s.Tweet.InReplyToStatusID, nil)
			HandleError(e)

			if sTweet.Text != "334" {
				return
			}
		}

		if t.Hour() == 3 && t.Minute() >= 32 && t.Minute() <= 36 { // 3:32 ~ 3:36
			diff := float64(t.Sub(just)) / float64(time.Second)

			var sb strings.Builder
			sb.WriteString("時間:")
			sb.WriteString(formatTime(t))
			sb.WriteString(" (3:34ちょうどの時間から ")
			// 本来であればfmtの%+.3fは0.0009の場合0.001に四捨五入されてしまうため切り捨てしているが、
			// 実際にはTwitterのタイムスタンプには.000までしかないため切り捨てしなくても問題ない。作者の性格に依存している。
			sb.WriteString(fmt.Sprintf("%+.3f", roundDown(diff, 3)))
			sb.WriteString("秒)")

			s.SendMessage(sb.String())
		} else {
			s.SendMessage("時間: " + formatTime(t))
		}

	case DirectMessageSender:
		ids := mapset.NewThreadUnsafeSet()
		urls := make(map[string]string, len(s.DirectMessageEvent.Message.Data.Entities.Urls)) // Shorten URL -> Expanded URL
		for _, url := range s.DirectMessageEvent.Message.Data.Entities.Urls {
			urls[url.URL] = url.ExpandedURL
		}
		for _, v := range args {
			if id, err := strconv.ParseInt(v, 10, 64); err == nil {
				ids.Add(id)
				continue
			}

			if url, ok := urls[v]; ok {
				if id, ok := parseTweetURL(url); ok {
					ids.Add(id)
				} else {
					s.SendMessage(InvalidArgumentError)
				}
			} else {
				s.SendMessage(InvalidArgumentError)
			}
		}

		if ids.Cardinality() == 0 {
			return
		}

		var sb strings.Builder
		sb.WriteString("2010年11月以前のツイートでは正常に動作しません。\n\n")

		tweets, _, err := client.Statuses.Lookup(interfaceToInt64(ids.ToSlice()), &twitter.StatusLookupParams{
			IncludeEntities: twitter.Bool(false),
		})
		if err != nil {
			return err
		}
		for _, tweet := range tweets {
			ids.Remove(tweet.ID)

			sb.WriteByte('@')
			sb.WriteString(tweet.User.ScreenName)
			sb.WriteByte(':')
			sb.WriteByte('\n')
			sb.WriteString(tweet.Text)
			sb.WriteByte('\n')
			sb.WriteString(formatTime(twitterIdToTime(tweet.ID)))
			sb.WriteString("\n\n")
		}

		ids.Each(func(i interface{}) bool {
			if i != 0 {
				sb.WriteString("\n\n")
			}
			sb.WriteString(strconv.FormatInt(id, 10))
			sb.WriteString(" は存在しないか非公開のアカウントのツイートです。")
			return false
		})

		s.SendMessage(sb.String())
	}
	return
}

func handleQuickTime(s DirectMessageSender) bool {
	if len(s.DirectMessageEvent.Message.Data.Entities.Urls) == 1 &&
		s.DirectMessageEvent.Message.Data.Text == s.DirectMessageEvent.Message.Data.Entities.Urls[0].URL {

		url := s.DirectMessageEvent.Message.Data.Entities.Urls[0].ExpandedURL

		if id, ok := parseTweetURL(url); ok {
			tweet, resp, err := client.Statuses.Show(id, &twitter.StatusShowParams{
				IncludeMyRetweet: twitter.Bool(false),
				IncludeEntities:  twitter.Bool(false),
			})
			if err != nil {
				if resp != nil {
					if resp.StatusCode == 404 {
						s.SendMessage(strconv.FormatInt(id, 10) + " は存在しないツイートです。")
					} else if resp.StatusCode == 403 {
						s.SendMessage("このツイートは非公開アカウントのツイートか、取得できないツイートです。")
					}
					return true
				} else {
					HandleError(err)
				}
			} else {
				var sb strings.Builder
				sb.WriteByte('@')
				sb.WriteString(tweet.User.ScreenName)
				sb.WriteString(":\n")
				sb.WriteString(tweet.Text)
				sb.WriteString("\n")
				sb.WriteString(formatTime(twitterIdToTime(tweet.ID)))

				s.SendMessage(sb.String())
			}
			return true
		} else {
			return false
		}
	}
	return false
}

package main

import (
	"strconv"

	"github.com/getsentry/sentry-go"
	"github.com/go-sql-driver/mysql"
	"github.com/tomocrafter/go-twitter/twitter"
)

func downloadCommand(s CommandSender, args []string) {
	switch s := s.(type) {
	case TimelineSender:
		if IsTimeRestricting() {
			return
		}

		replyID := s.Tweet.InReplyToStatusID
		if replyID == 0 {
			s.SendMessage("動画やgifのツイートにリプライしてください。")
			return
		}

		var tweet twitter.Tweet

		if s.ReplyCache != nil {
			tweet = *s.ReplyCache
		} else {
			tweet = queueProcessor.LookupTweetBlocking(replyID)
		}

		variant, err := GetVideoVariant(&tweet)
		if err != nil {
			s.SendMessage(err.Error())
			return
		}

		e := dbMap.Insert(&Download{
			ScreenName:     s.Tweet.User.ScreenName,
			VideoURL:       variant.URL,
			VideoThumbnail: tweet.ExtendedEntities.Media[0].MediaURLHttps,
			TweetID:        replyID,
		})
		if e != nil {
			if mysqlErr, ok := e.(*mysql.MySQLError); ok {
				// https://dev.mysql.com/doc/refman/8.0/en/server-error-reference.html
				if mysqlErr.Number == 1062 { // ER_DUP_ENTRY
					s.SendMessage("この動画/gifはすでに保存済みです。下記URLからダウンロードしてください。\nhttps://bot.tomocraft.net/downloads/" + s.Tweet.User.ScreenName)
					return
				}
			}
			s.SendMessage("@tomocrafter データベース上にてエラーが発生しました。開発者ができる限り早くサポート致します。")
			sentry.CaptureException(e)
		} else {
			s.SendMessage("ダウンロードの準備が整いました。下記URLからダウンロードしてください。\nhttps://bot.tomocraft.net/downloads/" + s.Tweet.User.ScreenName)
		}

	case DirectMessageSender:
		if len(args) == 0 {
			s.SendMessage("削除したい動画/gifのIDを指定してください。")
			return
		}

		if id, err := strconv.ParseInt(args[0], 10, 64); err != nil {
			s.SendMessage("TwitterのツイートIDを指定してください。通常、長い数字になるはずです。")
		} else {
			count, err := dbMap.Delete(&Download{TweetID: id, ScreenName: s.User.ScreenName})
			if err != nil {
				s.SendMessage("データベース上にてエラーが発生しました。開発者ができる限り早くサポート致します。")
				sentry.CaptureException(err)
			} else if count > 0 {
				s.SendMessage("削除が完了しました！")
			}
		}
	}
}

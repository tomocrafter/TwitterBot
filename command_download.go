package main

import (
	"github.com/dghubble/go-twitter/twitter"
	"github.com/getsentry/sentry-go"
	"github.com/go-sql-driver/mysql"
	"strconv"
)

func DownloadCommand(s CommandSender, args []string) error {
	switch s := s.(type) {
	case TimelineSender:
		replyId := s.Tweet.InReplyToStatusID
		if replyId == 0 {
			s.SendMessage("動画やgifのツイートにリプライしてください。")
			return nil
		}

		if IsTweetRestricting() {
			return nil
		}

		proc := func(tweet twitter.Tweet) {
			variant, err := GetVideoVariant(&tweet)
			if err != nil {
				s.SendMessage(err.Error())
				return
			}

			e := dbMap.Insert(&Download{
				ScreenName:     s.Tweet.User.ScreenName,
				VideoURL:       variant.URL,
				VideoThumbnail: tweet.ExtendedEntities.Media[0].MediaURLHttps,
				TweetID:        replyId,
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
		}

		if s.CacheReply != nil {
			proc(*s.CacheReply)
		} else {
			RegisterLookupHandler(replyId, func(tweet twitter.Tweet) {
				proc(tweet)
			})
		}

	case DirectMessageSender:
		if len(args) == 0 {
			s.SendMessage("削除したい動画/gifのIDを指定してください。")
			return nil
		}

		if id, err := strconv.ParseInt(args[0], 10, 64); err != nil {
			s.SendMessage("TwitterのツイートIDを指定してください。通常、長い数字になるはずです。")
		} else {
			_, e := dbMap.Exec("DELETE FROM download WHERE screen_name = ? AND tweet_id = ?", s.User.ScreenName, id)
			if e != nil {
				s.SendMessage("データベース上にてエラーが発生しました。開発者ができる限り早くサポート致します。")
				return e
			} else {
				s.SendMessage("削除が完了しました！")
			}
		}
	}
	return nil
}
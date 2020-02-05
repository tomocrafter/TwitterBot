package main

import (
	"github.com/dghubble/go-twitter/twitter"
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

		status, resp, err := client.Statuses.Show(replyId, &twitter.StatusShowParams{
			TrimUser:         twitter.Bool(true),
			IncludeMyRetweet: twitter.Bool(false),
			TweetMode:        "extended",
		})
		if err != nil { //Twitterサーバーからvalidationされてるはずなのでこれが発生することはないと思う。
			if resp != nil && resp.StatusCode == 404 {
				s.SendMessage("リプライされたツイートが見つかりませんでした。")
			}
			return nil
		}
		if status.ExtendedEntities == nil {
			s.SendMessage("動画やgifのツイートにリプライしてください。")
			return nil
		}
		if len(status.ExtendedEntities.Media) != 1 {
			s.SendMessage("動画やgifのツイートにリプライしてください。")
			return nil
		}
		mediaType := status.ExtendedEntities.Media[0].Type
		if mediaType != "video" && mediaType != "animated_gif" {
			s.SendMessage("動画やgifのツイートにリプライしてください。")
			return nil
		}

		//TODO: Supports company videos.
		var bestVariant *twitter.VideoVariant
		n := len(status.ExtendedEntities.Media[0].VideoInfo.Variants)
		for i := 0; i < n; i++ {
			variant := status.ExtendedEntities.Media[0].VideoInfo.Variants[i]
			if variant.ContentType == "video/mp4" && (bestVariant == nil || bestVariant.Bitrate < variant.Bitrate) {
				bestVariant = &variant
			}
		}

		if bestVariant == nil {
			s.SendMessage("動画やgifのツイートにリプライしてください。現在、企業向けのツイートメイカーにて作成されたツイートの動画をダウンロードすることはできません。")
			return nil
		}

		e := dbMap.Insert(&Download{
			ScreenName:     s.Tweet.User.ScreenName,
			VideoURL:       bestVariant.URL,
			VideoThumbnail: status.ExtendedEntities.Media[0].MediaURLHttps,
			TweetID:        replyId,
		})
		if e != nil {
			if mysqlErr, ok := e.(*mysql.MySQLError); ok {
				// https://dev.mysql.com/doc/refman/8.0/en/server-error-reference.html
				if mysqlErr.Number == 1062 { // ER_DUP_ENTRY
					s.SendMessage("この動画/gifはすでに保存済みです。下記URLからダウンロードしてください。\nhttps://bot.tomocraft.net/downloads/" + s.Tweet.User.ScreenName)
					return nil
				}
			}
			s.SendMessage("@tomocrafter データベース上にてエラーが発生しました。開発者ができる限り早くサポート致します。")
			return e
		} else {
			s.SendMessage("ダウンロードの準備が整いました。下記URLからダウンロードしてください。\nhttps://bot.tomocraft.net/downloads/" + s.Tweet.User.ScreenName)
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

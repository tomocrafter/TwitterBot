package main

type Download struct {
	ScreenName     string `db:"screen_name, primarykey" json:"screen_name,omitempty"`
	VideoURL       string `db:"video_url" json:"video_url"`
	VideoThumbnail string `db:"video_thumbnail" json:"video_thumbnail"`
	TweetID        int64  `db:"tweet_id" json:"tweet_id"`
	TweetIDStr     string `db:"-" json:"tweet_id_str"`
}
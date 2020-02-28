package main

type Download struct {
	ScreenName     string `db:"screen_name, primarykey"`
	VideoURL       string `db:"video_url"`
	VideoThumbnail string `db:"video_thumbnail"`
	TweetID        int64  `db:"tweet_id"`
}

type DownloadResponse struct {
	ScreenName     string `json:"screen_name,omitempty"`
	VideoURL       string `json:"video_url"`
	VideoThumbnail string `json:"video_thumbnail"`
	TweetID        string `json:"tweet_id"`
}

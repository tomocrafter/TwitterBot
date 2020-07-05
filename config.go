package main

type Config struct {
	Twitter struct {
		ConsumerKey       string `json:"consumer_key"`
		ConsumerSecret    string `json:"consumer_secret"`
		AccessToken       string `json:"access_token"`
		AccessTokenSecret string `json:"access_token_secret"`
	} `json:"twitter"`
	Telegram struct {
		Key    string `json:"key"`
		ChatId string `json:"chat_id"`
	} `json:"telegram"`
	MySQL struct {
		DB       string `json:"db"`
		Addr     string `json:"addr"`
		User     string `json:"user"`
		Password string `json:"password"`
	} `json:"mysql"`
	Redis struct {
		DB       int    `json:"db"`
		Addr     string `json:"addr"`
		Password string `json:"password"`
	} `json:"redis"`
	Path struct {
		Webhook string `json:"webhook"`
	} `json:"path"`
	Sentry struct {
		Dsn string `json:"dsn"`
	} `json:"sentry"`
	Osu struct {
		Key string `json:"key"`
	} `json:"osu"`
	COC struct {
		Key string `json:"key"`
	} `json:"coc"`
}

package config

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
)

// Config represents configuration of entire program.
type Config struct {
	Twitter struct {
		ConsumerKey       string `json:"consumer_key"`
		ConsumerSecret    string `json:"consumer_secret"`
		AccessToken       string `json:"access_token"`
		AccessTokenSecret string `json:"access_token_secret"`
	} `json:"twitter"`
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
}

// LoadConfig loads the configuration of entire program from given file name.
func LoadConfig(fileName string) (*Config, error) {
	file, err := ioutil.ReadFile(fileName)
	if err != nil {
		return nil, fmt.Errorf("opening file: %v", err)
	}

	var c *Config
	err = json.Unmarshal(file, c)
	if err != nil {
		return nil, fmt.Errorf("decoding json: %v", err)
	}
	return c, nil
}

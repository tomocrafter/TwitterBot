package main

import (
	"bufio"
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/dghubble/go-twitter/twitter"
	"github.com/dghubble/oauth1"
	"github.com/gin-gonic/gin"
	"github.com/go-gorp/gorp"
	"github.com/go-redis/redis"
	_ "github.com/go-sql-driver/mysql"
)

var (
	botConfig   Config
	id          int64
	dbMap       *gorp.DbMap
	client      *twitter.Client
	redisClient *redis.Client
	blackList   []string
)

func init() {
	go loadClientBlackList()
	file, err := ioutil.ReadFile("config.json")
	if err != nil {
		panic(err)
	}
	err = json.Unmarshal(file, &botConfig)
	if err != nil {
		panic(err)
	}
}

func readRequestBody(reader *io.ReadCloser) string {
	buf, _ := ioutil.ReadAll(*reader)
	s := string(buf)
	*reader = ioutil.NopCloser(bytes.NewBuffer(buf))
	return s
}

func HandleError(err error) {
	if err != nil {
		go SendMessageToTelegram(err.Error())
	}
}

func SendMessageToTelegram(message string) {
	req, _ := http.NewRequest("GET", fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", botConfig.Telegram.Key), nil)

	query := req.URL.Query()
	query.Set("chat_id", botConfig.Telegram.ChatId)
	query.Set("text", message)
	req.URL.RawQuery = query.Encode()

	resp, err := http.DefaultClient.Do(req)
	defer func() {
		err = resp.Body.Close()
		if err != nil {
			fmt.Printf("Error on closing response body: %+v\n", err)
		}
	}()
	body, readErr := ioutil.ReadAll(resp.Body)
	if readErr != nil {
		fmt.Printf("Error on reading response body: %+v\n", readErr)
	}
	if err != nil {
		fmt.Printf("Error on sending to telegram: %+v%s\n", err, body)
	}
}

func escape(target string) string {
	var sb strings.Builder
	for _, v := range target {
		if v == '_' || v == '%' {
			sb.WriteByte('\\')
		}
		sb.WriteRune(v)
	}
	return sb.String()
}

func loadClientBlackList() {
	fp, err := os.OpenFile("client_black_list.txt", os.O_RDONLY|os.O_CREATE, 0660)
	if err != nil {
		log.Fatal(err)
	}
	defer fp.Close()

	scanner := bufio.NewScanner(fp)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#") { // Comment
			continue
		}
		blackList = append(blackList, line)
	}
	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}
}

func isBlackListed(via string) bool {
	for _, v := range blackList {
		if v == via {
			return true
		}
	}
	return false
}

func main() {
	gin.SetMode(gin.ReleaseMode)

	db, err := sql.Open("mysql", botConfig.MySQL.User+":"+botConfig.MySQL.Password+"@unix(/var/lib/mysql/mysql.sock)/"+botConfig.MySQL.DB+"?parseTime=true")
	if err != nil {
		log.Panic("Error while connecting to MySQL", err)
	}
	dbMap = &gorp.DbMap{Db: db, Dialect: gorp.MySQLDialect{Engine: "InnoDB", Encoding: "UTF8"}}
	dbMap.AddTableWithName(Download{}, "download")
	defer func() {
		_ = db.Close()
	}()

	redisClient = redis.NewClient(&redis.Options{
		Network:  "unix",
		Addr:     "/tmp/redis.sock",
		Password: botConfig.Redis.Password,
		DB:       botConfig.Redis.DB,
	})

	config := oauth1.NewConfig(botConfig.Twitter.ConsumerKey, botConfig.Twitter.ConsumerSecret)
	token := oauth1.NewToken(botConfig.Twitter.AccessToken, botConfig.Twitter.AccessTokenSecret)
	twitterHttpClient := config.Client(oauth1.NoContext, token)

	client = twitter.NewClient(twitterHttpClient)

	user, _, err := client.Accounts.VerifyCredentials(nil)
	if err != nil {
		log.Panic("Error white fetching user", err)
	}
	log.Println("Logged in to @" + user.ScreenName)
	id = user.ID

	router := gin.New()
	router.Use(gin.Logger())

	router.GET("/", func(context *gin.Context) {
		context.String(200, user.ScreenName+" is online!")
	})
	router.GET("/api/downloads/:user", func(context *gin.Context) {
		if user, ok := context.Params.Get("user"); ok {
			var downloads []Download
			_, err := dbMap.Select(&downloads, "SELECT video_url,video_thumbnail,tweet_id FROM download WHERE screen_name = ?", user)
			if err != nil {
				context.JSON(http.StatusInternalServerError, []Download{})
				fmt.Printf("Error on requesting to MySQL: %+v", err)
				HandleError(err)
			} else {
				n := len(downloads)
				for i := 0; i < n; i++ {
					downloads[i].TweetIDStr = strconv.FormatInt(downloads[i].TweetID, 10)
				}
				context.JSON(http.StatusOK, downloads)
			}
		} else {
			context.JSON(http.StatusBadRequest, []Download{})
		}
	})
	router.GET("/api/suggests", func(context *gin.Context) {
		if query, ok := context.GetQuery("query"); ok {
			var screenNames []string
			_, err := dbMap.Select(&screenNames, "SELECT DISTINCT screen_name FROM download WHERE screen_name LIKE ?", "%"+escape(query)+"%")
			if err != nil {
				context.JSON(http.StatusInternalServerError, []string{})
				fmt.Printf("Error on requesting to MySQL: %+v", err)
				HandleError(err)
			} else {
				context.JSON(http.StatusOK, screenNames)
			}
		} else {
			context.JSON(http.StatusBadRequest, []string{})
		}
	})

	// Routing to /webhook
	router.GET(botConfig.Path.Webhook, HandleCRC)
	router.POST(botConfig.Path.Webhook, AuthTwitter, HandleTwitter)

	err = router.RunUnix("/tmp/bot.sock")
	if err != nil {
		log.Fatal("Error while running gin", err)
	}
}

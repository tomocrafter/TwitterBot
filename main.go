package main

import (
	"bufio"
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/getsentry/sentry-go"
	"github.com/gin-contrib/cors"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/dghubble/go-twitter/twitter"
	"github.com/dghubble/oauth1"
	"github.com/gin-gonic/gin"
	"github.com/go-gorp/gorp"
	"github.com/go-redis/redis"
	_ "github.com/go-sql-driver/mysql"
)

var (
	location = time.FixedZone("Asia/Tokyo", 9*60*60)

	botConfig   Config
	id          int64
	dbMap       *gorp.DbMap
	client      *twitter.Client
	redisClient *redis.Client
	blackList   []string
	lookupQueue = make(map[int64][]func(tweet twitter.Tweet))
)

const (
	// Error
	NotVideoTweet = "動画やgifのツイートにリプライしてください。"
	NoMediaFound  = "動画やgifのツイートにリプライしてください。また、現在、企業向けのツイートメイカーにて作成されたツイートの動画をダウンロードすることはできません。"

	// Redis Key
	NoReply            = "no-reply"
	ShowRateLimitReset = "show-rate-limit-reset"
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
	err := sentry.Init(sentry.ClientOptions{
		Dsn: "https://ab2e5e281e6543d7a98855b06da4e0da@o376900.ingest.sentry.io/5198552",
	})
	if err != nil {
		log.Fatalf("sentry.Init: %s", err)
	}

	defer func() {
		sentry.CaptureMessage("shutting down bot...")
		sentry.Flush(2 * time.Second)
	}()

	sentry.CaptureMessage("starting bot...")

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

	client = twitter.NewClient(config.Client(oauth1.NoContext, token))

	user, _, err := client.Accounts.VerifyCredentials(nil)
	if err != nil {
		log.Panic("Error white fetching user", err)
	}
	log.Println("Logged in to @" + user.ScreenName)
	id = user.ID

	router := gin.New()
	router.Use(gin.Logger())

	// CORS
	corsConfig := cors.DefaultConfig()
	corsConfig.AllowOrigins = []string{"https://bot.tomocraft.net"}
	router.Use(cors.New(corsConfig))

	router.GET("/", func(context *gin.Context) {
		context.String(200, user.ScreenName+" is online!")
	})
	router.GET("/api/downloads/:user", func(context *gin.Context) {
		if user, ok := context.Params.Get("user"); ok {
			var downloads []Download
			_, err := dbMap.Select(&downloads, "SELECT video_url,video_thumbnail,tweet_id FROM download WHERE screen_name = ?", user)
			if err != nil {
				context.JSON(http.StatusInternalServerError, []Download{})
				fmt.Printf("error on requesting to MySQL: %+v", err)
				sentry.CaptureException(err)
			} else {
				n := len(downloads)
				var res = make([]DownloadResponse, n)
				for i := 0; i < n; i++ {
					d := downloads[i]
					res[i] = DownloadResponse{ // TODO
						ScreenName:     d.ScreenName,
						VideoURL:       d.VideoURL,
						VideoThumbnail: d.VideoThumbnail,
						TweetID:        strconv.FormatInt(d.TweetID, 10),
					}
				}
				context.JSON(http.StatusOK, res)
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
				sentry.CaptureException(err)
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

	go func() {
		for {
			resetStr, err := redisClient.Get(ShowRateLimitReset).Result()
			if err != nil {
				if err != redis.Nil {
					sentry.CaptureException(err)
				}
			} else {
				if reset, err := strconv.ParseInt(resetStr, 10, 64); err == nil {
					if reset > time.Now().Unix() {
						time.Sleep(5 * time.Second)
						continue
					}
				} else {
					sentry.CaptureMessage(fmt.Sprintf("Not int64 value (%s) passed by redis with key %s", resetStr, ShowRateLimitReset))
				}
			}

			go func() {
				if len(lookupQueue) == 0 {
					return
				}

				ids := make([]int64, 0, len(lookupQueue))
				for id := range lookupQueue {
					ids = append(ids, id)
				}
				log.Printf("Looking up tweets: %v", ids)

				fallback := false
				tweets, _, err := client.Statuses.Lookup(ids, nil)
				if err != nil {
					if err.(twitter.APIError).Errors[0].Code == 88 { // Rate limit exceeded
						sentry.CaptureMessage("API /statuses/lookup exceeded rate limit!")
						fallback = true
					} else {
						sentry.CaptureException(err)
						return
					}
				}

				if fallback {
					for id, cbs := range lookupQueue {
						tweet, resp, err := client.Statuses.Show(id, &twitter.StatusShowParams{
							TrimUser:         twitter.Bool(true),
							IncludeMyRetweet: twitter.Bool(false),
							TweetMode:        "extended",
						})

						if err != nil {
							if resp != nil {
								if err.(twitter.APIError).Errors[0].Code == 88 { // Rate limit exceeded
									sentry.CaptureMessage("API /statuses/show/:id exceeded rate limit!")
									redisClient.Set("show-rate-limit-reset", resp.Header.Get("x-rate-limit-reset"), 0)
									break
								}
								if resp.StatusCode == 404 { // Tweet already deleted.
									continue
								}
							}
							continue
						}

						for _, cb := range cbs {
							go cb(*tweet)
						}
					}
				} else {
					// Ignore tweet that already deleted.
					for _, tweet := range tweets {
						cbs, ok := lookupQueue[tweet.ID]
						if !ok {
							continue
						}
						for _, cb := range cbs {
							go cb(tweet)
						}
					}
				}

				lookupQueue = make(map[int64][]func(tweet twitter.Tweet))
			}()
			time.Sleep(5 * time.Second)
		}
	}()

	err = router.RunUnix("/var/run/twitter/bot.sock")
	if err != nil {
		log.Fatal("Error while running gin", err)
	}
}

func RegisterLookupHandler(id int64, handler func(tweet twitter.Tweet)) {
	lookupQueue[id] = append(lookupQueue[id], handler)
}

func IsTweetRestricting() bool {
	now := time.Now()
	return now.Hour() == 3 && now.Minute() >= 30 && now.Minute() <= 40 // 3:30 ~ 3:40
}

func GetVideoVariant(status *twitter.Tweet) (*twitter.VideoVariant, error) {
	if status.ExtendedEntities == nil {
		return nil, errors.New(NotVideoTweet)
	}
	if len(status.ExtendedEntities.Media) != 1 {
		return nil, errors.New(NotVideoTweet)
	}
	mediaType := status.ExtendedEntities.Media[0].Type
	if mediaType != "video" && mediaType != "animated_gif" {
		return nil, errors.New(NotVideoTweet)
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
		return nil, errors.New(NoMediaFound)
	}

	return bestVariant, nil
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

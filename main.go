package main

import (
	"bufio"
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/gin-contrib/cors"

	"github.com/dghubble/oauth1"
	"github.com/gin-gonic/gin"
	"github.com/go-gorp/gorp"
	"github.com/go-redis/redis"
	_ "github.com/go-sql-driver/mysql"
	"github.com/tomocrafter/go-twitter/twitter"
)

var (
	location = time.FixedZone("Asia/Tokyo", 9*60*60)
	r        = regexp.MustCompile(`<a href=".*?" rel="nofollow">(.*?)</a>`)

	botConfig      Config
	id             int64
	dbMap          *gorp.DbMap
	client         *twitter.Client
	redisClient    *redis.Client
	blackList      []string
	queueProcessor *lookupQueue

	// Error
	errNotVideoTweet = errors.New("動画やgifのツイートにリプライしてください。")
	errNoMediaFound  = errors.New("動画やgifのツイートにリプライしてください。また、現在、企業向けのツイートメイカーにて作成されたツイートの動画をダウンロードすることはできません。")
)

const (
	// Redis Key
	NoReply            = "no-reply-id"
	ShowRateLimitReset = "show-rate-limit-reset"
)

func init() {
	go loadDeniedClientList()
	file, err := ioutil.ReadFile("config.json")
	if err != nil {
		panic(err)
	}
	err = json.Unmarshal(file, &botConfig)
	if err != nil {
		panic(err)
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

func loadDeniedClientList() {
	fp, err := os.OpenFile("denied_clients.txt", os.O_RDONLY|os.O_CREATE, 0660)
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
	var err error
	err = sentry.Init(sentry.ClientOptions{
		Dsn: botConfig.Sentry.Dsn,
	})
	if err != nil {
		log.Fatal("sentry.Init: ", err)
	}

	defer func() {
		sentry.Flush(2 * time.Second)
	}()

	gin.SetMode(gin.ReleaseMode)

	db, err := sql.Open("mysql", botConfig.MySQL.User+":"+botConfig.MySQL.Password+"@"+botConfig.MySQL.Addr+"/"+botConfig.MySQL.DB+"?parseTime=true")
	if err != nil {
		log.Fatal("Error while connecting to MySQL", err)
	}
	dbMap = &gorp.DbMap{Db: db, Dialect: gorp.MySQLDialect{Engine: "InnoDB", Encoding: "UTF8"}}
	dbMap.AddTableWithName(Download{}, "download")
	defer func() {
		_ = db.Close()
	}()

	redisClient = redis.NewClient(&redis.Options{
		Network:  "unix",
		DB:       botConfig.Redis.DB,
		Addr:     botConfig.MySQL.Addr,
		Password: botConfig.Redis.Password,
	})

	config := oauth1.NewConfig(botConfig.Twitter.ConsumerKey, botConfig.Twitter.ConsumerSecret)
	token := oauth1.NewToken(botConfig.Twitter.AccessToken, botConfig.Twitter.AccessTokenSecret)

	client = twitter.NewClient(config.Client(oauth1.NoContext, token))

	user, _, err := client.Accounts.VerifyCredentials(nil)
	if err != nil {
		log.Fatal("Error white fetching user", err)
	}
	log.Println("Logged in to @" + user.ScreenName)
	id = user.ID
	user = nil

	router := gin.New()
	router.Use(gin.Logger())

	// CORS
	router.Use(cors.New(cors.Config{
		AllowMethods:     []string{"GET", "POST"},
		AllowHeaders:     []string{"Origin", "Content-Length", "Content-Type"},
		AllowOrigins:     []string{"https://bot.tomocraft.net"},
		AllowCredentials: false,
		MaxAge:           12 * time.Hour,
	}))

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
			_, err := dbMap.Select(&screenNames, "SELECT DISTINCT screen_name FROM download WHERE screen_name LIKE ? LIMIT 10", "%"+escape(query)+"%")
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

	// Routing to GET /webhook for crc test!
	router.GET(botConfig.Path.Webhook, twitter.CreateCRCHandler(botConfig.Twitter.ConsumerSecret))

	// Routing to POST /webhook for handling webhook payload!
	payloads := make(chan interface{})

	handler, err := twitter.CreateWebhookHandler(payloads)
	if err != nil {
		sentry.CaptureException(err)
		log.Fatal("Error while creating webhook handler", err)
	}
	// Make a debug handler to prints body of webhook.
	debug := func(context *gin.Context) {
		reader := &context.Request.Body
		buf, _ := ioutil.ReadAll(*reader)
		s := string(buf)
		*reader = ioutil.NopCloser(bytes.NewBuffer(buf))
		log.Printf("webhook debug:\n%s\n", s)
	}
	router.POST(botConfig.Path.Webhook, twitter.CreateTwitterAuthHandler(botConfig.Twitter.ConsumerSecret), debug, handler)

	// Start listening payloads from webhook
	go listen(payloads)

	// Initialize queueProcessor
	queueProcessor = NewLookupQueue()
	go queueProcessor.StartTicker()

	// Now start serving!
	err = router.RunUnix("/var/run/twitter/bot.sock")
	if err != nil {
		err = fmt.Errorf("an error occurred while running gin: %s", err)
		sentry.CaptureException(err)
		log.Fatal(err)
	}
}

// IsTimeRestricting は3:30から3:40の間だけtrueを返し、それ以外の時間の場合はfalseを返します
func IsTimeRestricting() bool {
	now := time.Now()
	return now.Hour() == 3 && now.Minute() >= 30 && now.Minute() <= 40 // 3:30 ~ 3:40
}

func GetVideoVariant(status *twitter.Tweet) (*twitter.VideoVariant, error) {
	if status.ExtendedEntities == nil {
		return nil, errNotVideoTweet
	}
	if len(status.ExtendedEntities.Media) != 1 {
		return nil, errNotVideoTweet
	}
	media := status.ExtendedEntities.Media[0]
	if media.Type != "video" && media.Type != "animated_gif" {
		return nil, errNotVideoTweet
	}

	//TODO: Supports company videos.
	var bestVariant *twitter.VideoVariant
	n := len(media.VideoInfo.Variants)
	for i := 0; i < n; i++ {
		variant := media.VideoInfo.Variants[i]
		if variant.ContentType == "video/mp4" && (bestVariant == nil || bestVariant.Bitrate < variant.Bitrate) {
			bestVariant = &variant
		}
	}

	if bestVariant == nil {
		return nil, errNoMediaFound
	}

	return bestVariant, nil
}

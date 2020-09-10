package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/tomocrafter/TwitterBot/config"
	"github.com/tomocrafter/TwitterBot/routes"
	"go.uber.org/zap"

	"github.com/getsentry/sentry-go"
	"github.com/go-gorp/gorp"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/dghubble/oauth1"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/tomocrafter/go-twitter/twitter"
)

var (
	location = time.FixedZone("Asia/Tokyo", 9*60*60)

	id             int64
	dbMap          *gorp.DbMap
	client         *twitter.Client
	redisClient    *redis.Client
	deniedClients  []string
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

var logger *zap.Logger

func init() {
	logger, _ = zap.NewProduction()
	go loadDeniedClientList()
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
		logger.Fatal("failed to open denied clients setting file", zap.Error(err))
	}
	defer fp.Close()

	scanner := bufio.NewScanner(fp)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line[0] == '#' { // Comment
			continue
		}
		deniedClients = append(deniedClients, line)
	}
	if err := scanner.Err(); err != nil {
		logger.Fatal("failed to read denied clients setting file", zap.Error(err))
	}
}

func isDeniedClient(via string) bool {
	for _, v := range deniedClients {
		if v == via {
			return true
		}
	}
	return false
}

func main() {
	var err error
	defer logger.Sync()

	// Load config
	config, err := config.LoadConfig("config.json")
	if err != nil {
		logger.Fatal("failed to load config from file", zap.Error(err))
	}

	// Init sentry client
	err = sentry.Init(sentry.ClientOptions{
		Dsn: config.Sentry.Dsn,
	})
	if err != nil {
		logger.Fatal("failed to initilize sentry client",
			zap.Error(err),
			zap.String("dsn", config.Sentry.Dsn),
		)
	}

	defer func() {
		sentry.Flush(2 * time.Second)
	}()

	gin.SetMode(gin.ReleaseMode)

	router := gin.New()
	routes.InitRoutes(router, config)

	redisClient = redis.NewClient(&redis.Options{
		Network:  "unix",
		DB:       config.Redis.DB,
		Addr:     config.MySQL.Addr,
		Password: config.Redis.Password,
	})

	user, _, err := client.Accounts.VerifyCredentials(nil)
	if err != nil {
		logger.Fatal("failed to fetch user", zap.Error(err))
	}
	logger.Info("successfully logged in", zap.String("user", user.ScreenName))
	id = user.ID
	user = nil

	// Routing to GET /webhook for crc test!
	router.GET(config.Path.Webhook, twitter.CreateCRCHandler(config.Twitter.ConsumerSecret))

	// Initialize queueProcessor
	queueProcessor = NewLookupQueue()
	go queueProcessor.StartTicker()

	// Start Message Queue Processor
	go MessageSendTicker()

	// Now start serving!
	err = router.RunUnix("/var/run/twitter/bot.sock")
	if err != nil {
		err = fmt.Errorf("an error occurred while running gin: %s", err)
		sentry.CaptureException(err)
		logger.Fatal("failed to start gin web server", zap.Error(err))
	}
}

// initDB initializes and establishs database connection.
func initDB(c *config.Config) (*gorm.DB, error) {
	dsn := c.MySQL.User + ":" + c.MySQL.Password + "@" + c.MySQL.Addr + "/" + c.MySQL.DB + "?charset=utf8&parseTime=true"
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("connecting to MySQL: %v", err)
	}

	return db, nil
}

// initTwitterClient initializes twitter client from specified config.
func initTwitterClient(c *config.Config) *twitter.Client {
	config := oauth1.NewConfig(c.Twitter.ConsumerKey, c.Twitter.ConsumerSecret)
	token := oauth1.NewToken(c.Twitter.AccessToken, c.Twitter.AccessTokenSecret)

	return twitter.NewClient(config.Client(oauth1.NoContext, token))
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

package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/go-redis/redis"
	"github.com/tomocrafter/go-twitter/twitter"
	"go.uber.org/zap"
)

type lookupQueue struct {
	ticker    *time.Ticker
	executing bool

	queue *Queue
}

type Callback func(tweet twitter.Tweet)
type Queue map[int64][]Callback

func NewLookupQueue() *lookupQueue {
	queue := make(Queue)
	return &lookupQueue{
		ticker: time.NewTicker(1 * time.Second),
		queue:  &queue,
	}
}

func (t *lookupQueue) LookupTweetBlocking(id int64) twitter.Tweet {
	tc := make(chan twitter.Tweet, 1)

	t.EnqueueLookupHandler(id, func(tweet twitter.Tweet) {
		tc <- tweet
	})

	tweet, ok := <-tc
	if !ok {
		panic("unrecoverable panic occurred: channel unexpectedly closed while looking up tweet")
	}
	return tweet
}

// EnqueueLookupHandler はキューに検索するIDとツイートを処理するコールバックを追加します
func (t *lookupQueue) EnqueueLookupHandler(id int64, handler Callback) {
	(*t.queue)[id] = append((*t.queue)[id], handler)
}

// StartTicker は1秒間に一度キューの中身をすべて検索にかけ、コールバックを呼びます
func (t *lookupQueue) StartTicker() {
	for n := range t.ticker.C {
		go t.Execute(n)
	}
}

func (t *lookupQueue) Execute(n time.Time) {
	if t.executing {
		sentry.CaptureMessage("called progress function during executing")
		return
	}

	t.executing = true
	defer func() {
		t.executing = false
	}()

	// Check for redis rate limit.
	resetStr, err := redisClient.Get(ShowRateLimitReset).Result()
	if err != nil {
		if err != redis.Nil {
			sentry.CaptureException(err)
			return // DO NOT execute statuses/lookup if redis is down!
		}
	} else {
		if reset, err := strconv.ParseInt(resetStr, 10, 64); err == nil {
			if reset > n.Unix() {
				return // rate limited!
			}
		} else {
			sentry.CaptureException(fmt.Errorf("not int64 value (%s) passed by redis with key %s", resetStr, ShowRateLimitReset))

			// Reset the value of redis
			redisClient.Del(ShowRateLimitReset)
		}
	}

	// Move queue here
	queue := *t.queue

	if len(queue) == 0 {
		return // queue is still empty then do nothing.
	}

	// and re-create
	newQueue := make(Queue)
	t.queue = &newQueue

	// Make the list of ids for lookup
	ids := make([]int64, 0, len(queue))
	for id := range queue {
		ids = append(ids, id)
	}

	logger.Info("looking up tweet(s)", zap.Int64s("ids", ids))

	// fallback to the statuses/show endpoint if statuses/lookup endpoint is exceeded rate limit.
	fallbackToShow := false

	tweets, resp, err := client.Statuses.Lookup(ids, &twitter.StatusLookupParams{
		TrimUser:        twitter.Bool(true),
		IncludeEntities: twitter.Bool(true),
		TweetMode:       "extended",
	})
	if err != nil {
		if resp != nil {
			if err.(twitter.APIError).Errors[0].Code == 88 {
				sentry.CaptureMessage("API /statuses/lookup somehow exceeded rate limit!")
				fallbackToShow = true
			} else {
				sentry.CaptureException(fmt.Errorf("non rate limit error occurred while calling /statuses/lookup: %s", err))
				return
			}
		} else {
			sentry.CaptureException(fmt.Errorf("connection error occurred while calling /statuses/lookup: %s", err))
			return
		}
	}

	if fallbackToShow {
		for _, id := range ids {
			tweet, resp, err := client.Statuses.Show(id, &twitter.StatusShowParams{
				TrimUser:         twitter.Bool(true),
				IncludeMyRetweet: twitter.Bool(false),
				IncludeEntities:  twitter.Bool(true),
				TweetMode:        "extended",
			})
			if err != nil {
				if resp != nil {
					if err.(twitter.APIError).Errors[0].Code == 88 { // Rate limit exceeded
						sentry.CaptureMessage("API /statuses/show/:id exceeded rate limit!")
						redisClient.Set(ShowRateLimitReset, resp.Header.Get("x-rate-limit-reset"), 0)
						break // Process only the retrieved tweets.
					}
					if resp.StatusCode == 404 { // Tweet already deleted.
						continue
					}
				}
				sentry.CaptureException(fmt.Errorf("error occurred while calling /statuses/show: %s", err))
				continue
			}

			tweets = append(tweets, *tweet)
		}
	}

	foundIds := make([]int64, 0, len(ids))

	// Calling callbacks!
	// Ignore tweet that already deleted.
	var wg sync.WaitGroup
	for _, tweet := range tweets {
		foundIds = append(foundIds, tweet.ID)
		cbs := queue[tweet.ID]

		for _, cb := range cbs {
			wg.Add(1)
			go func(tweet twitter.Tweet, cb Callback) {
				defer wg.Done()
				cb(tweet)
			}(tweet, cb)
		}
	}
	// wait until all of worker goroutines (callback caller) done.
	wg.Wait()

	if len(foundIds) < len(ids) {
		w := bufio.NewWriterSize(os.Stdout, 512)
		_, _ = w.WriteString("Could not fetch tweet(s): [")

		// writing the difference of ids and foundIds to stdout
		// ref. https://stackoverflow.com/questions/19374219/how-to-find-the-difference-between-two-slices-of-strings
		for _, s1 := range ids {
			found := false
			for _, s2 := range foundIds {
				if s1 == s2 {
					found = true
					break
				}
			}
			// String not found. We add it to return slice
			if !found {
				// TODO: faster way to write int64 to buffer without casting to string.
				_, _ = w.WriteString(string(s1))
			}
		}
		_, _ = w.WriteRune(']')

		// Now print to stdout!
		_ = w.Flush()
	}
}

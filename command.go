package main

import (
	"fmt"
	"github.com/dghubble/go-twitter/twitter"
	"github.com/getsentry/sentry-go"
	"reflect"
	"runtime/debug"
	"strings"
)

var handlers = map[string]Executor{
	"time": TimeCommand,

	"download": DownloadCommand,
	"dl":       DownloadCommand,

	"roll": RollCommand,

	"omikuji": OmikujiCommand,
	"おみくじ":    OmikujiCommand,
	"おみくじ🎰":  OmikujiCommand,
}

type Executor func(sender CommandSender, args []string) error

//
func parseCommand(c string) (string, []string) {
	if IsBlank(c) {
		return "", []string{}
	}

	split := strings.Fields(c)

	return strings.ToLower(split[0]), split[1:]
}

func Dispatch(s CommandSender, c string) {
	c = strings.TrimSpace(c)
	label, args := parseCommand(c)

	// TODO: DM Handle Practice

	defer func() {
		if r := recover(); r != nil {
			err := fmt.Errorf("panic recovered: %v\n%s", r, string(debug.Stack()))
			fmt.Printf("Panicking on executing command: %+v\n", err)
			sentry.CaptureException(err)
		}
	}()

	var command Executor
	if label == "" {
		if tl, ok := s.(TimelineSender); ok {
			replyId := tl.Tweet.InReplyToStatusID
			if replyId != 0 {
				RegisterLookupHandler(replyId, func(tweet twitter.Tweet) {
					if _, err := GetVideoVariant(&tweet); err == nil { // If target tweet has downloadable media
						tl.CacheReply = &tweet
						command = DownloadCommand
					} else {
						command = TimeCommand
					}

					err := command(tl, args)
					if err != nil {
						sentry.CaptureException(err)
					}
					return
				})
			} else {
				command = TimeCommand
			}
		} else {
			fmt.Println("sender:", reflect.TypeOf(s))
			return
		}
	} else {
		command = handlers[label]
	}

	if command == nil {
		if s, ok := s.(DirectMessageSender); ok {
			if handleQuickTime(s) {
				return
			}
		}
		return
	}
	err := command(s, args)
	if err != nil {
		sentry.CaptureException(err)
	}
}

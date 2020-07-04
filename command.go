package main

import (
	"fmt"
	"reflect"
	"runtime/debug"
	"strings"
	"unicode"

	"github.com/getsentry/sentry-go"
)

var handlers = map[string]Executor{
	"time": timeCommand,

	"download": downloadCommand,
	"dl":       downloadCommand,

	"roll": RollCommand,

	"omikuji": OmikujiCommand,
	"おみくじ":    OmikujiCommand,
	"おみくじ🎰":   OmikujiCommand,
}

type Executor func(sender CommandSender, args []string)

func isBlank(str string) bool {
	for _, char := range str {
		if !unicode.IsSpace(char) {
			return false
		}
	}
	return true
}

func parseCommand(c string) (string, []string) {
	if isBlank(c) {
		return "", []string{}
	}

	split := strings.Fields(c)

	return strings.ToLower(split[0]), split[1:]
}

// Dispatch executes command that passed by webhook listener,
// Blocking will occurs if tweet need to be looked up.
// and then execute command in blocking.
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
			replyID := tl.Tweet.InReplyToStatusID
			if replyID != 0 {
				tweet := queueProcessor.LookupTweetBlocking(replyID)
				if _, err := GetVideoVariant(&tweet); err == nil { // If target tweet has downloadable media
					tl.ReplyCache = &tweet
					command = downloadCommand
				} else {
					command = timeCommand
				}
			} else {
				command = timeCommand
			}
		} else {
			fmt.Println("non timeline sender sent empty command: ", reflect.TypeOf(s))
		}
	} else {
		command = handlers[label]
	}

	if command == nil { // If unknown command has issued
		if s, ok := s.(DirectMessageSender); ok { // and If sender is from direct message.
			handleQuickTime(s)
		}
		return
	}

	command(s, args)
}

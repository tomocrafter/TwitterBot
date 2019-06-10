package main

import (
	"fmt"
	"reflect"
	"runtime/debug"
	"strings"
)

var handlers = map[string]Executor{
	"time": TimeCommand,

	"download": DownloadCommand,
	"dl":       DownloadCommand,

	"roll": RollCommand,
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

	var command Executor
	if label == "" {
		if _, ok := s.(TimelineSender); ok {
			command = TimeCommand
		} else {
			fmt.Println("sender:", reflect.TypeOf(s))
			return
		}
	} else {
		command = handlers[label]
	}

	// TODO: DM Handle Practice

	defer func() {
		if r := recover(); r != nil {
			err := fmt.Errorf("panic recovered: %v\n%s", r, string(debug.Stack()))
			fmt.Printf("Panicking on executing command: %+v\n", err)
			HandleError(err)
		}
	}()

	if command == nil {
		if s, ok := s.(DirectMessageSender); ok {
			if handleQuickTime(s) {
				return
			}
		}
		return
	}
	err := command(s, args)
	HandleError(err)
}

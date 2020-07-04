package main

import (
	"math/rand"
	"strconv"
	"time"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

func RollCommand(s CommandSender, args []string) {
	if len(args) == 0 {
		s.SendMessage(s.GetName() + " rolls " + strconv.Itoa(rand.Intn(100)+1) + " points(s)")
	} else if len(args) > 1 {
		s.SendMessage("選ばれたのは\n\n" + args[rand.Intn(len(args))] + "\n\nでした。")
	} else if max, err := strconv.ParseInt(args[0], 10, 32); err != nil {
		s.SendMessage("数値、もしくは2,147,483,647以下の数値、または要素を一つ以上指定してください。")
	} else {
		if max <= 1 {
			s.SendMessage("1以上の数字を指定してください。")
		} else {
			s.SendMessage(s.GetName() + " rolls " + strconv.FormatInt(rand.Int63n(max)+1, 10) + " point(s)")
		}
	}
}

package signal

import (
	"fmt"
	"github.com/nnanhthu/go-stomp-update"
	"math/rand"
	"strconv"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/dchest/uniuri"
)

// Handlepanic prevent panic
func handlepanic(data ...interface{}) {
	if a := recover(); a != nil {
		spew.Println("===========This data make signal panic==============")
		spew.Dump(data...)
		fmt.Println("RECOVER", a)
	}
}

func paddedRandomInt(max int) string {
	var (
		ml = len(strconv.Itoa(max))
		ri = rand.Intn(max)
		is = strconv.Itoa(ri)
	)

	if len(is) < ml {
		is = strings.Repeat("0", ml-len(is)) + is
	}

	return is
}

func createUrl(url string) string {
	return url + paddedRandomInt(999) + "/" + uniuri.New() + "/websocket"
}

func remove(items []stomp.Subscription, item stomp.Subscription) []stomp.Subscription {
	newitems := []stomp.Subscription{}

	for _, i := range items {
		if i != item {
			newitems = append(newitems, i)
		}
	}

	return newitems
}

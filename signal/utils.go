package signal

import (
	"fmt"
	"github.com/nnanhthu/go-stomp-update"
	"math/rand"
	"strconv"
	"strings"
	"time"

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

func formatSubscriptionId() string {
	timestamp := time.Now().UnixNano() / int64(time.Millisecond)
	random := rand.Intn(1000)
	return "sub-" + strconv.FormatInt(timestamp, 10) + "-" + strconv.Itoa(random)
}

// -- Helper funcs --------------------------------------------------------------------------------

func syncRead(chs ...chan *stomp.Message) chan []*stomp.Message {
	outChan := make(chan []*stomp.Message, 1000)
	go func() {
		defer close(outChan)
		for rs, ok := recvOneEach(chs...); ok; rs, ok = recvOneEach(chs...) {
			outChan <- rs
		}
	}()
	return outChan
}

func recvOneEach(chs ...chan *stomp.Message) (rs []*stomp.Message, ok bool) {
	ok = true
	for _, ch := range chs {
		if ch == nil {
			continue
		}
		r, ok2 := <-ch
		rs, ok = append(rs, r), ok && ok2
	}
	return rs, ok
}

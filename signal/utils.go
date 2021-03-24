package signal

import (
	"fmt"
	"github.com/nnanhthu/go-stomp-update"
	"math/rand"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/dchest/uniuri"
)

// string
var (
	// OkEvent linter
	OkEvent = "ok"
	// SdpEvent linter
	SdpEvent = "sdp"
	// CandidateEvent linter
	CandidateEvent = "candidate"
	// FullslotEvent linter
	FullslotEvent = "fullslot"
	// HandShakeErrorEvent linter
	HandShakeErrorEvent = "handshakeError"
	// SeatStatesEvent linter
	SeatStatesEvent = "seat"
	// ReconnectEvent linter
	ReconnectEvent = "reconnect"
	// ReconnectOkEvent linter
	ReconnectOkEvent = "reconnect-ok"
	// ReconnectEvent linter
	ReconnectErrEvent = "reconnectErr"
	// MessageEvent linter
	MessageEvent = "message"
	// DataEvent linter
	DataEvent = "data"
	// NewStreamEvent linter
	NewStreamEvent = "NEW_STREAM"
	// RemoveStreamEvent linter
	RemoveStreamEvent = "REMOVE_STREAM"
	// RemoveStreamEvent linter
	RemoveRelayEvent = "REMOVE_RELAY"
	// FatherTrackKind linter
	FatherTrackKind = "track"
	// FatherTransceiverkind
	FatherTransceiverkind = "transceiver"
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

// SetValueToSignal to set to signal
func SetValueToSignal(inputs ...interface{}) []interface{} {
	var data []interface{}
	return append(data, inputs...)
}

func getTimeout() int {
	i := 18
	if interval := os.Getenv("WSS_TIME_OUT"); interval != "" {
		j, err := strconv.Atoi(interval)
		if err == nil {
			i = j
		}
	}
	return i
}

func isTimeoutError(err error) bool {
	e, ok := err.(net.Error)
	return ok && e.Timeout()
}

func getReconnectLimiting() int {
	i := 10
	if interval := os.Getenv("RECONNECT_LIMIT"); interval != "" {
		j, err := strconv.Atoi(interval)
		if err == nil {
			i = j
		}
	}
	return i
}

func getMaxIdleConns() int {
	i := 100
	if interval := os.Getenv("MAX_IDLE_CONNECTION"); interval != "" {
		j, err := strconv.Atoi(interval)
		if err == nil {
			i = j
		}
	}
	return i
}

func getMaxConnsPerHost() int {
	i := 20
	if interval := os.Getenv("MAX_CONNECTION_PER_HOST"); interval != "" {
		j, err := strconv.Atoi(interval)
		if err == nil {
			i = j
		}
	}
	return i
}

func getMaxIdleConnsPerHost() int {
	i := 20
	if interval := os.Getenv("MAX_IDLE_CONNECTION_PER_HOST"); interval != "" {
		j, err := strconv.Atoi(interval)
		if err == nil {
			i = j
		}
	}
	return i
}

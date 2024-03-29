package signal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	log "github.com/lamhai1401/gologs/logs"
	"github.com/nnanhthu/go-stomp-update"
	"github.com/nnanhthu/go-stomp-update/frame"
	cli "github.com/nnanhthu/signal/client"
	"github.com/pkg/errors"
	"io/ioutil"
	"net/http"
	"reflect"
	"sync"
	"time"
)

// Signaler to connect signal
type Signaler struct {
	url                      string
	token                    string      // token to authenticate with STOMP server
	conn                     *stomp.Conn // handle connection
	publicChannel            string      // Keep info of channel to resubscribe
	privateChannel           string
	publicSubscription       *stomp.Subscription     // handle public subscription
	privateSubscription      *stomp.Subscription     // handle private subscription
	errChan                  chan string             // err to reconnect
	closeChann               chan int                // close all sk
	restartChann             chan int                // to handler restart msg
	msgChann                 chan *stomp.Message     // msg chann
	sendMsgChann             chan interface{}        // send msg
	processRecvData          func(interface{}) error // to handle process when mess is coming
	timeout                  time.Duration           // timeout to call API, in seconds
	isClosed                 bool                    //
	httpClient               *http.Client
	disConnectTimes          int
	publicSubscriptionTimes  int
	privateSubscriptionTimes int
	mutex                    sync.Mutex // handle concurrent
}

// NewSignaler to create new signaler
func NewSignaler(url string, processRecvData func(interface{}) error, token, publicChannel, privateChannel string, timeout int) *Signaler {
	//Create random url from root url
	newUrl := createUrl(url)
	to := time.Duration(timeout) * time.Second
	var netTransport = &http.Transport{
		//Dial: (&net.Dialer{
		//	Timeout: 5 * time.Second,
		//}).Dial,
		TLSHandshakeTimeout: to,
		MaxConnsPerHost:     getMaxConnsPerHost(),
		MaxIdleConnsPerHost: getMaxIdleConnsPerHost(),
	}
	var netClient = &http.Client{
		Timeout:   to,
		Transport: netTransport,
	}
	signaler := &Signaler{
		url:                      newUrl,
		token:                    token,
		publicChannel:            publicChannel,
		privateChannel:           privateChannel,
		processRecvData:          processRecvData,
		closeChann:               make(chan int, 1),
		msgChann:                 make(chan *stomp.Message, 10000),
		errChan:                  make(chan string, 10),
		restartChann:             make(chan int, 10),
		sendMsgChann:             make(chan interface{}, 1000),
		timeout:                  time.Duration(timeout) * time.Second,
		httpClient:               netClient,
		disConnectTimes:          0,
		publicSubscriptionTimes:  0,
		privateSubscriptionTimes: 0,
	}
	return signaler
}

// Getter, setter
func (s *Signaler) getToken() string {
	return s.token
}

func (s *Signaler) getPublicChannel() string {
	return s.publicChannel
}

func (s *Signaler) getPrivateChannel() string {
	return s.privateChannel
}

func (s *Signaler) getClosechann() chan int {
	return s.closeChann
}

func (s *Signaler) getSendMsgchann() chan interface{} {
	return s.sendMsgChann
}

func (s *Signaler) getRestartChann() chan int {
	return s.restartChann
}

func (s *Signaler) getErrchann() chan string {
	return s.errChan
}

func (s *Signaler) getMsgchann() chan *stomp.Message {
	return s.msgChann
}

func (s *Signaler) getProcessRecvData() func(interface{}) error {
	return s.processRecvData
}

func (s *Signaler) getURL() string {
	return s.url
}

func (s *Signaler) getConn() *stomp.Conn {
	return s.conn
}

func (s *Signaler) getPublicSubscription() *stomp.Subscription {
	return s.publicSubscription
}

func (s *Signaler) getPrivateSubscription() *stomp.Subscription {
	return s.privateSubscription
}

func (s *Signaler) getTimeout() time.Duration {
	return s.timeout
}

func (s *Signaler) getClient() *http.Client {
	return s.httpClient
}

func (s *Signaler) getDisconnectTimes() int {
	return s.disConnectTimes
}

func (s *Signaler) getPublicSubTimes() int {
	return s.publicSubscriptionTimes
}

func (s *Signaler) getPrivateSubTimes() int {
	return s.privateSubscriptionTimes
}

func (s *Signaler) checkClose() bool {
	return s.isClosed
}

func (s *Signaler) setClose(state bool) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.isClosed = state
}

func (s *Signaler) setToken(token string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.token = token
}

func (s *Signaler) setPublicChannel(publicChannel string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.publicChannel = publicChannel
}

func (s *Signaler) setPrivateChannel(privateChannel string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.privateChannel = privateChannel
}

func (s *Signaler) setDisconnectTimes(times int) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.disConnectTimes = times
}

func (s *Signaler) setPublicSubTimes(times int) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.publicSubscriptionTimes = times
}

func (s *Signaler) setPrivateSubTimes(times int) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.privateSubscriptionTimes = times
}

// Relating to connection
func (s *Signaler) setConn(conn *stomp.Conn) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.conn = conn
}

func (s *Signaler) removeConn() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.conn = nil
}

func (s *Signaler) CloseConn() {
	disconnectTimes := s.getDisconnectTimes()
	s.error("Number of disconnect to STOMP failed: %d", disconnectTimes)
	if disconnectTimes < 3 {
		ok := s.disconnect()
		if ok {
			s.removeConn()
			disconnectTimes = 0
			s.setDisconnectTimes(disconnectTimes)
			return
		}
		disconnectTimes += 1
		s.setDisconnectTimes(disconnectTimes)
		s.CloseConn()
	} else {
		if conn := s.getConn(); conn != nil {
			err := conn.MustDisconnect()
			if err != nil {
				s.error(fmt.Errorf("MustDisconnect to STOMP got error: %v", err.Error()))
			}
		}
		s.removeConn()
		disconnectTimes = 0
		s.setDisconnectTimes(disconnectTimes)
	}
}

func (s *Signaler) disconnect() bool {
	if conn := s.getConn(); conn != nil {
		tempChan := make(chan bool, 1)
		done := make(chan int, 0)
		to := time.Duration(getStompTimeout()) * time.Second
		ctx, cancel := context.WithTimeout(context.Background(), to)
		defer cancel()

		go func() {
			err := conn.Disconnect()
			done <- 1
			if err != nil {
				s.error(fmt.Errorf("Disconnect to STOMP got error: %v", err.Error()))
				tempChan <- true
				return
			}
			tempChan <- true
		}()

		go func() {
			select {
			case <-ctx.Done():
				s.error(fmt.Errorf("Timeout when Disconnect to STOMP"))
				tempChan <- false
				break
			case <-done:
				cancel()
			}
		}()

		temp := <-tempChan
		return temp
	}
	return true
}

func (s *Signaler) SubscribePublic(dest string) (*stomp.Subscription, error) {
	publicSubTimes := s.getPublicSubTimes()
	s.error("Number of subcribing public to STOMP failed: %d", publicSubTimes)
	if publicSubTimes < 30 {
		sub := s.subscribe(dest)
		if sub != nil {
			s.setPublicSubscription(sub)
			publicSubTimes = 0
			s.setPublicSubTimes(publicSubTimes)
			return sub, nil
		}
		publicSubTimes += 1
		s.setPublicSubTimes(publicSubTimes)
		return s.SubscribePublic(dest)
	} else {
		publicSubTimes = 0
		s.setPublicSubTimes(publicSubTimes)
		return nil, fmt.Errorf("Can't subscribe to public channel %s after retry 30 times", dest)
	}
}

func (s *Signaler) subscribe(dest string) *stomp.Subscription {
	tempChan := make(chan *stomp.Subscription, 1)
	done := make(chan int, 0)
	to := time.Duration(getStompTimeout()) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), to)
	defer cancel()

	go func() {
		id := formatSubscriptionId()
		sub, err := s.conn.Subscribe(dest, stomp.AckClientIndividual, stomp.SubscribeOpt.Header(frame.Id, id))
		done <- 1
		if err != nil {
			s.error(fmt.Sprintf("cannot subscribe to %s with error %s", dest, err.Error()))
			tempChan <- nil
			return
		}
		tempChan <- sub
	}()

	go func() {
		select {
		case <-ctx.Done():
			s.error(fmt.Errorf("Timeout when subscribe to STOMP"))
			tempChan <- nil
			break
		case <-done:
			cancel()
		}
	}()

	temp := <-tempChan
	return temp
}

func (s *Signaler) SubscribePrivate(dest string) (*stomp.Subscription, error) {
	privateSubTimes := s.getPrivateSubTimes()
	s.error("Number of subscribing private to STOMP failed: %d", privateSubTimes)
	if privateSubTimes < 30 {
		sub := s.subscribe(dest)
		if sub != nil {
			s.setPrivateSubscription(sub)
			privateSubTimes = 0
			s.setPrivateSubTimes(privateSubTimes)
			return sub, nil
		}
		privateSubTimes += 1
		s.setPrivateSubTimes(privateSubTimes)
		return s.SubscribePublic(dest)
	} else {
		privateSubTimes = 0
		s.setPrivateSubTimes(privateSubTimes)
		return nil, fmt.Errorf("Can't subscribe to private channel %s after retry 30 times", dest)
	}
}

// Subscribe to a destination on STOMP Server
//func (s *Signaler) SubscribePublic(dest string) (*stomp.Subscription, error) {
//	id := formatSubscriptionId()
//	sub, err := s.conn.Subscribe(dest, stomp.AckClientIndividual, stomp.SubscribeOpt.Header(frame.Id, id))
//	if err != nil {
//		s.error(err.Error())
//		log.Stack(fmt.Sprintf("cannot subscribe to %s with error %s", dest, err.Error()))
//		//disConnectTimes += 1
//		// Reconnect because at this time, server may be disconnect to client
//		s.RestartConn()
//		return nil, err
//	}
//	s.setPublicSubscription(sub)
//	return sub, nil
//}

//func (s *Signaler) SubscribePrivate(dest string) (*stomp.Subscription, error) {
//	id := formatSubscriptionId()
//	sub, err := s.conn.Subscribe(dest, stomp.AckClientIndividual, stomp.SubscribeOpt.Header(frame.Id, id))
//	if err != nil {
//		s.error(err.Error())
//		log.Stack(fmt.Sprintf("cannot subscribe to %s with error %s", dest, err.Error()))
//		//disConnectTimes += 1
//		// Reconnect because at this time, server may be disconnect to client
//		s.RestartConn()
//		return nil, err
//	}
//	s.setPrivateSubscription(sub)
//	return sub, nil
//}
//Connect to stomp
//Subscribe 2 channels for message broadcast and private message
//Listen to read data from 2 channels
func (s *Signaler) ConnectAndSubscribe() error {
	//Connect to stomp server
	if err := s.connect(0); err != nil {
		return err
	}
	log.Error(fmt.Sprintf("Connect STOMP successfully at time: %v(ms)", time.Now()))
	// subscribe room channel to listen to response from STOMP server
	if publicChannel := s.getPublicChannel(); len(publicChannel) > 0 {
		start := time.Now().UnixNano() / int64(time.Millisecond) //in ms
		if _, err := s.SubscribePublic(publicChannel); err != nil {
			return s.RestartConn()
		}
		log.Info(fmt.Sprintf("Subscribe successfully, start reading: %s", publicChannel))
		end := time.Now().UnixNano() / int64(time.Millisecond) //in ms
		log.Error(fmt.Sprintf("Subscribe %s successfully at time: %v(ms), take %d ms", publicChannel, time.Now(), end-start))
		go s.reading(publicChannel, true)
	}
	if privateChannel := s.getPrivateChannel(); len(privateChannel) > 0 {
		start := time.Now().UnixNano() / int64(time.Millisecond) //in ms
		if _, err := s.SubscribePrivate(privateChannel); err != nil {
			return s.RestartConn()
		}
		log.Info(fmt.Sprintf("Subscribe successfully, start reading: %s", privateChannel))
		end := time.Now().UnixNano() / int64(time.Millisecond) //in ms
		log.Error(fmt.Sprintf("Subscribe %s successfully at time: %v(ms), take %d ms", privateChannel, time.Now(), end-start))
		go s.reading(privateChannel, false)
	}

	return nil
}

func (s *Signaler) RestartConn() error {
	s.CloseConn()
	return s.ConnectAndSubscribe()

	//if err := s.ConnectAndSubscribe(); err != nil {
	//	s.pushError(err.Error())
	//}
}

func (s *Signaler) connect(count int) error {
	s.CloseConn()
	log.Stack(fmt.Sprintf("Connect times: %f", count))
	if count > 0 {
		time.Sleep(time.Duration(1 * time.Second))
	}
	if count >= getReconnectLimiting() {
		log.Error("Fail to connect. Close process")
		err := errors.New("Fail to connect. Close process")
		return err
	}
	url := s.getURL()
	token := s.getToken()
	// Can set readChannelCapacity, writeChannelCapacity through options when calling Dial
	// Create web socket connection first
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(getTimeout())*time.Second)
	defer cancel()
	netConn, _, err := websocket.DefaultDialer.DialContext(ctx, url, nil)
	if err != nil {
		log.Warn("*** STOMP Connection timeout. Try to reconnect")
		netConn = nil
		err = nil
		ctx = nil
		count++
		return s.connect(count)
	}
	//if err != nil {
	//	log.Stack("cannot connect to wss server", err.Error())
	//	count++
	//	return s.connect(count)
	//}
	// Now create the stomp connection

	stompConn, err := stomp.Connect(netConn,
		stomp.ConnOpt.Host(url),
		stomp.ConnOpt.AcceptVersion(stomp.V12),
		stomp.ConnOpt.Header("Authorization", token))

	if err != nil {
		log.Warn("cannot connect to stomp server", err.Error())
		//disConnectTimes += 1
		count++
		return s.connect(count)
	}
	s.setConn(stompConn)
	s.info(fmt.Sprintf("Connecting to %s", url))
	return nil
}

// Relating to subscription
func (s *Signaler) setPublicSubscription(sub *stomp.Subscription) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.publicSubscription = sub
}

func (s *Signaler) removePublicSubscription() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.publicSubscription = nil
}

func (s *Signaler) setPrivateSubscription(sub *stomp.Subscription) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.privateSubscription = sub
}

func (s *Signaler) removePrivateSubscription() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.privateSubscription = nil
}

func (s *Signaler) Unsubscribe() {
	s.info("Start unsubscribe STOMP channels")
	s.unsubscribePublic()
	s.unsubscribePrivate()
	s.info("Finish unsubscribe STOMP channels")
}

func (s *Signaler) unsubscribePublic() bool {
	if sub := s.getPublicSubscription(); sub != nil {
		tempChan := make(chan bool, 1)
		done := make(chan int, 0)
		to := time.Duration(getStompTimeout()) * time.Second
		ctx, cancel := context.WithTimeout(context.Background(), to)
		defer cancel()

		go func() {
			err := sub.Unsubscribe()
			done <- 1
			if err != nil {
				s.error(fmt.Errorf("UnsubscribePublic to STOMP got error: %v", err.Error()))
				tempChan <- true
				return
			}
			tempChan <- true
		}()

		go func() {
			select {
			case <-ctx.Done():
				s.error(fmt.Errorf("Timeout when UnsubscribePublic to STOMP"))
				tempChan <- false
				break
			case <-done:
				cancel()
			}
		}()

		temp := <-tempChan
		s.removePublicSubscription()
		return temp
	}
	s.removePublicSubscription()
	return true
}

func (s *Signaler) unsubscribePrivate() bool {
	if sub := s.getPrivateSubscription(); sub != nil {
		tempChan := make(chan bool, 1)
		done := make(chan int, 0)
		to := time.Duration(getStompTimeout()) * time.Second
		ctx, cancel := context.WithTimeout(context.Background(), to)
		defer cancel()

		go func() {
			err := sub.Unsubscribe()
			done <- 1
			if err != nil {
				s.error(fmt.Errorf("UnsubscribePrivate to STOMP got error: %v", err.Error()))
				tempChan <- true
				return
			}
			tempChan <- true
		}()

		go func() {
			select {
			case <-ctx.Done():
				s.error(fmt.Errorf("Timeout when UnsubscribePrivate to STOMP"))
				tempChan <- false
				break
			case <-done:
				cancel()
			}
		}()

		temp := <-tempChan
		s.removePrivateSubscription()
		return temp
	}
	s.removePrivateSubscription()
	return true
}

// Check subscription and connection
//func (s *Signaler) handlePingHandler(dest string) error {
//	if err := s.sendPong(dest); err != nil {
//		s.pushError(err.Error())
//		return err
//	}
//	return nil
//}
//
//// Note: destination to send pong must have no client subscribing
//// TODO: Find another way to send message to server without destination or way to check server connection without sending msg
//func (s *Signaler) sendPong(dest string) error {
//	// 1. Check subscription is active
//	if sub := s.getSubscription(); sub != nil {
//		if sub.Active() == false {
//			err := errors.New("Subscription was unsubscribed and channel was closed")
//			s.error(err)
//			return err
//		}
//	}
//	// 2. Check connection with server (server is still alive)
//	// send to server with receipt
//	err := s.conn.Send(
//		dest,                  // destination
//		"text/plain",          // content-type
//		[]byte("Keep alive?"), // body
//		stomp.SendOpt.Receipt)
//	if err != nil {
//		return err
//	}
//	return nil
//}

func (s *Signaler) handleCloseHandler(code int, text string) error {
	s.info(fmt.Sprintf("Close connection code: %d, with text: %s. Try to reconnect", code, text))
	// clear all old argument
	// s.pushRestart()
	return nil
}

// info to export log info
func (s *Signaler) info(v ...interface{}) {
	log.Info(fmt.Sprintf("Signal log: [%v]", v))
}

// error to export error info
func (s *Signaler) error(v ...interface{}) {
	log.Error(fmt.Sprintf("Signal log: [%v]", v))
}

// close chans
func (s *Signaler) closeSendMsgChann() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	close(s.sendMsgChann)
	s.sendMsgChann = nil
}

func (s *Signaler) closeErrChann() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	close(s.errChan)
	s.errChan = nil
}

func (s *Signaler) closeMsgChann() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	close(s.msgChann)
	s.msgChann = nil
}

func (s *Signaler) closeCloseChann() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	close(s.closeChann)
	s.closeChann = nil
}

func (s *Signaler) closeRestartChann() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	close(s.restartChann)
	s.restartChann = nil
}

// error chan
func (s *Signaler) pushError(err string) {
	if s.checkClose() && (s.getErrchann() != nil) {
		s.closeErrChann()
		return
	}
	if chann := s.getErrchann(); chann != nil {
		chann <- err
	}
}

// Send message to server through restful API
func (s *Signaler) SendPostAPI(dest string, data interface{}) (int, error) {
	//timeout := s.getTimeout()
	//
	//var netTransport = &http.Transport{
	//	//Dial: (&net.Dialer{
	//	//	Timeout: 5 * time.Second,
	//	//}).Dial,
	//	TLSHandshakeTimeout: timeout,
	//	MaxConnsPerHost: 20,
	//	MaxIdleConnsPerHost:20,
	//}
	//var netClient = &http.Client{
	//	Timeout:   timeout,
	//	Transport: netTransport,
	//}
	jsonValue, err := json.Marshal(data)
	if err != nil {
		return http.StatusServiceUnavailable, err
	}
	netClient := s.getClient()
	request, err := http.NewRequest("POST", dest, bytes.NewBuffer(jsonValue))
	//request, err := netClient.Post(dest, "application/json", bytes.NewBuffer(jsonValue))
	if err != nil {
		return http.StatusServiceUnavailable, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", s.getToken())
	//client := &http.Client{}
	response, err := netClient.Do(request)
	if err != nil {
		log.Stack(fmt.Sprintf("The HTTP request failed with error %s\n", err))
		return http.StatusServiceUnavailable, err
	}
	if response.StatusCode != http.StatusOK {
		log.Stack(fmt.Sprintf("The HTTP request failed with status %s\n", response.Status))
		return response.StatusCode, errors.New(fmt.Sprintf("The HTTP request failed with status %s\n", response.Status))
	}

	defer response.Body.Close()
	return response.StatusCode, nil
}

// Send message to get peer conn list through restful API
func (s *Signaler) GetPeerConnListAPI(dest string, data interface{}) (interface{}, int, error) {
	//timeout := s.getTimeout()
	//
	//var netTransport = &http.Transport{
	//	//Dial: (&net.Dialer{
	//	//	Timeout: 5 * time.Second,
	//	//}).Dial,
	//	TLSHandshakeTimeout: timeout,
	//	MaxConnsPerHost: 20,
	//	MaxIdleConnsPerHost:20,
	//}
	//var netClient = &http.Client{
	//	Timeout:   timeout,
	//	Transport: netTransport,
	//}
	jsonValue, err := json.Marshal(data)
	if err != nil {
		return nil, http.StatusServiceUnavailable, err
	}
	netClient := s.getClient()
	request, err := http.NewRequest("POST", dest, bytes.NewBuffer(jsonValue))
	//request, err := netClient.Post(dest, "application/json", bytes.NewBuffer(jsonValue))
	if err != nil {
		return nil, http.StatusServiceUnavailable, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", s.getToken())
	//client := &http.Client{}
	response, err := netClient.Do(request)
	if err != nil {
		log.Stack(fmt.Sprintf("The HTTP request failed with error %s\n", err))
		return nil, http.StatusServiceUnavailable, err
	}
	if response.StatusCode != http.StatusOK {
		log.Stack(fmt.Sprintf("The HTTP request failed with status %s\n", response.Status))
		return nil, response.StatusCode, errors.New(fmt.Sprintf("The HTTP request failed with status %s\n", response.Status))
	}
	if response.Body != nil {
		defer response.Body.Close()
		body, err := ioutil.ReadAll(response.Body)
		if err != nil {
			return nil, http.StatusServiceUnavailable, errors.Wrap(err, "failed to read body")
		}
		var res interface{}
		err = json.Unmarshal(body, &res)
		if err != nil {
			return nil, http.StatusServiceUnavailable, err
		}
		return res, response.StatusCode, nil
	}
	return nil, response.StatusCode, nil
}

func (s *Signaler) SendGetAPI(dest string) (interface{}, int, error) {
	//timeout := s.getTimeout()
	//var netTransport = &http.Transport{
	//	TLSHandshakeTimeout: timeout,
	//	MaxConnsPerHost: 20,
	//	MaxIdleConnsPerHost:20,
	//}
	//var netClient = &http.Client{
	//	Timeout:   timeout,
	//	Transport: netTransport,
	//}
	netClient := s.getClient()
	request, err := http.NewRequest("GET", dest, nil)
	if err != nil {
		return nil, http.StatusServiceUnavailable, err
	}
	//request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", s.getToken())
	response, err := netClient.Do(request)
	if err != nil {
		log.Stack(fmt.Sprintf("The HTTP request failed with error %s\n", err))
		return nil, http.StatusServiceUnavailable, err
	}
	if response.StatusCode != http.StatusOK {
		log.Stack(fmt.Sprintf("The HTTP request failed with status %s\n", response.Status))
		return nil, response.StatusCode, errors.New(fmt.Sprintf("The HTTP request failed with status %s\n", response.Status))
	}

	defer response.Body.Close()
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, http.StatusServiceUnavailable, errors.Wrap(err, "failed to read body")
	}
	var res interface{}
	err = json.Unmarshal(body, &res)
	if err != nil {
		return nil, http.StatusServiceUnavailable, err
	}
	return res, response.StatusCode, nil
}

func (s *Signaler) SendAPI(method, dest string, data interface{}) (interface{}, int, error) {
	cls, _ := cli.NewClient(dest, s.getToken(), s.getTimeout())
	defer cls.Close()
	return cls.API.Call(method, data)
}

// Proceed relating to message
// Send msg to a destination (channel) on STOMP server
func (s *Signaler) Send(dest string, contentType string, data []byte) error {
	if conn := s.getConn(); conn != nil {
		s.mutex.Lock()
		defer s.mutex.Unlock()
		//if err := conn.Send(dest, contentType, data, stomp.SendOpt.Receipt); err != nil {
		if err := conn.Send(dest, contentType, data); err != nil {
			//disConnectTimes += 1
			//Reconnect because after server proceed failed, it'll disconnect to client
			s.RestartConn()
			return err
		}
		log.Error(fmt.Sprintf("Sent message to stomp dest %s successfully", dest))
		return nil
	}
	return fmt.Errorf("Current connection is nil")
}

// Send message from client to channel node to prepare for sending to a destination on STOMP server
func (s *Signaler) pushSendMsg(msg interface{}) {
	if s.checkClose() && (s.getSendMsgchann() != nil) {
		s.closeSendMsgChann()
		return
	}
	if chann := s.getSendMsgchann(); chann != nil {
		log.Stack(fmt.Sprintf("Number of sent msg in chan: %d", len(chann)))
		chann <- msg
	}
}

// pushMsg call handle msg callback (after receiving msg from STOMP server)
func (s *Signaler) pushMsg(msg *stomp.Message, data interface{}) {
	if s.checkClose() && (s.getMsgchann() != nil) {
		s.info("Signal is closed. Close msg chanel to reset to nil and don't receive msg anymore.")
		s.closeMsgChann()
		return
	}
	if chann := s.getMsgchann(); chann != nil {
		log.Error(fmt.Sprintf("Number of received msg in chan: %d", len(chann)))
		chann <- msg
		log.Error(fmt.Sprintf("Add new msg (%v) to queue at time: %v", data, time.Now()))
	}
}

func (s *Signaler) handleSendMsg(dest string, data interface{}) {
	defer handlepanic(data)
	if data == nil {
		s.info("Cannot send to signal. Input data is nil")
		return
	}

	if s.IsZeroOfUnderlyingType(data) {
		s.info("Cannot send to signal. Data has zero value")
		return
	}

	msg, err := json.Marshal(data)
	if err != nil {
		s.pushError(err.Error())
		return
	}
	if err := s.Send(dest, "application/json", msg); err != nil {
		s.pushError(err.Error())
	}
}

// IsZeroOfUnderlyingType check zero value before doing someting
func (s *Signaler) IsZeroOfUnderlyingType(x interface{}) bool {
	return x == nil || reflect.DeepEqual(x, reflect.Zero(reflect.TypeOf(x)).Interface())
}

func (s *Signaler) sendACK(msg *stomp.Message) bool {
	s.info("Start sending ACK after processing msg")
	if conn := s.getConn(); conn != nil {
		tempChan := make(chan bool, 1)
		done := make(chan int, 0)
		to := time.Duration(getStompTimeout()) * time.Second
		ctx, cancel := context.WithTimeout(context.Background(), to)
		defer cancel()

		go func() {
			err := conn.Ack(msg)
			done <- 1
			if err != nil {
				s.error(fmt.Errorf("Send ACK to STOMP got error: %v", err.Error()))
				tempChan <- true
				return
			}
			s.info("Sent ACK successfully")
			tempChan <- true
		}()

		go func() {
			select {
			case <-ctx.Done():
				s.error(fmt.Errorf("Timeout when send ACK"))
				tempChan <- false
				break
			case <-done:
				cancel()
			}
		}()

		temp := <-tempChan
		return temp
	}
	return true
}

func (s *Signaler) sendNACK(msg *stomp.Message) bool {
	s.info("Start sending NACK after processing msg")
	if conn := s.getConn(); conn != nil {
		tempChan := make(chan bool, 1)
		done := make(chan int, 0)
		to := time.Duration(getStompTimeout()) * time.Second
		ctx, cancel := context.WithTimeout(context.Background(), to)
		defer cancel()

		go func() {
			err := conn.Nack(msg)
			done <- 1
			if err != nil {
				s.error(fmt.Errorf("Send NACK to STOMP got error: %v", err.Error()))
				tempChan <- true
				return
			}
			s.info("Sent NACK successfully")
			tempChan <- true
		}()

		go func() {
			select {
			case <-ctx.Done():
				s.error(fmt.Errorf("Timeout when send NACK"))
				tempChan <- false
				break
			case <-done:
				cancel()
			}
		}()

		temp := <-tempChan
		return temp
	}
	return true
}

func (s *Signaler) handleMsg(msg *stomp.Message) {
	if msg == nil {
		return
	}
	if handler := s.getProcessRecvData(); handler != nil {
		//Get msg body to proceed
		var res interface{}
		err := json.Unmarshal(msg.Body, &res)
		//Add message header before sending Ack

		//s.info(fmt.Sprintf("ADD ACK HEADER TO MESSAGE: %v.", err))
		msg.Header.Add(frame.Ack, "messageId")
		if err != nil {
			log.Stack(fmt.Sprintf("Signaler recv err: %v", err))
			//Send NACK
			s.sendNACK(msg)
			return
		}
		err = handler(res)
		if err != nil {
			//Send NACK
			s.sendNACK(msg)
		} else {
			//Send ACK
			// acknowledge the message
			s.sendACK(msg)
		}
	}
}

func (s *Signaler) pushRestart() {
	if s.checkClose() && (s.getRestartChann() != nil) {
		s.closeRestartChann()
		return
	}
	if chann := s.getRestartChann(); chann != nil {
		chann <- 1
	}
}

func (s *Signaler) pushCloseSignal() {
	if s.checkClose() && (s.getClosechann() != nil) {
		s.error("Error when pushing to close STOMP signal\n")
		s.closeCloseChann()
		return
	}
	if chann := s.getClosechann(); chann != nil {
		s.info("Pushed to Close STOMP signal\n")
		chann <- 1
	}
}

func (s *Signaler) handleRestart() {
	s.RestartConn()
}

// Recv to get result from STOMP server
func (s *Signaler) ReceiveFromPublic() (*stomp.Message, error) {
	var res *stomp.Message

	if sub := s.getPublicSubscription(); sub != nil {
		resp, err := sub.Read()

		if err != nil {
			//disConnectTimes += 1
			return nil, fmt.Errorf("recv err: %v", err)
		}

		if len(resp.Body) == 0 {
			return nil, nil
		}
		res = resp

		//err = json.Unmarshal(resp.Body, &res)
		//if err != nil {
		//	return nil, fmt.Errorf("Signaler recv err: %v", err)
		//}

	}
	return res, nil
}

func (s *Signaler) ReceiveFromPrivate() (*stomp.Message, error) {
	var res *stomp.Message

	if sub := s.getPrivateSubscription(); sub != nil {
		resp, err := sub.Read()

		if err != nil {
			//disConnectTimes += 1
			return nil, fmt.Errorf("recv err: %v", err)
		}

		if len(resp.Body) == 0 {
			return nil, nil
		}
		res = resp

		//err = json.Unmarshal(resp.Body, &res)
		//if err != nil {
		//	return nil, fmt.Errorf("Signaler recv err: %v", err)
		//}

	}
	return res, nil
}

func parseMsg(recv *stomp.Message) interface{} {
	var values interface{}
	err := json.Unmarshal(recv.Body, &values)
	if err != nil {
		return values
		//res, ok := values.(map[string]interface{})
		//if ok {
		//	messageId := ""
		//	if res["messageId"] != nil {
		//		messageId = res["messageId"].(string)
		//	}
		//	if res != nil && res["name"] != nil {
		//		if res["data"] != nil {
		//			d, ok := res["data"].(map[string]interface{})
		//			if ok {
		//				return messageId, res["name"].(string), d["systemTime"]
		//			}
		//		}
		//	}
		//}
	}
	return nil
}

func (s *Signaler) reading(dest string, isPublic bool) {
	defer s.RestartConn()
	for {
		//log.Stack(fmt.Sprintf("Number of disconnect: %f", disConnectTimes))

		var recv *stomp.Message
		var err error
		if isPublic {
			recv, err = s.ReceiveFromPublic()
		} else {
			recv, err = s.ReceiveFromPrivate()
		}
		if err != nil {
			s.error(fmt.Sprintf("reading error: %v. Could be was throw signal. Restarting conn", err))
			return
		}
		if recv == nil {
			continue
		}
		//Parse recv to log
		msgData := parseMsg(recv)
		urgent := false
		//currentTime := time.Now().UnixNano() / int64(time.Millisecond)
		//if systemTime != nil {
		//	sysTime, ok := systemTime.(int64)
		//	if ok {
		//		if currentTime-sysTime > 1000 {
		//			urgent = true
		//		}
		//	}
		//}
		//log.Error(fmt.Sprintf("**********[URGENT:%t]Received new item (MsgId: %s _ Event: %s _ systemTime: %v) from channel %s at time: %v **********",
		//	urgent, msgId, event, systemTime, dest, time.Now()))
		log.Error(fmt.Sprintf("**********[URGENT:%t]Received new item (%v) from channel %s at time: %v **********",
			urgent, msgData, dest, time.Now()))

		//s.info(recv)
		s.pushMsg(recv, msgData)
		recv = nil
	}
}

// Listener to serve requests
func (s *Signaler) serve(dest string) {
	for {
		select {
		case <-s.getClosechann():
			s.close()
			return
		case <-s.getRestartChann():
			s.handleRestart()
		case err := <-s.getErrchann():
			s.error(err)
		case msg := <-s.getMsgchann():
			s.handleMsg(msg)
			//go func() {
			//	start := time.Now().UnixNano() / int64(time.Millisecond) //in ms
			//	s.handleMsg(msg)
			//	end := time.Now().UnixNano() / int64(time.Millisecond) //in ms
			//	total := end - start
			//	data := parseMsg(msg)
			//	urgent := false
			//	if total > 1000 {
			//		urgent = true
			//	}
			//	log.Debug(fmt.Sprintf("[URGENT:%t][%v] processing time of msg: %v", urgent, total, data))
			//}()
		case data := <-s.getSendMsgchann():
			//byteData, _ := json.Marshal(data)
			//obj := &SendObj{}
			//json.Unmarshal(byteData, &obj)
			s.handleSendMsg(dest, data)
		}
	}
}

func (s *Signaler) close() {
	s.info("Start closing livestream signal\n")
	s.setClose(true)
	s.Unsubscribe()
	s.CloseConn()
}

// SendMsg to send data to wss
func (s *Signaler) SendMsg(data interface{}) {
	s.pushSendMsg(data)
}

// Close to running wss process
func (s *Signaler) Close() {
	s.info(fmt.Sprintf("Push to close channel to disconnect to STOMP\n"))
	//s.pushCloseSignal()
	s.close()
}

// Start to running wss process
func (s *Signaler) Start() error {
	s.info("Start connect and subscribe STOMP channel")
	start := time.Now().UnixNano() / int64(time.Millisecond) //in ms
	if err := s.ConnectAndSubscribe(); err != nil {
		return err
	}
	end := time.Now().UnixNano() / int64(time.Millisecond) //in ms
	total := end - start
	s.info(fmt.Sprintf("Finish connect and subscribe with time: %d (ms)", total))

	if publicChannel := s.getPublicChannel(); len(publicChannel) > 0 {
		go s.serve(publicChannel)
	}
	//if privateChannel := s.getPrivateChannel(); len(privateChannel) > 0 {
	//	go s.serve(privateChannel)
	//}

	s.info(fmt.Sprintf("Ready to use room ....!!!! \n"))
	return nil
}

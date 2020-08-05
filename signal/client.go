package signal

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-stomp/stomp"
	log "github.com/lamhai1401/gologs/logs"
	"reflect"
	"sync"
	"time"
)

// Wss constant
const (
	// Time allowed to write a message to the peer.
	writeWait = 30 * time.Second

	// Time allowed to read a message from server.
	readWait = 30 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = stomp.DefaultHeartBeatError // 60 * time.Second
)

// Obj to send from client
type SendObj struct {
	Destination string
	Data        []byte
}

// Signaler to connect signal
type Signaler struct {
	url             string
	Name            string              // signal name father or master or child
	conn            *stomp.Conn         // handle connection
	subscription    *stomp.Subscription // handle subscription
	errChan         chan string         // err to reconnect
	closeChann      chan int            // close all sk
	restartChann    chan int            // to handler restart msg
	msgChann        chan []interface{}  // msg chann
	sendMsgChann    chan interface{}    // send msg
	processRecvData func([]interface{}) // to handle process when mess is coming
	isClosed        bool                //
	mutex           sync.Mutex          // handle concurrent
}

// NewSignaler to create new signaler
func NewSignaler(url string, processRecvData func([]interface{}), name string) *Signaler {
	signaler := &Signaler{
		url:             url,
		Name:            name,
		processRecvData: processRecvData,
		closeChann:      make(chan int),
		msgChann:        make(chan []interface{}, 1000),
		errChan:         make(chan string, 10),
		restartChann:    make(chan int, 10),
		sendMsgChann:    make(chan interface{}, 1000),
	}
	return signaler
}

// Getter, setter
func (s *Signaler) getName() string {
	return s.Name
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

func (s *Signaler) getMsgchann() chan []interface{} {
	return s.msgChann
}

func (s *Signaler) getProcessRecvData() func([]interface{}) {
	return s.processRecvData
}

func (s *Signaler) getURL() string {
	return s.url
}

func (s *Signaler) getConn() *stomp.Conn {
	return s.conn
}

func (s *Signaler) getSubscription() *stomp.Subscription {
	return s.subscription
}

func (s *Signaler) checkClose() bool {
	return s.isClosed
}

func (s *Signaler) setClose(state bool) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.isClosed = state
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

func (s *Signaler) closeConn() {
	if conn := s.getConn(); conn != nil {
		if err := conn.Disconnect(); err != nil {
			s.error(err.Error())
		}
	}
	s.removeConn()
}

func (s *Signaler) connectConn() error {
	u := s.getURL()
	// Can set readChannelCapacity, writeChannelCapacity through options when calling Dial
	conn, err := stomp.Dial("tcp", u)

	if err != nil {
		println("cannot connect to server", err.Error())
		return err
	}
	s.setConn(conn)
	go s.reading()
	s.info(fmt.Sprintf("Connecting to %s", u))
	return nil
}

func (s *Signaler) restartConn() {
	s.closeConn()
	if err := s.connectConn(); err != nil {
		s.pushError(err.Error())
	}
}

// Relating to subscription
func (s *Signaler) setSubscription(sub *stomp.Subscription) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.subscription = sub
}

func (s *Signaler) removeSubscription() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.subscription = nil
}

// Subscribe to a destination on STOMP Server
func (s *Signaler) subscribe(destination string) (*stomp.Subscription, error) {
	sub, err := s.conn.Subscribe(destination, stomp.AckClientIndividual)
	if err != nil {
		s.error(err.Error())
		println("cannot subscribe to", destination, err.Error())
		// Reconnect because at this time, server may be disconnect to client
		s.connectConn()
		return nil, err
	}
	return sub, nil
}

// Unsubscribe from a destination on STOMP Server
func (s *Signaler) unsubscribe() {
	if sub := s.getSubscription(); sub != nil {
		if err := sub.Unsubscribe(); err != nil {
			s.error(err.Error())
		}
	}
	s.removeSubscription()
}

// Check subscription and connection
func (s *Signaler) handlePingHandler(destination string) error {
	if err := s.sendPong(destination); err != nil {
		s.pushError(err.Error())
		return err
	}
	return nil
}

// Note: destination to send pong must have no client subscribing
// TODO: Find another way to send message to server without destination or way to check server connection without sending msg
func (s *Signaler) sendPong(destination string) error {
	// 1. Check subscription is active
	if sub := s.getSubscription(); sub != nil {
		if sub.Active() == false {
			err := errors.New("Subscription was unsubscribed and channel was closed")
			s.error(err)
			return err
		}
	}
	// 2. Check connection with server (server is still alive)
	// send to server with receipt
	err := s.conn.Send(
		destination,           // destination
		"text/plain",          // content-type
		[]byte("Keep alive?"), // body
		stomp.SendOpt.Receipt)
	if err != nil {
		return err
	}
	return nil
}

func (s *Signaler) handleCloseHandler(code int, text string) error {
	s.info(fmt.Sprintf("Close connection code: %d, with text: %s. Try to reconnect", code, text))
	// clear all old argument
	// s.pushRestart()
	return nil
}

// info to export log info
func (s *Signaler) info(v ...interface{}) {
	log.Info(fmt.Sprintf("[%s] %v", s.getName(), v))
}

// error to export error info
func (s *Signaler) error(v ...interface{}) {
	log.Error(fmt.Sprintf("[%s] %v", s.getName(), v))
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

// Proceed relating to message
// Send msg to a destination (channel) on STOMP server
func (s *Signaler) send(destination string, contentType string, data []byte) error {
	if conn := s.getConn(); conn != nil {
		s.mutex.Lock()
		defer s.mutex.Unlock()
		if err := conn.Send(destination, contentType, data, stomp.SendOpt.Receipt); err != nil {
			//Reconnect because after server proceed failed, it'll disconnect to client
			s.connectConn()
			return err
		}
		return nil
	}
	return fmt.Errorf("Current connection is nil")
}

// Send message from client to channel node to prepare for sending to a destination on STOMP server
func (s *Signaler) pushSendMsg(destination string, msg interface{}) {
	if s.checkClose() && (s.getSendMsgchann() != nil) {
		s.closeSendMsgChann()
		return
	}
	if chann := s.getSendMsgchann(); chann != nil {
		println("Number of sent msg in chan: %f", len(chann))
		chann <- msg
	}
}

// pushMsg call handle msg callback (after receiving msg from STOMP server)
func (s *Signaler) pushMsg(msg []interface{}) {
	if s.checkClose() && (s.getMsgchann() != nil) {
		s.closeMsgChann()
		return
	}
	if chann := s.getMsgchann(); chann != nil {
		println("Number of received msg in chan: %f", len(chann))
		chann <- msg
	}
}

func (s *Signaler) handleSendMsg(destination string, data interface{}) {
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
	if err := s.send(destination, "text/plain", msg); err != nil {
		s.pushError(err.Error())
	}
}

// IsZeroOfUnderlyingType check zero value before doing someting
func (s *Signaler) IsZeroOfUnderlyingType(x interface{}) bool {
	return x == nil || reflect.DeepEqual(x, reflect.Zero(reflect.TypeOf(x)).Interface())
}

func (s *Signaler) handleMsg(msg []interface{}) {
	if handler := s.getProcessRecvData(); handler != nil {
		handler(msg)
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
		s.closeCloseChann()
		return
	}
	if chann := s.getClosechann(); chann != nil {
		chann <- 1
	}
}

func (s *Signaler) handleRestart() {
	s.restartConn()
}

// Recv to get result from STOMP server
func (s *Signaler) recv() ([]interface{}, error) {

	var result []interface{}

	if sub := s.getSubscription(); sub != nil {
		resp, err := sub.Read()

		if err != nil {
			return nil, fmt.Errorf("recv err: %v", err)
		}

		if len(resp.Body) == 0 {
			return nil, nil
		}

		err = json.Unmarshal(resp.Body, &result)
		if err != nil {
			return nil, fmt.Errorf("Signaler recv err: %v", err)
		}

		// acknowledge the message
		if conn := s.getConn(); conn != nil {
			err = conn.Ack(resp)
			if err != nil {
				return result, err
			}
		}
	}
	return result, nil
}

func (s *Signaler) reading() {
	defer s.restartConn()
	for {
		recv, err := s.recv()
		if err != nil {
			s.error(fmt.Sprintf("reading error: %v. Could be was throw signal. Restarting conn", err))
			return
		}
		if recv == nil {
			continue
		}
		s.pushMsg(recv)
		recv = nil
	}
}

// Listener to serve requests
func (s *Signaler) serve() {
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
		case data := <-s.getSendMsgchann():
			byteData, _ := json.Marshal(data)
			obj := &SendObj{}
			json.Unmarshal(byteData, &obj)
			s.handleSendMsg(obj.Destination, obj.Data)
		}
	}
}

func (s *Signaler) close() {
	s.setClose(true)
	s.unsubscribe()
	s.closeConn()
}

// SendText to send data to wss
func (s *Signaler) SendText(destination string, data interface{}) {
	s.pushSendMsg(destination, data)
}

// Close to running wss process
func (s *Signaler) Close() {
	s.pushCloseSignal()
}

// Start to running wss process
func (s *Signaler) Start() error {
	if err := s.connectConn(); err != nil {
		return err
	}
	go s.serve()
	s.info(fmt.Sprintf("Ready to use....!!!! \n"))
	return nil
}

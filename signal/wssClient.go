package signal

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"reflect"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	log "github.com/lamhai1401/gologs/logs"
)

// Wss constant
const (
	// Time allowed to write a message to the peer.
	writeWait = 30 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second
)

// Signaler to connect signal
type WssSignaler struct {
	url             string
	Name            string                  // signal name father or master or child
	conn            *websocket.Conn         // handle connection
	errChan         chan string             // err to reconnect
	closeChann      chan int                // close all sk
	restartChann    chan int                // to handler restart msg
	msgChann        chan []interface{}      // msg chann
	sendMsgChann    chan interface{}        // send msg
	processRecvData func(interface{}) error // to handle process when mess is coming
	isClosed        bool                    //
	mutex           sync.Mutex              // handle concurrent
}

// NewSignaler to create new signaler
func NewWssSignaler(url string, processRecvData func(interface{}) error, name string) *WssSignaler {
	signaler := &WssSignaler{
		url:             url,
		Name:            name,
		processRecvData: processRecvData,
		closeChann:      make(chan int),
		msgChann:        make(chan []interface{}, 10000),
		errChan:         make(chan string, 10),
		restartChann:    make(chan int, 10),
		sendMsgChann:    make(chan interface{}, 1000),
	}
	return signaler
}

func (s *WssSignaler) getName() string {
	return s.Name
}

func (s *WssSignaler) getClosechann() chan int {
	return s.closeChann
}

func (s *WssSignaler) getSendMsgchann() chan interface{} {
	return s.sendMsgChann
}

func (s *WssSignaler) getRestartChann() chan int {
	return s.restartChann
}

func (s *WssSignaler) getErrchann() chan string {
	return s.errChan
}

func (s *WssSignaler) getMsgchann() chan []interface{} {
	return s.msgChann
}

func (s *WssSignaler) checkClose() bool {
	return s.isClosed
}

func (s *WssSignaler) setClose(state bool) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.isClosed = state
}

func (s *WssSignaler) getProcessRecvData() func(interface{}) error {
	return s.processRecvData
}

func (s *WssSignaler) getConn() *websocket.Conn {
	return s.conn
}

func (s *WssSignaler) setConn(conn *websocket.Conn) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.conn = conn
}

func (s *WssSignaler) closeConn() {
	if conn := s.getConn(); conn != nil {
		if err := conn.Close(); err != nil {
			s.error(err.Error())
		}
	}
	s.removeConn()
}

func (s *WssSignaler) getURL() string {
	return s.url
}

// info to export log info
func (s *WssSignaler) info(v ...interface{}) {
	log.Info(fmt.Sprintf("[%s] %v", s.getName(), v))
}

// error to export error info
func (s *WssSignaler) error(v ...interface{}) {
	log.Error(fmt.Sprintf("[%s] %v", s.getName(), v))
}

func (s *WssSignaler) handlePingHandler(message string) error {
	if err := s.sendPong(); err != nil {
		s.pushError(err.Error())
		return err
	}
	return nil
}

func (s *WssSignaler) handleCloseHander(code int, text string) error {
	s.info(fmt.Sprintf("Close connection code: %d, with text: %s. Try to reconnect", code, text))
	// clear all old argument
	// s.pushRestart()
	return nil
}

func (s *WssSignaler) sendPong() error {
	err := s.send(websocket.PongMessage, []byte("keppAlive"))
	if err == websocket.ErrCloseSent {
		return nil
	} else if e, ok := err.(net.Error); ok && e.Temporary() {
		return nil
	} else {
		return err
	}
}

func (s *WssSignaler) send(messType int, data []byte) error {
	if conn := s.getConn(); conn != nil {
		if err := conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
			return err
		}
		s.mutex.Lock()
		defer s.mutex.Unlock()
		if err := conn.WriteMessage(messType, data); err != nil {
			return err
		}
		return nil
	}
	return fmt.Errorf("Current connection is nil")
}

// connectConn loop until connected
func (s *WssSignaler) connectConn() error {
	u, err := url.Parse(s.getURL())
	if err != nil {
		return err
	}

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return err
	}
	s.setConn(conn)

	// handler here
	conn.SetPingHandler(s.handlePingHandler)
	conn.SetCloseHandler(s.handleCloseHander)

	// set ping to server
	// timeout := 5 * time.Second
	// go signaler.keepAlive(timeout)

	go s.reading()
	s.info(fmt.Sprintf("Connecting to %s", u.String()))
	return nil
}

func (s *WssSignaler) restartConn() {
	s.closeConn()
	if err := s.connectConn(); err != nil {
		s.pushError(err.Error())
	}
}

func (s *WssSignaler) closeSendMsgChann() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	close(s.sendMsgChann)
	s.sendMsgChann = nil
}

func (s *WssSignaler) closeErrChann() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	close(s.errChan)
	s.errChan = nil
}

func (s *WssSignaler) closeMsgChann() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	close(s.msgChann)
	s.msgChann = nil
}

func (s *WssSignaler) closeCloseChann() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	close(s.closeChann)
	s.closeChann = nil
}

func (s *WssSignaler) closeRestartChann() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	close(s.restartChann)
	s.restartChann = nil
}

func (s *WssSignaler) pushError(err string) {
	if s.checkClose() && (s.getErrchann() != nil) {
		s.closeErrChann()
		return
	}
	if chann := s.getErrchann(); chann != nil {
		chann <- err
	}
}

// pushSendMsg send msg to wss
func (s *WssSignaler) pushSendMsg(msg interface{}) {
	if s.checkClose() && (s.getSendMsgchann() != nil) {
		s.closeSendMsgChann()
		return
	}
	if chann := s.getSendMsgchann(); chann != nil {
		chann <- msg
	}
}

// pushMsg call handle msg callback
func (s *WssSignaler) pushMsg(msg []interface{}) {
	if s.checkClose() && (s.getMsgchann() != nil) {
		s.closeMsgChann()
		return
	}
	if chann := s.getMsgchann(); chann != nil {
		chann <- msg
	}
}

func (s *WssSignaler) pushRestart() {
	if s.checkClose() && (s.getRestartChann() != nil) {
		s.closeRestartChann()
		return
	}
	if chann := s.getRestartChann(); chann != nil {
		chann <- 1
	}
}

func (s *WssSignaler) pushCloseSignal() {
	if s.checkClose() && (s.getClosechann() != nil) {
		s.closeCloseChann()
		return
	}
	if chann := s.getClosechann(); chann != nil {
		chann <- 1
	}
}

func (s *WssSignaler) serve() {
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
			s.handleSendMsg(data)
		}
	}
}

func (s *WssSignaler) handleSendMsg(data interface{}) {
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
	if err := s.send(websocket.TextMessage, msg); err != nil {
		s.pushError(err.Error())
	}
}

// IsZeroOfUnderlyingType check zero value before doing someting
func (s *WssSignaler) IsZeroOfUnderlyingType(x interface{}) bool {
	return x == nil || reflect.DeepEqual(x, reflect.Zero(reflect.TypeOf(x)).Interface())
}

func (s *WssSignaler) handleMsg(msg interface{}) {
	if handler := s.getProcessRecvData(); handler != nil {
		handler(msg)
	}
}

func (s *WssSignaler) handleRestart() {
	s.restartConn()
}

func (s *WssSignaler) removeConn() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.conn = nil
}

// Start to running wss process
func (s *WssSignaler) close() {
	s.setClose(true)
	s.closeConn()
}

// SendText to send data to wss
func (s *WssSignaler) SendText(data interface{}) {
	s.pushSendMsg(data)
}

// Close to running wss process
func (s *WssSignaler) Close() {
	s.pushCloseSignal()
}

// Start to running wss process
func (s *WssSignaler) Start() error {
	if err := s.connectConn(); err != nil {
		return err
	}
	go s.serve()
	s.info(fmt.Sprintf("Ready to use....!!!! \n"))
	return nil
}

// Recv to get result from ws
func (s *WssSignaler) recv() ([]interface{}, error) {

	var result []interface{}

	if conn := s.getConn(); conn != nil {
		_, resp, err := conn.ReadMessage()

		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway) {
				return nil, fmt.Errorf("Websocket closeGoingAway error: %v", err)
			}
			return nil, fmt.Errorf("recv err: %v", err)
		}

		if len(resp) == 0 {
			return nil, nil
		}

		err = json.Unmarshal(resp, &result)
		if err != nil {
			return nil, fmt.Errorf("Signaler recv err: %v", err)
		}
	}
	return result, nil
}

func (s *WssSignaler) reading() {
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

// keepAlive to send ping pong
func (s *WssSignaler) keepAlive(timeout time.Duration) {
	lastResponse := time.Now()
	s.conn.SetPongHandler(func(msg string) error {
		s.conn.SetReadDeadline(time.Now().Add(pongWait))
		lastResponse = time.Now()
		return nil
	})

	for {
		// Signal is close. So stop send ping MESSAGE
		if s.conn == nil {
			return
		}
		err := s.send(websocket.PingMessage, []byte("keepalive"))
		if err != nil {
			return
		}
		time.Sleep(timeout / 2)
		if time.Now().Sub(lastResponse) > timeout {
			return
		}
	}
}

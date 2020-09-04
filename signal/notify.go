package signal

import (
	"fmt"
	"os"
	"sync"

	log "github.com/lamhai1401/gologs/logs"
	"github.com/pion/webrtc/v2"
	"github.com/segmentio/ksuid"
)

// GenerateID to create new string id
func generateID() string {
	id := ksuid.New().String()
	return id
}

// GetNotifyURL to get notify signal
func GetNotifyURL(id string) string {
	if url := os.Getenv("MULTIPLE_URLL"); url != "" {
		return fmt.Sprintf("%s/?id=%s", url, id)
	}
	return fmt.Sprintf("wss://signal-test.dechen.app/?id=%s", id)
}

// NotifySignal handle signal of wertc (candidate, sdp)
type MsgSignal struct {
	id              string
	url             string
	conn            *WssSignaler
	closeWssChannel chan int            // handle close
	sendMsgChannel  chan interface{}    // to save message
	processRecvData func([]interface{}) // handle incoming msg
	isClosed        bool                // handle close signal
	mutex           sync.Mutex
}

// NewNotifySignal create new notify signal
func NewNotifySignal(id string, processRecvData func([]interface{})) *MsgSignal {
	return &MsgSignal{
		id:              fmt.Sprintf("Notify-Signal-%s", generateID()),
		url:             GetNotifyURL(id),
		isClosed:        false,
		sendMsgChannel:  make(chan interface{}, 1000),
		processRecvData: processRecvData,
	}
}

// Start to start signal
func (n *MsgSignal) Start() error {
	conn := NewWssSignaler(n.getURL(), n.getProcessRecvData(), n.getID())
	if err := conn.Start(); err != nil {
		return err
	}
	n.setConn(conn)
	go n.serve()
	return nil
}

// Close to close wss
func (n *MsgSignal) Close() {
	chann := n.getCloseWssChannel()
	if chann == nil {
		return
	}

	if n.checkClose() {
		return
	}
	chann <- 1
}

// Message to send msg to wss (sdp/ice candidate)
func (n *MsgSignal) message(idTo string, event string, data interface{}) {
	// send media state to client
	n.push(SetValueToSignal(idTo, event, data))
}

func (n *MsgSignal) getCloseWssChannel() chan int {
	return n.closeWssChannel
}

func (n *MsgSignal) removeCloseWssChannel() {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	n.closeWssChannel = nil
}

func (n *MsgSignal) closeCloseWssChannel() {
	if chann := n.getCloseWssChannel(); chann != nil {
		close(chann)
		n.removeCloseWssChannel()
	}
}

func (n *MsgSignal) getSendMsgChannel() chan interface{} {
	return n.sendMsgChannel
}

func (n *MsgSignal) removeSendMsgChannel() {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	n.sendMsgChannel = nil
}

func (n *MsgSignal) closeSendMsgChannel() {
	if chann := n.getSendMsgChannel(); chann != nil {
		close(chann)
		n.removeSendMsgChannel()
	}
}

func (n *MsgSignal) serve() {
	for {
		select {
		case <-n.getCloseWssChannel():
			n.close()
			return
		case msg := <-n.getSendMsgChannel():
			n.send(msg)
		}
	}
}

func (n *MsgSignal) push(msg interface{}) {
	chann := n.getSendMsgChannel()
	if chann == nil {
		return
	}
	if !n.checkClose() {
		chann <- msg
	} else {
		n.closeSendMsgChannel()
	}
}

func (n *MsgSignal) send(msg interface{}) {
	if conn := n.getConn(); conn != nil && !n.checkClose() {
		conn.SendText(msg)
	}
}

func (n *MsgSignal) getURL() string {
	return n.url
}

// Close to close signal
func (n *MsgSignal) close() {
	n.setClose(true)
	n.closeConn()
	n.removeProcessRecvData()
}

func (n *MsgSignal) getProcessRecvData() func([]interface{}) {
	return n.processRecvData
}

func (n *MsgSignal) removeProcessRecvData() {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	n.processRecvData = nil
}

func (n *MsgSignal) getConn() *WssSignaler {
	return n.conn
}

func (n *MsgSignal) setConn(conn *WssSignaler) {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	n.conn = conn
}

func (n *MsgSignal) removeConn() {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	n.conn = nil
}

func (n *MsgSignal) closeConn() {
	if conn := n.getConn(); conn != nil {
		conn.close()
		conn = nil
	}
}

func (n *MsgSignal) getID() string {
	return n.id
}

// info to export log info
func (n *MsgSignal) info(v ...interface{}) {
	log.Info(fmt.Sprintf("[%s] %v", n.getID(), v))
}

// error to export error info
func (n *MsgSignal) error(v ...interface{}) {
	log.Error(fmt.Sprintf("[%s] %v", n.getID(), v))
}

func (n *MsgSignal) checkClose() bool {
	return n.isClosed
}

func (n *MsgSignal) setClose(state bool) {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	n.isClosed = state
}

// OK to send ok to peer
func (n *MsgSignal) OK(idTo string) {
	n.message(idTo, OkEvent, "Ready to peering")
}

// Candidate to send candidate for trickle ice
func (n *MsgSignal) Candidate(idTo string, icecandidate *webrtc.ICECandidate) {
	if icecandidate == nil {
		return
	}
	n.message(idTo, CandidateEvent, icecandidate.ToJSON())
}

// SDP send sdp back to client
func (n *MsgSignal) SDP(idTo string, sdp webrtc.SessionDescription) {
	n.message(idTo, SdpEvent, sdp)
}

// HandShakeError To send if handshake error
func (n *MsgSignal) HandShakeError(idTo string, reason string) {
	n.message(idTo, HandShakeErrorEvent, reason)
}

// ReconnectErr to send reconnect err to client
func (n *MsgSignal) ReconnectErr(idTo string, reason string) {
	n.message(idTo, ReconnectErrEvent, reason)
}

// ReconnectOk to send reconnect ok to client
func (n *MsgSignal) ReconnectOk(idTo string) {
	n.message(idTo, ReconnectOkEvent, "reconnect ok men !!!")
}

// Message to send msg to client
func (n *MsgSignal) Message(idTo string, msg string) {
	n.message(idTo, MessageEvent, msg)
}

// Send send any msg to signal
// ex: idTo, requestID, event, data
func (n *MsgSignal) Send(inputs ...interface{}) {
	n.push(SetValueToSignal(inputs...))
}

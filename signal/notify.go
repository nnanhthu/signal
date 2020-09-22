package signal

import (
	"fmt"
	"sync"

	log "github.com/lamhai1401/gologs/logs"
	"github.com/segmentio/ksuid"
)

// GenerateID to create new string id
func generateID() string {
	id := ksuid.New().String()
	return id
}

// GetNotifyURL to get msg signal
//func GetNotifyURL(id string) string {
//	if url := os.Getenv("MULTIPLE_URLL"); url != "" {
//		return fmt.Sprintf("%s/?id=%s", url, id)
//	}
//	return fmt.Sprintf("wss://signal-test.dechen.app/?id=%s", id)
//}

// NotifySignal handle signal of wertc (candidate, sdp)
type MsgSignal struct {
	id              string
	stompUrl        string
	wssUrl          string
	stompConn       *Signaler
	wssConn         *WssSignaler
	token           string
	publicChannel   string
	privateChannel  string
	timeout         int
	closeWssChannel chan int                // handle close
	sendMsgChannel  chan interface{}        // to save message
	processRecvData func(interface{}) error // handle incoming msg
	isClosed        bool                    // handle close signal
	mutex           sync.Mutex
}

// NewNotifySignal create new notify signal
func NewMsgSignal(id, stompUrl, wssUrl, token, publicChannel, privateChannel string, timeout int, processRecvData func(interface{}) error) *MsgSignal {
	return &MsgSignal{
		id:              fmt.Sprintf("Msg-Signal-%s", generateID()),
		stompUrl:        stompUrl,
		wssUrl:          wssUrl,
		token:           token,
		publicChannel:   publicChannel,
		privateChannel:  privateChannel,
		timeout:         timeout,
		isClosed:        false,
		sendMsgChannel:  make(chan interface{}, 1000),
		processRecvData: processRecvData,
	}
}

// Start to start signal
func (n *MsgSignal) Start() error {
	stompUrl := n.getStompURL()
	wssUrl := n.getWssURL()
	if len(wssUrl) > 0 {
		conn := NewWssSignaler(wssUrl, n.getProcessRecvData(), n.getID())
		if err := conn.Start(); err != nil {
			return err
		}
		n.setWssConn(conn)
	} else if len(stompUrl) > 0 {
		conn := NewSignaler(stompUrl, n.getProcessRecvData(), n.getToken(), n.getPublicChan(), n.getPrivateChan(), n.getTimeout())
		if err := conn.Start(); err != nil {
			return err
		}
		n.setStompConn(conn)
	}
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
	//if conn := n.getConn(); conn != nil && !n.checkClose() {
	//	conn.SendText(msg)
	//}
}

func (n *MsgSignal) getStompURL() string {
	return n.stompUrl
}

func (n *MsgSignal) getWssURL() string {
	return n.wssUrl
}

func (n *MsgSignal) getToken() string {
	return n.token
}

func (n *MsgSignal) getPublicChan() string {
	return n.publicChannel
}

func (n *MsgSignal) getPrivateChan() string {
	return n.privateChannel
}

func (n *MsgSignal) getTimeout() int {
	return n.timeout
}

// Close to close signal
func (n *MsgSignal) close() {
	n.setClose(true)
	n.closeConn()
	n.removeProcessRecvData()
}

func (n *MsgSignal) getProcessRecvData() func(interface{}) error {
	return n.processRecvData
}

func (n *MsgSignal) removeProcessRecvData() {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	n.processRecvData = nil
}

func (n *MsgSignal) getWssConn() *WssSignaler {
	return n.wssConn
}

func (n *MsgSignal) getStompConn() *Signaler {
	return n.stompConn
}

func (n *MsgSignal) setWssConn(conn *WssSignaler) {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	n.wssConn = conn
}

func (n *MsgSignal) setStompConn(conn *Signaler) {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	n.stompConn = conn
}

func (n *MsgSignal) removeWssConn() {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	n.wssConn = nil
}

func (n *MsgSignal) removeStompConn() {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	n.stompConn = nil
}

func (n *MsgSignal) closeWssConn() {
	if conn := n.getWssConn(); conn != nil {
		conn.close()
		conn = nil
	}
}

func (n *MsgSignal) closeStompConn() {
	if conn := n.getStompConn(); conn != nil {
		conn.close()
		conn = nil
	}
}

func (n *MsgSignal) closeConn() {
	n.closeWssConn()
	n.closeStompConn()
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

// Send send any msg to signal
// ex: idTo, requestID, event, data
//func (n *MsgSignal) Send(method, dest string, inputs interface{}) (interface{}, int, error) {
//	stompUrl := n.getStompURL()
//	wssUrl := n.getWssURL()
//	if len(wssUrl) > 0 {
//		//Send through wss node js
//		if conn := n.getWssConn(); conn != nil {
//			conn.SendText(inputs)
//		}
//	} else if len(stompUrl) > 0 {
//		//Send through stomp
//		if conn := n.getStompConn(); conn != nil {
//			conn.SendAPI(method, dest, inputs)
//		}
//	}
//	return nil
//	//n.push(SetValueToSignal(inputs...))
//}

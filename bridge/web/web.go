package bweb

import (
	"encoding/json"
	"fmt"
	"github.com/42wim/matterbridge/bridge/config"
	log "github.com/Sirupsen/logrus"
	"github.com/gorilla/websocket"
	"net/http"
	"time"
)

type Bweb struct {
	BindAddress string
	Account     string
	Messages    chan config.Message
	Remote      chan config.Message
}

const (
	// Time allowed to write the file to the client.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the client.
	pongWait = 60 * time.Second

	// Send pings to client with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10
)

var (
	flog     *log.Entry
	upgrader = websocket.Upgrader{}
)

func init() {
	flog = log.WithFields(log.Fields{"module": "web"})
}

func New(cfg config.Protocol, account string, c chan config.Message) *Bweb {
	b := &Bweb{}
	b.BindAddress = cfg.BindAddress
	b.Messages = make(chan config.Message)
	b.Account = account
	b.Remote = c
	flog.Infof("Creating new websocket server %s", cfg.BindAddress)
	return b
}

func (b *Bweb) Connect() error {
	go b.Listen()
	return nil
}

func (b *Bweb) JoinChannel(s string) error {
	return nil
}

func (b *Bweb) Send(msg config.Message) error {
	b.Messages <- msg
	return nil
}

func (b *Bweb) ReqWrite(ws *websocket.Conn) {
	pingTicker := time.NewTicker(pingPeriod)
	defer func() {
		pingTicker.Stop()
		ws.Close()
	}()
	for {
		select {
		case msg := <-b.Messages:
			err := ws.WriteJSON(msg)
			if err != nil {
				return
			}
		case <-pingTicker.C:
			ws.SetWriteDeadline(time.Now().Add(writeWait))
			if err := ws.WriteMessage(websocket.PingMessage, []byte{}); err != nil {
				return
			}
		}
	}
}

func (b *Bweb) ReqRead(ws *websocket.Conn) {
	defer ws.Close()
	ws.SetReadLimit(512)
	ws.SetReadDeadline(time.Now().Add(pongWait))
	ws.SetPongHandler(func(string) error {
		ws.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	for {
		_, jsonMsg, err := ws.ReadMessage()
		if err != nil {
			break
		}
		msg := &config.Message{}
		err = json.Unmarshal(jsonMsg, msg)
		if err != nil {
			ws.WriteMessage(
				websocket.TextMessage,
				[]byte(fmt.Sprintf("Failed to parse JSON: %s", err)))
			continue
		}
		msg.Account = b.Account
		msg.Username = "" // Empty for now
		b.Remote <- *msg
	}
}

func (b *Bweb) HandleRequest(res http.ResponseWriter, req *http.Request) {
	ws, err := upgrader.Upgrade(res, req, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}
	defer ws.Close()

	go b.ReqWrite(ws)
	b.ReqRead(ws)
}

func (b *Bweb) Listen() error {
	http.HandleFunc("/", b.HandleRequest)
	flog.Printf("Starting websocket server on %s", b.BindAddress)
	return http.ListenAndServe(b.BindAddress, nil)
}

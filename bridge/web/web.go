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
	Users       chan config.User
	Channels    chan config.Channel
	Remote      chan config.Message
	Commands    chan string
	count       int
}

const (
	// Time allowed to write the file to the client.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the client.
	pongWait = 60 * time.Second

	// Send pings to client with this period. Must be less than pongWait.
	pingPeriod = pongWait * 9 / 10
)

var (
	flog     *log.Entry
	upgrader = websocket.Upgrader{}
)

func init() {
	flog = log.WithFields(log.Fields{"module": "web"})
}

func New(cfg config.Protocol, account string, c config.Comms) *Bweb {
	b := &Bweb{}
	b.BindAddress = cfg.BindAddress
	b.Messages = make(chan config.Message)
	b.Users = make(chan config.User)
	b.Channels = make(chan config.Channel)

	b.Account = account
	b.Remote = c.Messages
	b.Commands = c.Commands
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
	go func() {
		b.Messages <- msg
	}()
	return nil
}

func (b *Bweb) Presence(user config.User) error {
	go func() {
		b.Users <- user
	}()
	return nil
}

func (b *Bweb) Discovery(channel config.Channel) error {
	go func() {
		b.Channels <- channel
	}()
	return nil
}

func (b *Bweb) ReqWrite(ws *websocket.Conn) {
	pingTicker := time.NewTicker(pingPeriod)
	defer func() {
		flog.Printf("Write closing")
		pingTicker.Stop()
		ws.Close()
		flog.Printf("Closed")
	}()
	flog.Printf("WriteReq running %d", b.count)
	for {
		select {
		case msg := <-b.Messages:
			err := ws.WriteJSON(msg)
			if err != nil {
				return
			}
		case msg := <-b.Users:
			err := ws.WriteJSON(msg)
			if err != nil {
				return
			}
		case msg := <-b.Channels:
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
	defer func() {
		flog.Printf("Read closing")
		ws.Close()
	}()
	ws.SetReadDeadline(time.Now().Add(pongWait))
	flog.Printf("ReqRead running")
	ws.SetPongHandler(func(string) error {
		ws.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	for {
		flog.Printf("waiting for message")
		_, jsonMsg, err := ws.ReadMessage()
		flog.Printf("message received %s", jsonMsg)
		if err != nil {
			flog.Printf("failed to read message %s", err)
			return
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
	flog.Printf("Request start")
	ws, err := upgrader.Upgrade(res, req, nil)
	if err != nil {
		flog.Print("upgrade err:", err)
		return
	}
	b.count = b.count + 1

	b.Commands <- "get messages"

	go b.ReqWrite(ws)
	b.ReqRead(ws)
	flog.Printf("Request end")
}

func (b *Bweb) Listen() error {
	http.HandleFunc("/ws", b.HandleRequest)
	http.Handle("/", http.FileServer(http.Dir("web/dist")))
	flog.Printf("Starting web server on %s", b.BindAddress)
	return http.ListenAndServe("127.0.0.1:8001", nil)
}

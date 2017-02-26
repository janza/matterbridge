package bweb

import (
	"encoding/json"
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
	Hub         *Hub
	Commands    chan config.Command
	count       int
}

type WireMessage struct {
	Type    string
	Message config.Message
	User    config.User
	Channel config.Channel
}

type InboundWireMessage struct {
	Type    string
	Message interface{}
}

const (
	// Time allowed to write the file to the client.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the client.
	pongWait = 60 * time.Second

	// Send pings to client with this period. Must be less than pongWait.
	pingPeriod = pongWait * 9 / 10

	// Maximum message size allowed from peer.
	maxMessageSize = 512
)

var (
	newline  = []byte{'\n'}
	flog     *log.Entry
	upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
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

func (b *Bweb) serveWs(hub *Hub, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}
	client := &Client{
		hub:            hub,
		conn:           conn,
		account:        b.Account,
		remoteMessages: b.Remote,
		remoteCommands: b.Commands,
		send:           make(chan []byte, 256),
	}
	client.hub.register <- client
	go client.writePump()
	client.readPump()
}

func (b *Bweb) Run() {
	for {
		select {
		case msg := <-b.Messages:
			json, err := json.Marshal(WireMessage{
				Type:    "message",
				Message: msg,
			})
			if err != nil {
				panic(err)
			}
			b.Hub.broadcast <- json
		case msg := <-b.Users:
			json, err := json.Marshal(WireMessage{
				Type: "user",
				User: msg,
			})
			if err != nil {
				panic(err)
			}
			b.Hub.broadcast <- json
		case msg := <-b.Channels:
			json, err := json.Marshal(WireMessage{
				Type:    "channel",
				Channel: msg,
			})
			if err != nil {
				panic(err)
			}
			b.Hub.broadcast <- json
		}
	}
}

func (b *Bweb) Listen() error {
	b.Hub = newHub()
	go b.Hub.run()
	go b.Run()
	http.Handle("/", http.FileServer(http.Dir("web/dist")))
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		b.serveWs(b.Hub, w, r)
	})
	err := http.ListenAndServe("127.0.0.1:8001", nil)
	return err
}

// Client is a middleman between the websocket connection and the hub.
type Client struct {
	hub *Hub

	// The websocket connection.
	conn *websocket.Conn

	account string

	remoteMessages chan config.Message
	remoteCommands chan config.Command

	// Buffered channel of outbound messages.
	send chan []byte
}

func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()
	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	for {
		_, jsonMsg, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway) {
				log.Printf("error: %v", err)
			}
			break
		}
		var msg json.RawMessage
		inbound := InboundWireMessage{Message: &msg}
		err = json.Unmarshal(jsonMsg, &inbound)
		if err != nil {
			log.Printf("unable to parse json: %s", jsonMsg)
			continue
		}
		switch inbound.Type {
		case "message":
			var incomingMessage config.Message
			if err := json.Unmarshal(msg, &incomingMessage); err != nil {
				log.Fatal(err)
				continue
			}
			incomingMessage.Account = c.account
			incomingMessage.Username = "" // Empty for now
			c.remoteMessages <- incomingMessage
		case "command":
			var specificCommand json.RawMessage
			cmd := config.Command{
				Command: &specificCommand,
			}
			if err := json.Unmarshal(msg, &cmd); err != nil {
				log.Fatal(err)
				continue
			}
			switch cmd.Type {
			case "replay_messages":
				var command config.GetMessagesCommand
				err := json.Unmarshal(specificCommand, &command)
				if err != nil {
					log.Fatal(err)
					continue
				}
				cmd.Command = command
			case "get_channels":
				var command config.GetChannelsCommand
				err := json.Unmarshal(specificCommand, &command)
				if err != nil {
					log.Fatal(err)
					continue
				}
				cmd.Command = command
			case "get_users":
				var command config.GetUsersCommand
				err := json.Unmarshal(specificCommand, &command)
				if err != nil {
					log.Fatal(err)
					continue
				}
				cmd.Command = command
			}
			c.remoteCommands <- cmd
		}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// The hub closed the channel.
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)
			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, []byte{}); err != nil {
				return
			}
		}
	}
}

// hub maintains the set of active clients and broadcasts messages to the
// clients.
type Hub struct {
	// Registered clients.
	clients map[*Client]bool

	// Inbound messages from the clients.
	broadcast chan []byte

	// Register requests from the clients.
	register chan *Client

	// Unregister requests from clients.
	unregister chan *Client
}

func newHub() *Hub {
	return &Hub{
		broadcast:  make(chan []byte),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		clients:    make(map[*Client]bool),
	}
}

func (h *Hub) run() {

	for {
		select {
		case client := <-h.register:
			h.clients[client] = true
		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
		case message := <-h.broadcast:
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
		}
	}
}

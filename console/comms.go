package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/42wim/matterbridge/bridge/config"
	"github.com/42wim/matterbridge/bridge/web"
	"github.com/gorilla/websocket"
)

var (
	url = "wss://chat.jjanzic.com/ws"
)

type msgHandler func(config.Message) bool
type userHandler func(config.User)
type channelHandler func(config.Channel)
type readMessageHandler func(config.Message)

// Conn handles websocket connections
type Conn struct {
	done              chan struct{}
	commands          chan config.Command
	messages          chan config.Message
	debouncedCommands chan config.Command
	newMessage        msgHandler
	newUser           userHandler
	newChannel        channelHandler
	readMessage       readMessageHandler
}

// NewConnection created a connection instance
func NewConnection(done chan struct{}, m msgHandler, u userHandler, c channelHandler, r readMessageHandler) *Conn {
	comms := &Conn{}
	comms.done = done
	comms.messages = make(chan config.Message)
	comms.commands = make(chan config.Command)
	comms.debouncedCommands = comms.debounceCommandChannel(3*time.Second, comms.commands)
	comms.newMessage = m
	comms.newUser = u
	comms.newChannel = c
	comms.readMessage = r
	return comms
}

func (c *Conn) debounceCommandChannel(
	interval time.Duration,
	output chan config.Command,
) chan config.Command {
	input := make(chan config.Command)

	go func() {
		var buffer config.Command
		var ok bool

		buffer, ok = <-input
		if !ok {
			return
		}

		for {
			select {
			case buffer, ok = <-input:
				if !ok {
					return
				}

			case <-time.After(interval):
				output <- buffer
				buffer, ok = <-input
				if !ok {
					return
				}
			}
		}
	}()

	return input
}

func (c *Conn) WebsocketConnect() error {

	var dialer *websocket.Dialer

	h := http.Header{
		"Authorization": {"Basic bWFyYTptYXJh"},
	}

	conn, _, err := dialer.Dial(url, h)
	if err != nil {
		log.Printf("Error dialing socket %s", err)
		return err
	}

	conn.SetReadLimit(0)

	defer func() {
		conn.Close()
		log.Println("Closing from reader")
	}()

	go c.wsWriter(conn)

	c.sendInitialCommands()

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Printf("ReadMessage error %s %s", err.Error(), string(websocket.CloseMessageTooBig))
			return err
		}

		c.handleWebsocketMessage(message)
	}
}

func (c *Conn) handleWebsocketMessage(message []byte) {
	wireMsg := bweb.WireMessage{}
	err := json.Unmarshal(message, &wireMsg)
	if err != nil {
		return
	}

	if wireMsg.Type == "user" {
		c.newUser(wireMsg.User)
	}

	if wireMsg.Type == "channel" {
		c.newChannel(wireMsg.Channel)
	}

	if wireMsg.Type == "message" {
		if c.newMessage(wireMsg.Message) {
			c.MarkAsRead(wireMsg.Message)
		}
	}

	if wireMsg.Type == "read_status" {
		log.Printf("Got socket message %s\n", wireMsg.Type)
		incomingMsg := wireMsg.Message
		c.commands <- config.Command{
			Type: "replay_messages",
			Command: config.GetMessagesCommand{
				Channel: incomingMsg.Channel + ":" + incomingMsg.Account,
				Offset:  incomingMsg.Timestamp,
			},
		}
		c.readMessage(incomingMsg)
	}
}

func (c *Conn) sendInitialCommands() {
	c.commands <- config.Command{
		Type:    "get_channels",
		Command: config.GetChannelsCommand{},
	}

	c.commands <- config.Command{
		Type:    "get_users",
		Command: config.GetUsersCommand{},
	}

	c.commands <- config.Command{
		Type:    "get_last_read_messages",
		Command: config.GetLastReadMessages{},
	}
}

func (c *Conn) fetchMessages(channel config.Channel, since time.Time) {
	getMessages := config.GetMessagesCommand{
		Channel: channel.ID,
		Offset:  since,
	}
	c.commands <- config.Command{
		Type:    "replay_messages",
		Command: getMessages,
	}
}

func (c *Conn) MarkAsRead(msg config.Message) {
	c.debouncedCommands <- config.Command{
		Type: "mark_message_as_read",
		Command: config.MarkMessageAsRead{
			Message: msg,
		},
	}
}

func (c *Conn) wsWriter(conn *websocket.Conn) {
	defer func() {
		log.Println("Closing from writer")
		conn.Close()
	}()
	for {
		select {
		case message := <-c.messages:
			jsonMessage, _ := json.Marshal(bweb.InboundWireMessage{
				Type:    "message",
				Message: message,
			})
			log.Printf("Sending message to socket %s\n", message.Text)
			conn.WriteMessage(
				websocket.TextMessage,
				jsonMessage,
			)
		case command := <-c.commands:
			jsonMessage, _ := json.Marshal(bweb.InboundWireMessage{
				Type:    "command",
				Message: command,
			})
			log.Printf("Sending command to socket %s\n", jsonMessage)
			conn.WriteMessage(
				websocket.TextMessage,
				jsonMessage,
			)
		case <-c.done:
			return
		}
	}
}

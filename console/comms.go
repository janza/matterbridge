package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/42wim/matterbridge/bridge/config"
	"github.com/42wim/matterbridge/bridge/web"
	"github.com/gorilla/websocket"
)

var (
	url = "ws://localhost:8001/ws"
)

type msgHandler func(config.Message)
type userHandler func(config.User)
type channelHandler func(config.Channel)

// Conn handles websocket connections
type Conn struct {
	done       chan struct{}
	commands   chan config.Command
	messages   chan config.Message
	newMessage msgHandler
	newUser    userHandler
	newChannel channelHandler
}

// NewConnection created a connection instance
func NewConnection(done chan struct{}, m msgHandler, u userHandler, c channelHandler) *Conn {
	comms := &Conn{}
	comms.done = done
	comms.messages = make(chan config.Message)
	comms.commands = make(chan config.Command)
	comms.newMessage = m
	comms.newUser = u
	comms.newChannel = c
	return comms
}

func (c *Conn) WebsocketConnect() {

	var dialer *websocket.Dialer

	conn, _, err := dialer.Dial(url, nil)
	if err != nil {
		fmt.Println(err)
		return
	}

	defer conn.Close()

	go c.wsWriter(conn)

	c.commands <- config.Command{
		Type:    "get_channels",
		Command: config.GetChannelsCommand{},
	}

	c.commands <- config.Command{
		Type:    "get_users",
		Command: config.GetUsersCommand{},
	}

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			fmt.Println("read:", err)
			return
		}

		msg := bweb.WireMessage{}

		err = json.Unmarshal(message, &msg)
		if err != nil {
			fmt.Println("failed to parse json:", err)
			return
		}

		if msg.Type == "user" {
			c.newUser(msg.User)
		}

		if msg.Type == "channel" {
			c.newChannel(msg.Channel)
		}

		if msg.Type == "message" {
			c.newMessage(msg.Message)
		}
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

func (c *Conn) wsWriter(conn *websocket.Conn) {
	defer func() {
		conn.Close()
	}()
	for {
		select {
		case message := <-c.messages:
			fmt.Printf("Message: %#v", message)

			jsonMessage, _ := json.Marshal(bweb.InboundWireMessage{
				Type:    "message",
				Message: message,
			})
			conn.WriteMessage(
				websocket.TextMessage,
				jsonMessage,
			)
		case command := <-c.commands:
			jsonMessage, _ := json.Marshal(bweb.InboundWireMessage{
				Type:    "command",
				Message: command,
			})
			conn.WriteMessage(
				websocket.TextMessage,
				jsonMessage,
			)
		case <-c.done:
			return
		}
	}
}

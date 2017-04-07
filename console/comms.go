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
				fmt.Println("adding to buffer")
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

func (c *Conn) WebsocketConnect() {

	var dialer *websocket.Dialer

	conn, _, err := dialer.Dial(url, nil)
	if err != nil {
		fmt.Println(err)
		return
	}

	defer conn.Close()

	go c.wsWriter(conn)

	c.sendInitialCommands()

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			fmt.Println("read:", err)
			return
		}

		c.handleWebsocketMessage(message)
	}
}

func (c *Conn) handleWebsocketMessage(message []byte) {
	msg := bweb.WireMessage{}
	err := json.Unmarshal(message, &msg)
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
		if c.newMessage(msg.Message) {
			c.MarkAsRead(msg.Message)
		}
	}

	if msg.Type == "read_status" {
		incomingMsg := msg.Message
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
		conn.Close()
	}()
	for {
		select {
		case message := <-c.messages:
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

// Copyright 2014 The gocui Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"sort"
	"sync"
	"time"

	"github.com/42wim/matterbridge/bridge/config"
	"github.com/42wim/matterbridge/bridge/web"
	"github.com/gorilla/websocket"
	"github.com/jroimartin/gocui"
)

var (
	done = make(chan struct{})
	wg   sync.WaitGroup

	grayColor  = "\033[0;37m"
	whiteColor = "\033[1;37m"
	redColor   = "\033[0;31m"
	blueColor  = "\033[1;32m"
	resetColor = "\033[0m"
)

type key struct {
	command string
}

func random(min, max int) int {
	rand.Seed(time.Now().Unix())
	return rand.Intn(max-min) + min
}

func insertMessage(s []config.Message, f config.Message) []config.Message {
	l := len(s)
	if l == 0 {
		return []config.Message{f}
	}

	i := sort.Search(l, func(i int) bool {
		return s[i].Timestamp.After(f.Timestamp)
	})
	s = append(s, f)
	copy(s[i+1:], s[i:])
	s[i] = f
	return s
}

func main() {
	g, err := gocui.NewGui(gocui.OutputNormal)
	if err != nil {
		log.Panicln(err)
	}
	defer g.Close()

	g.SetManagerFunc(layout)

	ws := &comms{
		messages: make(chan config.Message),
		commands: make(chan config.Command),
		storage: storage{
			messages: make(map[string][]config.Message),
		},
	}

	if err := keybindings(g, ws); err != nil {
		log.Panicln(err)
	}

	wg.Add(1)
	go ws.websocketConnect(g)

	if err := g.MainLoop(); err != nil && err != gocui.ErrQuit {
		log.Panicln(err)
	}

	wg.Wait()
}

func layout(g *gocui.Gui) error {
	maxX, maxY := g.Size()
	if v, err := g.SetView("chan", -1, -1, 12, maxY-1); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Frame = false
		fmt.Fprintln(v, "Channels")
	}
	if v, err := g.SetView("msgs", 13, -1, maxX, maxY-1); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Autoscroll = true
		v.Frame = false
		fmt.Fprintln(v, "Msgs")
	}
	if v, err := g.SetView("input", -1, maxY-2, maxX, maxY); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		if _, err := g.SetCurrentView("input"); err != nil {
			return err
		}
		fmt.Fprintf(v, blueColor)
		v.FgColor = gocui.ColorYellow
		v.Editable = true
		v.Wrap = true
		v.Frame = false
	}
	return nil
}

func keybindings(g *gocui.Gui, c *comms) error {
	if err := g.SetKeybinding("", gocui.KeyCtrlC, gocui.ModNone, quit); err != nil {
		return err
	}

	if err := g.SetKeybinding("", gocui.KeyCtrlJ, gocui.ModNone, c.selectNextChan); err != nil {
		return err
	}

	if err := g.SetKeybinding("", gocui.KeyCtrlK, gocui.ModNone, c.selectPrevChan); err != nil {
		return err
	}
	return nil
}

func quit(g *gocui.Gui, v *gocui.View) error {
	close(done)
	return gocui.ErrQuit
}

type channelSlice []config.Channel

func (slice channelSlice) pos(value config.Channel) int {
	for p, v := range slice {
		if v == value {
			return p
		}
	}
	return -1
}

type storage struct {
	activeChannel config.Channel
	channels      channelSlice
	users         []config.User
	messages      map[string][]config.Message
}

type comms struct {
	messages chan config.Message
	commands chan config.Command
	storage  storage
}

func findInSlice(slice []interface{}, itemToFind interface{}) int {
	for i, item := range slice {
		if item == itemToFind {
			return i
		}
	}
	return -1
}

func (c *comms) selectNextChan(g *gocui.Gui, v *gocui.View) error {
	l := len(c.storage.channels)
	pos := c.storage.channels.pos(c.storage.activeChannel)
	c.storage.activeChannel = c.storage.channels[(pos+1)%l]
	c.fetchMessages()
	c.redraw(g)
	return nil
}

func (c *comms) selectPrevChan(g *gocui.Gui, v *gocui.View) error {
	l := len(c.storage.channels)
	pos := c.storage.channels.pos(c.storage.activeChannel)
	c.storage.activeChannel = c.storage.channels[(pos+l-1)%l]
	c.fetchMessages()
	c.redraw(g)
	return nil
}

func redrawChannels(g *gocui.Gui, channels channelSlice, activeChannel config.Channel) {
	vChan, _ := g.View("chan")
	vChan.Clear()
	for _, channel := range channels {
		c1, c2 := "", ""
		if channel == activeChannel {
			c1 = redColor
			c2 = resetColor
		}
		fmt.Fprintf(vChan, "%s%s%s\n", c1, channel.Name, c2)
	}
}

func (c *comms) fetchMessages() {
	channelMsgs := c.storage.messages[c.storage.activeChannel.ID]
	getMessages := config.GetMessagesCommand{
		Channel: c.storage.activeChannel.ID,
	}
	if len(channelMsgs) > 0 {
		getMessages.Offset = channelMsgs[0].Timestamp
	}
	c.commands <- config.Command{
		Type:    "replay_messages",
		Command: getMessages,
	}
}

func (c *comms) getUser(account, userID string) config.User {
	for _, user := range c.storage.users {
		if user.User == userID {
			return user
		}
	}
	return config.User{Name: userID}
}

func formatTime(t time.Time) string {
	return t.Format("15:04")
}

func (c *comms) redraw(g *gocui.Gui) error {

	vMsg, err := g.View("msgs")
	if err != nil {
		return err
	}
	vMsg.Clear()
	fmt.Fprintln(vMsg, c.storage.activeChannel.Name)

	activeChannelMsgs := c.storage.messages[c.storage.activeChannel.ID]

	// _, y := vMsg.Size()
	l := len(activeChannelMsgs)
	for i := 0; i < l; i++ {
		msg := activeChannelMsgs[i]
		fmt.Fprintf(
			vMsg,
			"%s%s %s%s: %s%s%s\n",
			grayColor,
			formatTime(msg.Timestamp),
			blueColor,
			c.getUser(msg.Account, msg.Username).Name,
			whiteColor,
			msg.Text,
			resetColor,
		)
	}

	redrawChannels(g, c.storage.channels, c.storage.activeChannel)

	if _, err := g.SetCurrentView("input"); err != nil {
		return err
	}

	return nil
}

func (c *comms) websocketConnect(g *gocui.Gui) {
	URL := "ws://localhost:8001/ws"

	var dialer *websocket.Dialer

	conn, _, err := dialer.Dial(URL, nil)
	if err != nil {
		fmt.Println(err)
		return
	}

	defer func() {
		conn.Close()
		wg.Add(-1)
	}()

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
			c.storage.users = append(c.storage.users, msg.User)
		}

		if msg.Type == "channel" {
			c.storage.channels = append(c.storage.channels, msg.Channel)
			nullChan := config.Channel{}
			if c.storage.activeChannel == nullChan {
				c.storage.activeChannel = msg.Channel
			}
		}

		if msg.Type == "message" {
			bucket := msg.Message.Channel + ":" + msg.Message.Account
			c.storage.messages[bucket] = insertMessage(c.storage.messages[bucket], msg.Message)
		}

		g.Execute(func(g *gocui.Gui) error {
			return c.redraw(g)
		})
	}
}

func (c *comms) wsWriter(conn *websocket.Conn) {
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
		case <-done:
			return
		}
	}
}

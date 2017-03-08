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
	"github.com/fatih/color"
	"github.com/gorilla/websocket"
	"github.com/jroimartin/gocui"
	"github.com/kennygrant/sanitize"
	"github.com/plar/go-adaptive-radix-tree"
)

var (
	done = make(chan struct{})
	wg   sync.WaitGroup
	mut  = &sync.Mutex{}

	grayColor = color.New(color.FgHiGreen).SprintFunc()
	redColor  = color.New(color.FgRed).SprintFunc()
	blueColor = color.New(color.FgBlue).SprintFunc()
)

type key struct {
	command string
}

func whiteColor(s string) string {
	return fmt.Sprintf("\x1b[38;5;%dm%s\x1b[0m", 256, s)
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
	g, err := gocui.NewGui(gocui.Output256)
	if err != nil {
		log.Panicln(err)
	}
	defer g.Close()

	g.SetManagerFunc(layout)

	ws := &comms{
		messages: make(chan config.Message),
		commands: make(chan config.Command),
		storage: storage{
			messages:       make(map[string]art.Tree),
			unreadMessages: make(messagesInChannel),
			totalMessages:  make(messagesInChannel),
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
	channelsWidth := 20
	if v, err := g.SetView("chan", -1, 0, channelsWidth, maxY-1); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Frame = false
		fmt.Fprintln(v, "Channels")
	}
	if v, err := g.SetView("msgs", channelsWidth+1, 0, maxX, maxY-1); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Autoscroll = true
		v.Frame = false
		v.Wrap = true
		fmt.Fprintln(v, "Msgs")
	}
	if v, err := g.SetView("input", -1, maxY-2, maxX, maxY); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		if _, err := g.SetCurrentView("input"); err != nil {
			return err
		}
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

func (slice channelSlice) Sort(unreadMessages messagesInChannel, totalMessages messagesInChannel) {
	cs := &channelSorter{
		channelSlice:   slice,
		unreadMessages: unreadMessages,
		totalMessages:  totalMessages,
	}

	sort.Sort(cs)
}

type messagesInChannel map[string]int

type channelSorter struct {
	channelSlice   channelSlice
	unreadMessages messagesInChannel
	totalMessages  messagesInChannel
}

func (c *channelSorter) Len() int { return len(c.channelSlice) }
func (c *channelSorter) Swap(i, j int) {
	c.channelSlice[i], c.channelSlice[j] = c.channelSlice[j], c.channelSlice[i]
}
func (c *channelSorter) Less(i, j int) bool {
	u1 := c.unreadMessages[c.channelSlice[i].ID]
	u2 := c.unreadMessages[c.channelSlice[j].ID]
	if u1 == u2 {
		return c.totalMessages[c.channelSlice[i].ID] > c.totalMessages[c.channelSlice[j].ID]
	}
	return u1 > u2
}

type storage struct {
	activeChannel  config.Channel
	channels       channelSlice
	unreadMessages messagesInChannel
	totalMessages  messagesInChannel
	users          []config.User
	messages       map[string]art.Tree
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
	c.storage.unreadMessages[c.storage.activeChannel.ID] = 0
	c.fetchMessages()
	c.redraw(g)
	return nil
}

func (c *comms) selectPrevChan(g *gocui.Gui, v *gocui.View) error {
	l := len(c.storage.channels)
	pos := c.storage.channels.pos(c.storage.activeChannel)
	c.storage.activeChannel = c.storage.channels[(pos+l-1)%l]
	c.storage.unreadMessages[c.storage.activeChannel.ID] = 0
	c.fetchMessages()
	c.redraw(g)
	return nil
}

func redrawChannels(g *gocui.Gui, channels channelSlice, activeChannel config.Channel, unreadMessages messagesInChannel) {
	vChan, _ := g.View("chan")
	vChan.Clear()
	for _, channel := range channels {
		if unreadMessages[channel.ID] != 0 {
			fmt.Fprintf(vChan, "(%d) ", unreadMessages[channel.ID])
		}
		format := "%s\n"
		if channel == activeChannel {
			fmt.Fprintf(vChan, format, redColor(channel.Name))
		} else {
			fmt.Fprintf(vChan, format, channel.Name)
		}
	}
}

func (c *comms) fetchMessages() {
	getMessages := config.GetMessagesCommand{
		Channel: c.storage.activeChannel.ID,
	}
	if channelMsgs, ok := c.storage.messages[c.storage.activeChannel.ID]; ok {
		mut.Lock()
		msgs := channelMsgs.Iterator()
		if msgs.HasNext() {
			firstMsg, _ := msgs.Next()
			msg, _ := firstMsg.Value().(config.Message)
			getMessages.Offset = msg.Timestamp
		}
		mut.Unlock()
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

func colorize(s string) string {
	sum := 0
	for _, c := range s {
		sum = (sum + int(c)) % 256
	}
	return fmt.Sprintf("\x1b[38;5;%dm%s\x1b[0m", sum, s)
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

	if activeChannelMsgs, ok := c.storage.messages[c.storage.activeChannel.ID]; ok {
		mut.Lock()
		for it := activeChannelMsgs.Iterator(); it.HasNext(); {
			node, err := it.Next()
			if err != nil || node.Kind() != art.Leaf {
				panic(err)
			}
			value := node.Value()
			msg, _ := value.(config.Message)
			userName := c.getUser(msg.Account, msg.Username).Name
			fmt.Fprintf(
				vMsg,
				"%s %s: %s\n",
				grayColor(formatTime(msg.Timestamp)),
				colorize(userName),
				whiteColor(sanitize.HTML(msg.Text)),
			)
		}
		mut.Unlock()
	}

	mut.Lock()
	redrawChannels(g, c.storage.channels, c.storage.activeChannel, c.storage.unreadMessages)
	mut.Unlock()

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
			if _, ok := c.storage.messages[bucket]; !ok {
				c.storage.messages[bucket] = art.New()
			}
			mut.Lock()
			c.storage.messages[bucket].Insert(art.Key(msg.Message.GetKey()), msg.Message)
			c.storage.totalMessages[bucket]++
			if c.storage.activeChannel.ID != bucket {
				c.storage.unreadMessages[bucket]++
			}
			c.storage.channels.Sort(c.storage.unreadMessages, c.storage.totalMessages)
			mut.Unlock()
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

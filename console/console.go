package main

import (
	"fmt"
	// "log"
	// "math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/42wim/matterbridge/bridge/config"
	// "github.com/42wim/matterbridge/console"
	"github.com/fatih/color"
	"github.com/jroimartin/gocui"
	"github.com/kennygrant/sanitize"
	"github.com/kr/text"
	"github.com/plar/go-adaptive-radix-tree"
)

var (
	done = make(chan struct{})
	wg   sync.WaitGroup

	grayColor        = color.New(color.FgHiGreen).SprintFunc()
	redColor         = color.New(color.FgRed).Add(color.Underline).SprintFunc()
	highlightChannel = color.New(color.Underline).SprintFunc()
	yellowColor      = color.New(color.FgYellow).Add(color.Underline).SprintFunc()
	blueColor        = color.New(color.FgBlue).SprintFunc()
)

type key struct {
	command string
}

func whiteColor(s string) string {
	return fmt.Sprintf("\x1b[38;5;%dm%s\x1b[0m", 256, s)
}

// func random(min, max int) int {
// 	rand.Seed(time.Now().Unix())
// 	return rand.Intn(max-min) + min
// }

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
		panic(err)
	}
	defer func() {
		g.Close()
	}()

	storage := NewStorage(func() {
		g.Execute(func(g *gocui.Gui) error {
			return nil
		})
	})

	connection := NewConnection(done, storage.NewMessage, storage.NewUser, storage.NewChannel)

	messages := NewMessagesWidget("messages", 0, 0, storage)
	channels := NewChannelsWidget("channels", 0, 0, storage, connection)
	g.SetManager(messages, channels)

	if err := keybindings(g); err != nil {
		panic(err)
	}

	go func() {
		connection.WebsocketConnect()
		defer func() {
			g.Close()
		}()
	}()

	if err := g.MainLoop(); err != nil && err != gocui.ErrQuit {
		fmt.Println(err)
	}
}

// func (c *comms) redrawChannelSelector(v *gocui.View, selectionIndex int) {
// 	c.storage.filteredChannels = filterChannels(c.storage.channels, func(channel config.Channel) bool {
// 		return strings.Contains(channel.ID, c.storage.channelFilter)
// 	})
// 	c.storage.filteredChannels.Sort()

// 	v.Clear()
// 	for i, channel := range c.storage.filteredChannels {
// 		format := "%s\n"
// 		output := strings.Replace(
// 			channel.ID,
// 			c.storage.channelFilter,
// 			redColor(c.storage.channelFilter), 1)
// 		if i == selectionIndex {
// 			output = yellowColor(output)
// 		}
// 		fmt.Fprintf(v, format, output)
// 	}
// }

// func (c *comms) channelEditor(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) {
// 	switch {
// 	case ch != 0 && mod == 0:
// 		c.storage.channelFilter += string(ch)
// 	case key == gocui.KeySpace:
// 		c.storage.channelFilter += " "
// 	case key == gocui.KeyCtrlJ:
// 		c.storage.channelIndex++
// 	case key == gocui.KeyCtrlK:
// 		c.storage.channelIndex--
// 	case key == gocui.KeyBackspace || key == gocui.KeyBackspace2:
// 		if len(c.storage.channelFilter) > 0 {
// 			c.storage.channelFilter = c.storage.channelFilter[:len(c.storage.channelFilter)-1]
// 		}
// 	case key == gocui.KeyEnter:
// 		nChannels := len(c.storage.filteredChannels)
// 		c.storage.channelIndex = (c.storage.channelIndex + nChannels) % nChannels
// 		if nChannels > 0 {
// 			c.setActiveChannel(c.storage.filteredChannels[c.storage.channelIndex])
// 		}
// 		c.g.SetViewOnTop("layout")
// 		c.g.SetViewOnTop("chan")
// 		c.g.SetViewOnTop("msgs")
// 		c.g.SetCurrentView("input")
// 		c.g.SetViewOnTop("input")
// 		c.g.SetViewOnTop("active_channel")
// 		c.redraw(c.g)
// 		return
// 	}
// 	nChannels := len(c.storage.filteredChannels)
// 	if nChannels > 0 {
// 		c.storage.channelIndex = (c.storage.channelIndex + nChannels) % nChannels
// 	}
// 	c.redrawChannelSelector(v, c.storage.channelIndex)
// }

// func (c *comms) layout(g *gocui.Gui) error {
// 	maxX, maxY := g.Size()
// 	channelsWidth := 20
// 	activeChannelHeight := 1
// 	if v, err := g.SetView("chan_selector", -1, -1, maxX, maxY); err != nil {
// 		if err != gocui.ErrUnknownView {
// 			return err
// 		}
// 		v.Frame = false
// 		v.Editable = true
// 		v.Wrap = true
// 		v.Editor = gocui.EditorFunc(c.channelEditor)
// 	}
// 	if v, err := g.SetView("active_channel", -1, -1, maxX, activeChannelHeight); err != nil {
// 		if err != gocui.ErrUnknownView {
// 			return err
// 		}
// 		fmt.Fprintln(v, "Active Channel")
// 	}
// 	if v, err := g.SetView("chan", -1, activeChannelHeight, channelsWidth, maxY-1); err != nil {
// 		if err != gocui.ErrUnknownView {
// 			return err
// 		}
// 		v.Frame = false
// 	}
// 	if v, err := g.SetView("msgs", channelsWidth+1, activeChannelHeight, maxX, maxY-1); err != nil {
// 		if err != gocui.ErrUnknownView {
// 			return err
// 		}
// 		v.Autoscroll = true
// 		v.Frame = true
// 		v.Wrap = true
// 		fmt.Fprintln(v, "Msgs")
// 	}
// 	if v, err := g.SetView("input", -1, maxY-2, maxX, maxY); err != nil {
// 		if err != gocui.ErrUnknownView {
// 			return err
// 		}
// 		v.FgColor = gocui.ColorYellow
// 		v.Editable = true
// 		v.Wrap = true
// 		v.Frame = true
// 	}
// 	return nil
// }

func keybindings(g *gocui.Gui) error {
	if err := g.SetKeybinding("", gocui.KeyCtrlC, gocui.ModNone, quit); err != nil {
		return err
	}
	return nil
}

// 	if err := g.SetKeybinding("input", gocui.KeyCtrlJ, gocui.ModNone, c.selectNextChan); err != nil {
// 		return err
// 	}

// 	if err := g.SetKeybinding("input", gocui.KeyCtrlK, gocui.ModNone, c.selectPrevChan); err != nil {
// 		return err
// 	}

// 	if err := g.SetKeybinding("", gocui.KeyCtrlL, gocui.ModNone, c.channelSelector); err != nil {
// 		return err
// 	}
// 	return nil
// }

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

func (slice channelSlice) Sort() {
	cs := &channelSorter{
		channelSlice: slice,
	}

	sort.Sort(cs)
}

type messagesInChannel map[string]int

type channelSorter struct {
	channelSlice channelSlice
}

func (c *channelSorter) Len() int { return len(c.channelSlice) }
func (c *channelSorter) Swap(i, j int) {
	c.channelSlice[i], c.channelSlice[j] = c.channelSlice[j], c.channelSlice[i]
}
func (c *channelSorter) Less(i, j int) bool {
	return c.channelSlice[i].ID < c.channelSlice[j].ID
}

type MessagesWidget struct {
	name string
	x, y int
	w    int

	storage       *Storage
	activeChannel config.Channel
	users         []config.User
	messages      map[string]art.Tree
}

func NewMessagesWidget(name string, x, y int, s *Storage) *MessagesWidget {
	return &MessagesWidget{
		name:    name,
		x:       x,
		y:       y,
		storage: s,
	}
}

func (w *MessagesWidget) Layout(g *gocui.Gui) error {
	maxX, maxY := g.Size()
	v, err := g.SetView(w.name, 0, maxY/2, maxX, maxY)
	if err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
	}
	v.Clear()
	fmt.Fprint(v, "These are the messages:\n")

	w.storage.IterateOverChannelMsgs(func(msg config.Message, userName string) {
		indentedText := text.Indent(sanitize.HTML(msg.Text), "")
		fmt.Fprintf(
			v,
			"%s %s: %s\n",
			grayColor(formatTime(msg.Timestamp)),
			colorize(fmt.Sprintf("%12.12s", userName)),
			whiteColor(indentedText),
		)
	})
	return nil
}

type ChannelsWidget struct {
	name string
	x, y int
	w    int

	storage          *Storage
	conn             *Conn
	channelFilter    string
	channelSelection int
}

func NewChannelsWidget(name string, x, y int, s *Storage, c *Conn) *ChannelsWidget {
	return &ChannelsWidget{
		name:    name,
		x:       x,
		y:       y,
		storage: s,
		conn:    c,
	}
}

func filterChannels(channels channelSlice, f func(config.Channel) bool) channelSlice {
	vsf := make(channelSlice, 0)
	for _, v := range channels {
		if f(v) {
			vsf = append(vsf, v)
		}
	}
	return vsf
}

func (w *ChannelsWidget) filteredChannels() channelSlice {
	return filterChannels(
		w.storage.channels,
		func(channel config.Channel) bool {
			return w.channelFilter == "" || strings.Contains(channel.ID, w.channelFilter)
		},
	)
}

// Layout handles console display layouting
func (w *ChannelsWidget) Layout(g *gocui.Gui) error {
	maxX, maxY := g.Size()
	v, err := g.SetView(w.name, 0, 0, maxX, maxY/2)
	if err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		if _, err := g.SetCurrentView(w.name); err != nil {
			return err
		}
		v.Editable = true
		v.Editor = gocui.EditorFunc(w.Editor)
	}

	v.Clear()
	format := "%s\n"
	fmt.Fprintf(v, format, w.storage.activeChannel.ID)
	for i, channel := range w.filteredChannels() {
		if channel == w.storage.activeChannel {
			continue
		}
		isSelected := i == w.channelSelection
		splitByColor := strings.Split(channel.ID, w.channelFilter)
		if isSelected {
			selectionColored := make([]string, len(splitByColor))
			for i, val := range splitByColor {
				selectionColored[i] = highlightChannel(val)
			}
			splitByColor = selectionColored
		}
		output := strings.Join(splitByColor, redColor(w.channelFilter))
		fmt.Fprintf(v, format, output)
	}
	return nil
}

func (w *ChannelsWidget) Editor(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) {
	switch {
	case ch != 0 && mod == 0:
		w.channelFilter += string(ch)
	case key == gocui.KeySpace:
		w.channelFilter += " "
	case key == gocui.KeyCtrlJ:
		w.channelSelection++
	case key == gocui.KeyCtrlK:
		w.channelSelection--
	case key == gocui.KeyBackspace || key == gocui.KeyBackspace2:
		if len(w.channelFilter) > 0 {
			w.channelFilter = w.channelFilter[:len(w.channelFilter)-1]
		}
	case key == gocui.KeyEnter:
		channels := w.filteredChannels()
		nChannels := len(channels)
		if nChannels > 0 {
			w.channelSelection = (w.channelSelection + nChannels) % nChannels
			w.storage.SetActiveChannel(channels[w.channelSelection])
			w.conn.fetchMessages(
				w.storage.activeChannel,
				w.storage.getLastMessageTimestamp(),
			)
		}

		w.channelFilter = ""
		return
	}

	channels := w.filteredChannels()
	nChannels := len(channels)
	if nChannels > 0 {
		w.channelSelection = (w.channelSelection + nChannels) % nChannels
	}
	// c.redrawChannelSelector(v, w.channelSelection)
}

func (w *ChannelsWidget) setActiveChannel(g *gocui.Gui, v *gocui.View) error {
	channels := w.filteredChannels()
	nChannels := len(channels)
	w.channelSelection = (w.channelSelection + nChannels) % nChannels
	if nChannels > 0 {
		// w.setActiveChannel(channels[w.channelIndex])
		w.storage.SetActiveChannel(channels[w.channelSelection])
	}
	// w.conn.fetchMessages("", w.storage.getLastMessageTimestamp())
	return nil
}

type storage struct {
	activeChannel    config.Channel
	channels         channelSlice
	unreadMessages   messagesInChannel
	totalMessages    messagesInChannel
	users            []config.User
	messages         map[string]art.Tree
	channelFilter    string
	channelIndex     int
	filteredChannels channelSlice
}

type comms struct {
	conn    *Conn
	storage *Storage
	g       *gocui.Gui
}

func findInSlice(slice []interface{}, itemToFind interface{}) int {
	for i, item := range slice {
		if item == itemToFind {
			return i
		}
	}
	return -1
}

// func (c *comms) channelSelector(g *gocui.Gui, v *gocui.View) error {
// 	c.storage.channelFilter = ""
// 	c.storage.channelIndex = 0
// 	c.storage.filteredChannels = c.storage.channels
// 	_, err := g.SetCurrentView("chan_selector")
// 	g.SetViewOnTop("chan_selector")
// 	chanelSelectorV, _ := g.View("chan_selector")
// 	c.redrawChannelSelector(chanelSelectorV, 0)
// 	return err
// }

// func (c *comms) setActiveChannel(channel config.Channel) {
// 	c.storage.activeChannel = channel
// 	c.storage.unreadMessages[c.storage.activeChannel.ID] = 0
// 	c.fetchMessages()
// }

// func (c *comms) selectNextChan(g *gocui.Gui, v *gocui.View) error {
// 	l := len(c.storage.channels)
// 	pos := c.storage.channels.pos(c.storage.activeChannel)
// 	c.setActiveChannel(c.storage.channels[(pos+1)%l])
// 	c.redraw(g)
// 	return nil
// }

// func (c *comms) selectPrevChan(g *gocui.Gui, v *gocui.View) error {
// 	l := len(c.storage.channels)
// 	pos := c.storage.channels.pos(c.storage.activeChannel)
// 	c.setActiveChannel(c.storage.channels[(pos+l-1)%l])
// 	c.redraw(g)
// 	return nil
// }

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

// func (c *comms) getLastMessageTimestamp(channel string) time.Time {
// 	defer mut.Unlock()
// 	if channelMsgs, ok := c.storage.messages[c.storage.activeChannel.ID]; ok {
// 		mut.Lock()
// 		msgs := channelMsgs.Iterator()
// 		if msgs.HasNext() {
// 			firstMsg, _ := msgs.Next()
// 			msg, _ := firstMsg.Value().(config.Message)
// 			return msg.Timestamp
// 		}
// 	}
// 	return time.Time{}
// }

// func (c *comms) fetchMessages() {
// 	activeChannel := c.storage.activeChannel.ID
// 	c.conn.fetchMessages(activeChannel, c.getLastMessageTimestamp(activeChannel))
// }

// func (c *comms) getUser(account, userID string) config.User {
// 	for _, user := range c.storage.users {
// 		if user.User == userID {
// 			return user
// 		}
// 	}
// 	return config.User{Name: userID}
// }

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

// func (c *comms) redraw(g *gocui.Gui) error {
// 	if vActiveChannel, err := g.View("active_channel"); err == nil {
// 		vActiveChannel.Clear()
// 		fmt.Fprintln(vActiveChannel, redColor(c.storage.activeChannel.Name))
// 	}

// 	vMsg, err := g.View("msgs")
// 	if err != nil {
// 		return err
// 	}
// 	vMsg.Clear()

// 	if activeChannelMsgs, ok := c.storage.messages[c.storage.activeChannel.ID]; ok {
// 		mut.Lock()
// 		for it := activeChannelMsgs.Iterator(); it.HasNext(); {
// 			node, err := it.Next()
// 			if err != nil || node.Kind() != art.Leaf {
// 				panic(err)
// 			}
// 			value := node.Value()
// 			msg, _ := value.(config.Message)
// 			userName := c.getUser(msg.Account, msg.Username).Name
// 			indentedText := text.Indent(sanitize.HTML(msg.Text), "")
// 			fmt.Fprintf(
// 				vMsg,
// 				"%s %s: %s\n",
// 				grayColor(formatTime(msg.Timestamp)),
// 				colorize(fmt.Sprintf("%12.12s", userName)),
// 				whiteColor(indentedText),
// 			)
// 		}
// 		mut.Unlock()
// 	}

// 	mut.Lock()
// 	redrawChannels(g, c.storage.channels, c.storage.activeChannel, c.storage.unreadMessages)
// 	mut.Unlock()

// 	return nil
// }

// func (c *comms) websocketConnect(g *gocui.Gui) {
// 	URL := "ws://localhost:8001/ws"

// 	var dialer *websocket.Dialer

// 	conn, _, err := dialer.Dial(URL, nil)
// 	if err != nil {
// 		fmt.Println(err)
// 		g.Close()
// 		return
// 	}

// 	defer func() {
// 		conn.Close()
// 		wg.Add(-1)
// 	}()

// 	go c.wsWriter(conn)

// 	c.commands <- config.Command{
// 		Type:    "get_channels",
// 		Command: config.GetChannelsCommand{},
// 	}

// 	c.commands <- config.Command{
// 		Type:    "get_users",
// 		Command: config.GetUsersCommand{},
// 	}

// 	for {
// 		_, message, err := conn.ReadMessage()
// 		if err != nil {
// 			fmt.Println("read:", err)
// 			return
// 		}

// 		msg := bweb.WireMessage{}

// 		err = json.Unmarshal(message, &msg)
// 		if err != nil {
// 			fmt.Println("failed to parse json:", err)
// 			return
// 		}

// 		if msg.Type == "user" {
// 			c.storage.users = append(c.storage.users, msg.User)
// 		}

// 		if msg.Type == "channel" {
// 			c.storage.channels = append(c.storage.channels, msg.Channel)
// 			nullChan := config.Channel{}
// 			if c.storage.activeChannel == nullChan {
// 				c.storage.activeChannel = msg.Channel
// 			}
// 		}

// 		if msg.Type == "message" {
// 			bucket := msg.Message.Channel + ":" + msg.Message.Account
// 			if _, ok := c.storage.messages[bucket]; !ok {
// 				c.storage.messages[bucket] = art.New()
// 			}
// 			mut.Lock()
// 			c.storage.messages[bucket].Insert(art.Key(msg.Message.GetKey()), msg.Message)
// 			c.storage.totalMessages[bucket]++
// 			if c.storage.activeChannel.ID != bucket {
// 				c.storage.unreadMessages[bucket]++
// 			}
// 			c.storage.channels.Sort(c.storage.unreadMessages, c.storage.totalMessages)
// 			mut.Unlock()
// 		}

// 		g.Execute(func(g *gocui.Gui) error {
// 			return c.redraw(g)
// 		})
// 	}
// }

// func (c *comms) wsWriter(conn *websocket.Conn) {
// 	defer func() {
// 		conn.Close()
// 	}()
// 	for {
// 		select {
// 		case message := <-c.messages:
// 			fmt.Printf("Message: %#v", message)

// 			jsonMessage, _ := json.Marshal(bweb.InboundWireMessage{
// 				Type:    "message",
// 				Message: message,
// 			})
// 			conn.WriteMessage(
// 				websocket.TextMessage,
// 				jsonMessage,
// 			)
// 		case command := <-c.commands:
// 			jsonMessage, _ := json.Marshal(bweb.InboundWireMessage{
// 				Type:    "command",
// 				Message: command,
// 			})
// 			conn.WriteMessage(
// 				websocket.TextMessage,
// 				jsonMessage,
// 			)
// 		case <-done:
// 			return
// 		}
// 	}
// }

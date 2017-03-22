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

func whiteColor(s string) string {
	return fmt.Sprintf("\x1b[38;5;%dm%s\x1b[0m", 256, s)
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

func keybindings(g *gocui.Gui) error {
	if err := g.SetKeybinding("", gocui.KeyCtrlC, gocui.ModNone, quit); err != nil {
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
}

func (w *ChannelsWidget) setActiveChannel(g *gocui.Gui, v *gocui.View) error {
	channels := w.filteredChannels()
	nChannels := len(channels)
	w.channelSelection = (w.channelSelection + nChannels) % nChannels
	if nChannels > 0 {
		w.storage.SetActiveChannel(channels[w.channelSelection])
	}
	return nil
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

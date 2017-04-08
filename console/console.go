package main

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/42wim/matterbridge/bridge/config"
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

	connection := NewConnection(
		done,
		storage.NewMessage,
		storage.NewUser,
		storage.NewChannel,
		storage.MarkAsRead,
	)

	window := NewWindow(g, storage, connection)
	if err := window.manage(); err != nil {
		panic(err)
	}

	go func() {
		err := connection.WebsocketConnect()
		fmt.Println(err)
		g.Close()
	}()

	if err := g.MainLoop(); err != nil && err != gocui.ErrQuit {
		fmt.Println(err)
	}
}

type Window struct {
	messagesWidget  *MessagesWidget
	channelsWidget  *ChannelsWidget
	statusbarWidget *StatusBarWidget
	inputWidget     *InputWidget

	storage    *Storage
	connection *Conn

	gui *gocui.Gui
}

func NewWindow(gui *gocui.Gui, storage *Storage, connection *Conn) *Window {
	w := &Window{
		messagesWidget:  newMessagesWidget("messages", storage),
		channelsWidget:  newChannelsWidget("channels", storage),
		statusbarWidget: newStatusBarWidget("statusbar", storage),
		storage:         storage,
		connection:      connection,
		gui:             gui,
	}

	w.inputWidget = newInputWidget("input", w.sendMessage)
	return w
}

func (w *Window) sendMessage(text string) {
	activeChannel := w.storage.activeChannel
	w.connection.messages <- config.Message{
		Text:    strings.TrimSpace(text),
		Channel: activeChannel.Channel,
		To:      activeChannel.Account,
	}
}

func (w *Window) manage() error {
	w.gui.SetManager(w.statusbarWidget, w.messagesWidget, w.inputWidget, w.channelsWidget)

	if err := w.gui.SetKeybinding("", gocui.KeyCtrlC, gocui.ModNone, w.quit); err != nil {
		return err
	}
	if err := w.gui.SetKeybinding("", gocui.KeyCtrlK, gocui.ModNone, w.openChannelPicker); err != nil {
		return err
	}
	return nil
}

func (w *Window) SetActiveChannel(channel config.Channel) {
	w.storage.SetActiveChannel(channel)
	w.connection.fetchMessages(
		w.storage.activeChannel,
		w.storage.getLastMessageTimestamp(),
	)
	lastMessageInChannel := w.storage.LastMessageInChannel(channel.ID)
	w.connection.MarkAsRead(lastMessageInChannel)
}

func (w *Window) openChannelPicker(g *gocui.Gui, v *gocui.View) error {
	w.messagesWidget.active = false
	w.inputWidget.active = false
	w.channelsWidget.active = true
	w.channelsWidget.addCallback(func(isCanceled bool, channel config.Channel) {
		w.messagesWidget.active = true
		w.inputWidget.active = true
		w.channelsWidget.active = false
		if !isCanceled {
			w.SetActiveChannel(channel)
		}
	})
	return nil
}

func (w *Window) quit(g *gocui.Gui, v *gocui.View) error {
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

type InputWidget struct {
	name        string
	active      bool
	storage     *Storage
	sendMessage func(text string)
}

func newInputWidget(name string, sendMessage func(text string)) *InputWidget {
	return &InputWidget{
		name:        name,
		active:      true,
		sendMessage: sendMessage,
	}
}

func (w *InputWidget) Layout(g *gocui.Gui) error {
	maxX, maxY := g.Size()
	v, err := g.SetView(w.name, -1, maxY-2, maxX, maxY)
	if err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Autoscroll = true
		v.Wrap = true
		v.Frame = true
		v.Editor = gocui.EditorFunc(w.editor)
	}
	v.Editable = w.active
	if w.active {
		g.SetCurrentView(w.name)
		g.SetViewOnTop(w.name)
	}

	return nil
}

func (w *InputWidget) editor(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) {
	switch {
	case ch != 0 && mod == 0:
		v.EditWrite(ch)
	case key == gocui.KeySpace:
		v.EditWrite(' ')
	case key == gocui.KeyBackspace || key == gocui.KeyBackspace2:
		v.EditDelete(true)
	case key == gocui.KeyDelete:
		v.EditDelete(false)
	case key == gocui.KeyInsert:
		v.Overwrite = !v.Overwrite
	case key == gocui.KeyEnter:
		w.sendMessage(v.Buffer())
		v.Clear()
	case key == gocui.KeyArrowDown:
		v.MoveCursor(0, 1, false)
	case key == gocui.KeyArrowUp:
		v.MoveCursor(0, -1, false)
	case key == gocui.KeyArrowLeft:
		v.MoveCursor(-1, 0, false)
	case key == gocui.KeyArrowRight:
		v.MoveCursor(1, 0, false)
	}
}

type MessagesWidget struct {
	name   string
	active bool

	storage       *Storage
	activeChannel config.Channel
	users         []config.User
	messages      map[string]art.Tree
}

func newMessagesWidget(name string, s *Storage) *MessagesWidget {
	return &MessagesWidget{
		name:    name,
		active:  true,
		storage: s,
	}
}

func (w *MessagesWidget) Layout(g *gocui.Gui) error {
	maxX, maxY := g.Size()
	v, err := g.SetView(w.name, -1, 1, maxX, maxY-1)
	if err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Autoscroll = true
		v.Wrap = true
		v.Frame = false
	}
	if w.active {
		g.SetCurrentView(w.name)
		g.SetViewOnTop(w.name)
	}
	v.Clear()
	fmt.Fprint(v, "These are the messages:\n")

	w.storage.IterateOverChannelMsgs(
		w.storage.activeChannel.ID,
		func(msg config.Message, userName string) {
			indentedText := text.Indent(sanitize.HTML(msg.Text), "")
			fmt.Fprintf(
				v,
				"%s %s: %s\n",
				grayColor(formatTime(msg.Timestamp)),
				colorize(fmt.Sprintf("%12.12s", userName)),
				whiteColor(indentedText),
			)
		},
	)
	return nil
}

type StatusBarWidget struct {
	name   string
	active bool

	storage *Storage
}

func newStatusBarWidget(name string, s *Storage) *StatusBarWidget {
	return &StatusBarWidget{
		name:    name,
		storage: s,
	}
}

func (w *StatusBarWidget) Layout(g *gocui.Gui) error {
	maxX, _ := g.Size()
	v, err := g.SetView(w.name, -1, -1, maxX, 1)
	if err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		if _, err := g.SetCurrentView(w.name); err != nil {
			return err
		}
		v.Frame = false
	}

	v.Clear()
	w.storage.readLock.Lock()
	for channel, count := range w.storage.unreadMessages {
		if count > 0 {
			fmt.Fprintf(v, "%s (%d)", channel, count)
		}
	}
	w.storage.readLock.Unlock()

	return nil
}

type ChannelsWidget struct {
	name   string
	active bool

	storage          *Storage
	channelFilter    string
	channelSelection int
	channelSelected  func(isCanceled bool, c config.Channel)
}

func newChannelsWidget(name string, s *Storage) *ChannelsWidget {
	return &ChannelsWidget{
		name:    name,
		storage: s,
	}
}

func (w *ChannelsWidget) addCallback(cb func(isCanceled bool, c config.Channel)) {
	w.channelSelected = cb
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
	v, err := g.SetView(w.name, -1, 1, maxX, maxY)
	if err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		if _, err := g.SetCurrentView(w.name); err != nil {
			return err
		}
		v.Frame = false
		v.Editable = true
		v.Editor = gocui.EditorFunc(w.editor)
	}

	if !w.active {
		return nil
	}

	g.SetCurrentView(w.name)
	g.SetViewOnTop(w.name)

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

func (w *ChannelsWidget) editor(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) {
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
			w.channelSelected(false, channels[w.channelSelection])
		} else {
			w.channelSelected(true, config.Channel{})
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

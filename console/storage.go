package main

import (
	"sync"
	"time"

	"github.com/42wim/matterbridge/bridge/config"
	"github.com/plar/go-adaptive-radix-tree"
)

var (
	mut = &sync.Mutex{}
)

type readMessagesInChannel map[string]config.Message

// Storage handles storing stuff
type Storage struct {
	messages       map[string]art.Tree
	unreadMessages map[string]int
	readMessages   readMessagesInChannel
	readLock       *sync.Mutex
	activeChannel  config.Channel
	channels       channelSlice
	users          []config.User
	redraw         func()
}

func NewStorage(redraw func()) *Storage {
	storage := &Storage{}
	storage.messages = make(map[string]art.Tree)
	storage.unreadMessages = make(map[string]int)
	storage.readMessages = make(readMessagesInChannel)
	storage.activeChannel = config.Channel{}
	storage.channels = make(channelSlice, 0)
	storage.users = make([]config.User, 0)
	storage.redraw = redraw
	storage.readLock = &sync.Mutex{}
	return storage
}

func (s *Storage) NewMessage(m config.Message) bool {
	channelID := m.Channel + ":" + m.Account
	if _, ok := s.messages[channelID]; !ok {
		s.messages[channelID] = art.New()
	}
	mut.Lock()
	s.messages[channelID].Insert(art.Key(m.GetKey()), m)
	mut.Unlock()

	if s.activeChannel.ID != channelID {
		s.readLock.Lock()
		s.unreadMessages[channelID] = s.CountUnreadMessages(channelID)
		s.readLock.Unlock()
	}
	s.redraw()
	return m.Channel == s.activeChannel.Channel && m.Account == s.activeChannel.Account
}

func (s *Storage) NewChannel(c config.Channel) {
	s.channels = append(s.channels, c)
	s.channels.Sort()
	s.redraw()
}

func (s *Storage) NewUser(u config.User) {
	s.users = append(s.users, u)
	s.redraw()
}

func (s *Storage) MarkAsRead(msg config.Message) {
	channelID := msg.Channel + ":" + msg.Account
	s.readMessages[channelID] = msg
	s.readLock.Lock()
	s.unreadMessages[channelID] = s.CountUnreadMessages(channelID)
	s.readLock.Unlock()
}

func (s *Storage) SetActiveChannel(channel config.Channel) {
	s.activeChannel = channel
	s.MarkAsRead(s.LastMessageInChannel(channel.ID))
}

func (s *Storage) GetUser(account, userID string) config.User {
	for _, user := range s.users {
		if user.User == userID {
			return user
		}
	}
	return config.User{Name: userID}
}

func (s *Storage) getLastMessageTimestamp() time.Time {
	if channelMsgs, ok := s.messages[s.activeChannel.ID]; ok {
		mut.Lock()
		defer mut.Unlock()
		msgs := channelMsgs.Iterator()
		if msgs.HasNext() {
			firstMsg, _ := msgs.Next()
			msg, _ := firstMsg.Value().(config.Message)
			return msg.Timestamp
		}
	}
	return time.Time{}
}

func (s *Storage) LastMessageInChannel(channelID string) config.Message {
	lastMsg := config.Message{}
	s.IterateOverChannelMsgs(channelID, func(msg config.Message, _ string) {
		lastMsg = msg
	})
	return lastMsg
}

func (s *Storage) CountUnreadMessages(channelID string) int {
	unread := 0
	lastReadTime := s.readMessages[channelID].Timestamp
	s.IterateOverChannelMsgs(channelID, func(msg config.Message, _ string) {
		if msg.Timestamp.After(lastReadTime) {
			unread++
		}
	})
	return unread
}

func (s *Storage) IterateOverChannelMsgs(
	channelID string,
	cb func(msg config.Message, userName string),
) {
	mut.Lock()
	defer mut.Unlock()
	if activeChannelMsgs, ok := s.messages[channelID]; ok {
		for it := activeChannelMsgs.Iterator(); it.HasNext(); {
			node, err := it.Next()
			if err != nil || node.Kind() != art.Leaf {
				panic(err)
			}
			value := node.Value()
			msg, _ := value.(config.Message)
			userName := s.GetUser(msg.Account, msg.Username).Name
			cb(msg, userName)
		}
	}
}

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

// Storage handles storing stuff
type Storage struct {
	messages       map[string]art.Tree
	unreadMessages map[string]int
	activeChannel  config.Channel
	channels       channelSlice
	users          []config.User
	redraw         func()
}

func NewStorage(redraw func()) *Storage {
	storage := &Storage{}
	storage.messages = make(map[string]art.Tree)
	storage.unreadMessages = make(map[string]int)
	storage.activeChannel = config.Channel{}
	storage.channels = make(channelSlice, 0)
	storage.users = make([]config.User, 0)
	storage.redraw = redraw
	return storage
}

func (s *Storage) NewMessage(m config.Message) {
	bucket := m.Channel + ":" + m.Account
	if _, ok := s.messages[bucket]; !ok {
		s.messages[bucket] = art.New()
	}
	mut.Lock()
	s.messages[bucket].Insert(art.Key(m.GetKey()), m)
	mut.Unlock()
	if s.activeChannel.ID != bucket {
		s.unreadMessages[bucket]++
	}
	s.redraw()
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

func (s *Storage) SetActiveChannel(channel config.Channel) {
	s.activeChannel = channel
	s.unreadMessages[s.activeChannel.ID] = 0
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

func (s *Storage) IterateOverChannelMsgs(cb func(msg config.Message, userName string)) {
	mut.Lock()
	defer mut.Unlock()
	if activeChannelMsgs, ok := s.messages[s.activeChannel.ID]; ok {
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

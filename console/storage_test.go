package main

import (
	"testing"
	"time"

	"github.com/42wim/matterbridge/bridge/config"
	"github.com/stretchr/testify/assert"
)

func TestCountUnreadNewMessages(t *testing.T) {
	s := NewStorage(func() {})
	s.activeChannel = config.Channel{Account: "foo2", Channel: "bar", ID: "bar:foo2"}
	s.NewMessage(config.Message{Account: "foo", Channel: "bar", Timestamp: time.Date(2000, 1, 1, 0, 0, 0, 0, time.Local)})
	s.NewMessage(config.Message{Account: "foo", Channel: "bar", Timestamp: time.Date(2000, 1, 1, 0, 0, 0, 0, time.Local)})
	s.NewMessage(config.Message{Account: "foo", Channel: "bar", Timestamp: time.Date(2002, 1, 1, 0, 0, 0, 0, time.Local)})
	s.NewMessage(config.Message{Account: "foo", Channel: "bar", Timestamp: time.Date(2002, 1, 1, 0, 0, 0, 0, time.Local)})
	s.readMessages["bar:foo"] = config.Message{Timestamp: time.Date(2001, 1, 1, 0, 0, 0, 0, time.Local)}
	unread := s.unreadMessages["bar:foo"]
	assert.Equal(t, 2, unread)
}

func TestCountUnreadMessages(t *testing.T) {
	s := NewStorage(func() {})
	s.activeChannel = config.Channel{Account: "foo2", Channel: "bar", ID: "bar:foo2"}
	s.NewMessage(config.Message{Account: "foo", Channel: "bar", Timestamp: time.Date(2000, 1, 1, 0, 0, 0, 0, time.Local)})
	s.NewMessage(config.Message{Account: "foo", Channel: "bar", Timestamp: time.Date(2000, 1, 1, 0, 0, 0, 0, time.Local)})
	s.NewMessage(config.Message{Account: "foo", Channel: "bar", Timestamp: time.Date(2002, 1, 1, 0, 0, 0, 0, time.Local)})
	lastMessage := config.Message{Account: "foo", Channel: "bar", Timestamp: time.Date(2003, 1, 1, 0, 0, 0, 0, time.Local)}
	s.NewMessage(lastMessage)
	s.MarkAsRead(lastMessage)

	unread := s.unreadMessages["bar:foo"]
	assert.Equal(t, 0, unread)
}

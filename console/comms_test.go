package main

import (
	"testing"
	"time"

	"github.com/42wim/matterbridge/bridge/config"
	"github.com/stretchr/testify/assert"
)

func TestHandleWebsocketMessageUser(t *testing.T) {
	c := &Conn{}
	handlerCalled := false
	c.newUser = func(config.User) {
		handlerCalled = true
	}
	c.handleWebsocketMessage([]byte(`{"Type": "user", "User": {}}`))
	assert.Equal(t, true, handlerCalled)
}

func TestHandleWebsocketMessageChannel(t *testing.T) {
	c := &Conn{}
	handlerCalled := false
	c.newChannel = func(config.Channel) {
		handlerCalled = true
	}
	c.handleWebsocketMessage([]byte(`{"Type": "channel", "Channel": {}}`))
	assert.Equal(t, true, handlerCalled)
}

func TestHandleWebsocketMessageMessage(t *testing.T) {
	c := &Conn{}
	handlerCalled := false
	c.newMessage = func(config.Message) bool {
		handlerCalled = true
		return true
	}
	c.handleWebsocketMessage([]byte(`{"Type": "message", "Message": {}}`))
	assert.Equal(t, true, handlerCalled)
}

func TestHandleWebsocketMessageReadStatus(t *testing.T) {
	c := &Conn{}
	handler1Called := false
	handler2Called := false
	c.readMessage = func(config.Message) {
		handler2Called = true
	}

	done := make(chan struct{})

	c.commands = make(chan config.Command)
	go func() {
		for range c.commands {
			handler1Called = true
			done <- struct{}{}
		}
	}()
	c.handleWebsocketMessage([]byte(`{"Type": "read_status", "Message": {}}`))
	<-done
	assert.Equal(t, true, handler1Called)
	assert.Equal(t, true, handler2Called)
}

func TestMarkAsRead(t *testing.T) {
	c := &Conn{}
	c.debouncedCommands = make(chan config.Message)
	done := make(chan config.Message)
	go func() {
		for c := range c.debouncedCommands {
			done <- c
		}
	}()
	c.SendMarkAsRead(config.Message{})
	assert.Equal(t, config.Command{
		Type: "mark_message_as_read",
		Command: config.MarkMessageAsRead{
			Message: config.Message{},
		},
	}, <-done)
}

func TestDebouncedCommands(t *testing.T) {
	c := &Conn{}
	c.commands = make(chan config.Command)
	c.debouncedCommands = debounceCommandChannel(1*time.Second, c.commands)
	done := make(chan config.Command)
	go func() {
		for c := range c.commands {
			done <- c
		}
	}()

	firstMsgTime := time.Now()
	c.debouncedCommands <- config.Message{Timestamp: firstMsgTime, Text: "foobar"}
	time.Sleep(500 * time.Millisecond)
	secondMsgTime := time.Now()
	c.debouncedCommands <- config.Message{Timestamp: secondMsgTime, Text: "foobar2"}
	assert.Equal(t, config.Command{
		Type: "mark_message_as_read",
		Command: config.MarkMessageAsRead{
			Message: config.Message{
				Timestamp: secondMsgTime,
				Text:      "foobar2",
			},
		},
	}, <-done)
	assert.Equal(t, true, time.Since(firstMsgTime) > 1500*time.Millisecond)
	assert.Equal(t, true, time.Since(firstMsgTime) < 1600*time.Millisecond)

	ThirdMsgTime := time.Now()
	c.debouncedCommands <- config.Message{Timestamp: ThirdMsgTime, Text: "foobar3"}
	assert.Equal(t, config.Command{
		Type: "mark_message_as_read",
		Command: config.MarkMessageAsRead{
			Message: config.Message{
				Timestamp: ThirdMsgTime,
				Text:      "foobar3",
			},
		},
	}, <-done)
	assert.Equal(t, true, time.Since(ThirdMsgTime) > 1000*time.Millisecond)
	assert.Equal(t, true, time.Since(ThirdMsgTime) < 1100*time.Millisecond)
}

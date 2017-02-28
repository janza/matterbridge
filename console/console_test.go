package main

import (
	"testing"
	"time"

	"github.com/42wim/matterbridge/bridge/config"
	"github.com/stretchr/testify/assert"
)

func TestInsertMessage(t *testing.T) {
	msgs := []config.Message{}

	msgs = insertMessage(msgs, config.Message{
		Timestamp: time.Date(2005, 1, 1, 0, 0, 0, 0, time.Local),
	})
	msgs = insertMessage(msgs, config.Message{
		Timestamp: time.Date(2000, 1, 1, 0, 0, 0, 0, time.Local),
	})
	msgs = insertMessage(msgs, config.Message{
		Timestamp: time.Date(2010, 1, 1, 0, 0, 0, 0, time.Local),
	})

	assert.Equal(t, []config.Message{
		config.Message{Timestamp: time.Date(2000, 1, 1, 0, 0, 0, 0, time.Local)},
		config.Message{Timestamp: time.Date(2005, 1, 1, 0, 0, 0, 0, time.Local)},
		config.Message{Timestamp: time.Date(2010, 1, 1, 0, 0, 0, 0, time.Local)},
	}, msgs)
}

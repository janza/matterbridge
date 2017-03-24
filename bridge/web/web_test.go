package bweb

import (
	"testing"
	"time"

	"github.com/42wim/matterbridge/bridge/config"
	"github.com/stretchr/testify/assert"
)

func processString(s string) (interface{}, error) {
	c := Client{
		account: "foobar",
	}
	return c.processMessage([]byte(s))
}

func TestInvalidJsonReturnsError(t *testing.T) {
	_, err := processString("")
	assert.NotNil(t, err)
}

func TestProcessInvalidMessage(t *testing.T) {
	_, err := processString("{}")
	assert.Equal(t, "missing Message field", err.Error())
}

func TestProcessInvalidMessageType(t *testing.T) {
	_, err := processString(`{
		"Type": "foobar",
		"Message": {
			"Username": "test"
		}
	}`)
	assert.Equal(t, "unknown message received", err.Error())
}

func TestProcessIncomingMessage(t *testing.T) {
	message, err := processString(`{
		"Type": "message",
		"Message": {
			"Username": "test"
		}
	}`)
	assert.Nil(t, err)
	assert.Equal(t, config.Message{Account: "foobar"}, message)
}

func TestProcessInvalidCommand(t *testing.T) {
	_, err := processString(`{
		"Type": "command",
		"Message": {
			"Username": "test"
		}
	}`)
	assert.Equal(t, "unknown command received", err.Error())
}

func TestProcessIncomingGetLastReadMessageCommand(t *testing.T) {
	message, err := processString(`{
		"Type": "command",
		"Message": {
			"Type": "get_last_read_message",
			"Command": {
				"Channel": "chan"
			}
		}
	}`)
	assert.Nil(t, err)
	assert.Equal(
		t,
		config.Command{
			Type:    "get_last_read_message",
			Command: config.GetLastReadMessage{Channel: "chan"},
		},
		message,
	)
}

func TestProcessIncomingMarkMessageAsReadCommand(t *testing.T) {
	message, err := processString(`{
		"Type": "command",
		"Message": {
			"Type": "mark_message_as_read",
			"Command": {
				"Message": {
					"Username": "test"
				}
			}
		}
	}`)
	assert.Nil(t, err)
	assert.Equal(
		t,
		config.Command{
			Type:    "mark_message_as_read",
			Command: config.MarkMessageAsRead{Message: config.Message{Username: "test"}},
		},
		message,
	)
}

func TestProcessIncomingGetMessagesCommand(t *testing.T) {
	message, err := processString(`{
		"Type": "command",
		"Message": {
			"Type": "replay_messages",
			"Command": {
				"Channel": "channel",
				"Offset": "0001-01-01T00:00:00Z"
			}
		}
	}`)
	assert.Nil(t, err)
	assert.Equal(
		t,
		config.Command{
			Type: "replay_messages",
			Command: config.GetMessagesCommand{
				Channel: "channel",
				Offset:  time.Time{},
			},
		},
		message,
	)
}

func TestProcessIncomingGetChannelsCommand(t *testing.T) {
	message, err := processString(`{
		"Type": "command",
		"Message": {
			"Type": "get_channels",
			"Command": {}
		}
	}`)
	assert.Nil(t, err)
	assert.Equal(
		t,
		config.Command{
			Type:    "get_channels",
			Command: config.GetChannelsCommand{},
		},
		message,
	)
}

func TestProcessIncomingGetUsersCommand(t *testing.T) {
	message, err := processString(`{
		"Type": "command",
		"Message": {
			"Type": "get_users",
			"Command": {}
		}
	}`)
	assert.Nil(t, err)
	assert.Equal(
		t,
		config.Command{
			Type:    "get_users",
			Command: config.GetUsersCommand{},
		},
		message,
	)
}

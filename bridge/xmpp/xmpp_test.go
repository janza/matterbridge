package bxmpp

import (
	"testing"

	"github.com/42wim/matterbridge/bridge/config"
	"github.com/stretchr/testify/assert"
)

func TestParseJid(t *testing.T) {
	b := &Bxmpp{
		Config: &config.Protocol{
			Muc: "chat.hipchat.com",
		},
	}
	node, domain, resource := b.parseJid("157160_3883674@chat.hipchat.com/none||proxy|pubproxy-b600.hipchat.com|5262")
	assert.Equal(t, "157160_3883674", node)
	assert.Equal(t, "chat.hipchat.com", domain)
	assert.Equal(t, "none||proxy|pubproxy-b600.hipchat.com|5262", resource)
}

func TestGetUserNameFromJid(t *testing.T) {
	b := &Bxmpp{
		Config: &config.Protocol{
			Muc: "chat.hipchat.com",
		},
	}
	username := b.getUsernameFromJid("157160_3883674@chat.hipchat.com/Nick")
	assert.Equal(t, "Nick", username)
}

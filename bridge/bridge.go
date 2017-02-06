package bridge

import (
	"github.com/42wim/matterbridge/bridge/config"
	"github.com/42wim/matterbridge/bridge/discord"
	"github.com/42wim/matterbridge/bridge/gitter"
	"github.com/42wim/matterbridge/bridge/irc"
	"github.com/42wim/matterbridge/bridge/mattermost"
	"github.com/42wim/matterbridge/bridge/rocketchat"
	"github.com/42wim/matterbridge/bridge/slack"
	"github.com/42wim/matterbridge/bridge/telegram"
	"github.com/42wim/matterbridge/bridge/xmpp"
	"strings"
)

type Bridger interface {
	Send(msg config.Message) error
	Connect() error
	JoinChannel(channel string) error
}

type Bridge struct {
	Config config.Protocol
	Bridger
	Name     string
	Account  string
	Protocol string
}

func New(cfg *config.Config, bridge *config.Bridge, c config.Comms) *Bridge {
	b := new(Bridge)
	accInfo := strings.Split(bridge.Account, ".")
	protocol := accInfo[0]
	name := accInfo[1]
	b.Name = name
	b.Protocol = protocol
	b.Account = bridge.Account
	m := c.Messages

	// override config from environment
	config.OverrideCfgFromEnv(cfg, protocol, name)
	switch protocol {
	case "mattermost":
		b.Config = cfg.Mattermost[name]
		b.Bridger = bmattermost.New(cfg.Mattermost[name], bridge.Account, m)
	case "irc":
		b.Config = cfg.IRC[name]
		b.Bridger = birc.New(cfg.IRC[name], bridge.Account, m)
	case "gitter":
		b.Config = cfg.Gitter[name]
		b.Bridger = bgitter.New(cfg.Gitter[name], bridge.Account, m)
	case "slack":
		b.Config = cfg.Slack[name]
		b.Bridger = bslack.New(cfg.Slack[name], bridge.Account, m)
	case "xmpp":
		b.Config = cfg.Xmpp[name]
		b.Bridger = bxmpp.New(cfg.Xmpp[name], bridge.Account, c)
	case "discord":
		b.Config = cfg.Discord[name]
		b.Bridger = bdiscord.New(cfg.Discord[name], bridge.Account, m)
	case "telegram":
		b.Config = cfg.Telegram[name]
		b.Bridger = btelegram.New(cfg.Telegram[name], bridge.Account, m)
	case "rocketchat":
		b.Config = cfg.Rocketchat[name]
		b.Bridger = brocketchat.New(cfg.Rocketchat[name], bridge.Account, m)
	}
	return b
}

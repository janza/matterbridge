package webgateway

import (
	"github.com/42wim/matterbridge/bridge"
	"github.com/42wim/matterbridge/bridge/config"
	"github.com/42wim/matterbridge/bridge/disk"
	"github.com/42wim/matterbridge/bridge/web"
	log "github.com/Sirupsen/logrus"
	// "strings"
)

var (
	flog *log.Entry
)

func init() {
	flog = log.WithFields(log.Fields{"module": "gateway"})
}

type WebGateway struct {
	*config.Config
	MyConfig   *config.WebGateway
	Bridges    map[string]*bridge.Bridge
	WebBridge  *bweb.Bweb
	DiskBridge *bdisk.Bdisk
	// Channels map[string]*bridge.Bridge
}

func New(cfg *config.Config, gateway *config.WebGateway) error {
	c := config.Comms{}
	c.Messages = make(chan config.Message, 10)
	c.Users = make(chan config.User, 10)
	c.Channels = make(chan config.Channel, 10)
	c.MessageLog = make(chan config.Message, 10)
	c.Commands = make(chan string, 10)
	gw := &WebGateway{}
	gw.Bridges = make(map[string]*bridge.Bridge)
	gw.Config = cfg
	gw.MyConfig = gateway

	gw.WebBridge = bweb.New(cfg.Web["web.server"], "web.server", c)
	gw.WebBridge.Connect()

	gw.DiskBridge = bdisk.New(c)

	for _, account := range gateway.Accounts {
		br := config.Bridge{Account: account.Account}
		flog.Infof("Starting bridge: %s", account.Account)
		gw.Bridges[account.Account] = bridge.New(cfg, &br, c)
	}

	for _, br := range gw.Bridges {
		err := br.Connect()
		if err != nil {
			flog.Fatalf("Bridge %s failed to start: %v", br.Account, err)
		}
		for _, account := range gateway.Accounts {
			if account.Account != br.Account {
				continue
			}
			for _, channel := range account.Channels {
				flog.Infof("%s: joining %s", br.Account, channel)
				br.JoinChannel(channel)
			}
		}
	}
	gw.handleReceive(c)

	return nil
}

func (gw *WebGateway) handleReceive(c config.Comms) {
	for {
		select {
		case msg := <-c.Messages:
			flog.Debugf("Got message %#v", msg)
			gw.handleMessage(msg)
		case user := <-c.Users:
			flog.Debugf("Got user presence %#v", user)
			gw.handleUser(user)
		case channel := <-c.Channels:
			flog.Debugf("Got channel %#v", channel)
			gw.handleChannel(channel)
		case cmd := <-c.Commands:
			flog.Debugf("Got command %#v", cmd)
			gw.handleCommand(cmd)
		case msg := <-c.MessageLog:
			flog.Debugf("Got message from log %#v", msg)
			gw.handleLog(msg)
		}
	}
}

func (gw *WebGateway) handleUser(user config.User) {
	if user.Origin != "disk" {
		gw.DiskBridge.Presence(user)
	}
	gw.WebBridge.Presence(user)
}

func (gw *WebGateway) handleChannel(channel config.Channel) {
	if channel.Origin != "disk" {
		gw.DiskBridge.Discovery(channel)
	}
	gw.WebBridge.Discovery(channel)
}

func (gw *WebGateway) handleCommand(cmd string) {
	gw.DiskBridge.HandleCommand(cmd)
}

func (gw *WebGateway) handleLog(msg config.Message) {
	flog.Debugf("Got message log message %#v", msg)
	gw.WebBridge.Send(msg)
}

func (gw *WebGateway) handleMessage(msg config.Message) {
	flog.Debugf("Got message from %s: %s", msg.Account, msg.Text)
	if err := gw.DiskBridge.Send(msg); err != nil {
		flog.Error(err)
	}
	if msg.Account == gw.WebBridge.Account {
		targetBridge := gw.Bridges[msg.To]
		if targetBridge != nil {
			if err := targetBridge.Send(msg); err != nil {
				flog.Error(err)
			}
		} else {
			flog.Errorf("Bridge not found: %s", msg.To)
		}
		return
	}
	flog.Debugf("Sending %#v from %s (%s)", msg, msg.Account, msg.Channel)
	if err := gw.WebBridge.Send(msg); err != nil {
		flog.Error(err)
	}
	flog.Debug("Sent message")
}

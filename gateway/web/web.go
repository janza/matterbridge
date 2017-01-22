package webgateway

import (
	"github.com/42wim/matterbridge/bridge"
	"github.com/42wim/matterbridge/bridge/config"
	log "github.com/Sirupsen/logrus"
	// "strings"
)

type WebGateway struct {
	*config.Config
	MyConfig  *config.WebGateway
	Bridges   map[string]*bridge.Bridge
	WebBridge *bridge.Bridge
	// Channels map[string]*bridge.Bridge
}

func New(cfg *config.Config, gateway *config.WebGateway) error {
	c := make(chan config.Message)
	gw := &WebGateway{}
	gw.Bridges = make(map[string]*bridge.Bridge)
	gw.Config = cfg
	gw.MyConfig = gateway

	gw.WebBridge = bridge.New(cfg, &config.Bridge{Account: "web.server"}, c)
	gw.WebBridge.Connect()

	for _, account := range gateway.Accounts {
		br := config.Bridge{Account: account.Account}
		log.Infof("Starting bridge: %s", account.Account)
		gw.Bridges[account.Account] = bridge.New(cfg, &br, c)
	}

	for _, br := range gw.Bridges {
		err := br.Connect()
		if err != nil {
			log.Fatalf("Bridge %s failed to start: %v", br.Account, err)
		}
		for _, account := range gateway.Accounts {
			if account.Account != br.Account {
				continue
			}
			for _, channel := range account.Channels {
				log.Infof("%s: joining %s", br.Account, channel)
				br.JoinChannel(channel)
			}
		}
	}
	gw.handleReceive(c)

	return nil
}

func (gw *WebGateway) handleReceive(c chan config.Message) {
	for {
		select {
		case msg := <-c:
			gw.handleMessage(msg)
		}
	}
}

func (gw *WebGateway) handleMessage(msg config.Message) {
	log.Debugf("Got message from %s: %s", msg.Account, msg.Text)

	if msg.Account == gw.WebBridge.Account {
		targetBridge := gw.Bridges[msg.To]
		if targetBridge != nil {
			if err := targetBridge.Send(msg); err != nil {
				log.Error(err)
			}
		} else {
			log.Errorf("Bridge not found %s", msg.To)
		}
		return
	}
	log.Debugf("Sending %#v from %s (%s)", msg, msg.Account, msg.Channel)
	if err := gw.WebBridge.Send(msg); err != nil {
		log.Error(err)
	}
}

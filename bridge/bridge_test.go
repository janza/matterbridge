package bridge_test

import (
	"github.com/42wim/matterbridge/bridge"
	"github.com/42wim/matterbridge/bridge/config"
	"testing"
)

func TestBasic(t *testing.T) {
	protocolMap := make(map[string]config.Protocol)
	protocolMap["test"] = config.Protocol{}
	cfg := &config.Config{
		General: config.Protocol{},
		Web:     protocolMap,
	}
	c := make(chan config.Message)
	webBridge := bridge.New(cfg, &config.Bridge{Account: "web.test"}, c)
	webBridge.Connect()
	webBridge.Send(config.Message{Text: "test"})
}

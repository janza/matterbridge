package config

import (
	"fmt"
	"github.com/BurntSushi/toml"
	"log"
	"os"
	"reflect"
	"strings"
	"time"
)

const (
	EVENT_JOIN_LEAVE = "join_leave"
	EVENT_FAILURE    = "failure"
)

type Message struct {
	Text      string
	Channel   string
	Username  string
	Avatar    string
	Account   string
	Event     string
	To        string
	IsPriv    bool
	Timestamp time.Time
	Protocol  string
}

type User struct {
	ID      string
	User    string
	Name    string
	Account string
	Origin  string
}

type Channel struct {
	ID      string
	Channel string
	Name    string
	Account string
	Origin  string
}

type Command struct {
	Command string
	Origin  string
}

type Comms struct {
	Messages   chan Message
	MessageLog chan Message
	Users      chan User
	Channels   chan Channel
	Commands   chan string
}

type Protocol struct {
	BindAddress            string // mattermost, slack, web
	Buffer                 int    // api
	IconURL                string // mattermost, slack
	IgnoreNicks            string // all protocols
	Jid                    string // xmpp
	Login                  string // mattermost
	Muc                    string // xmpp
	Name                   string // all protocols
	Nick                   string // all protocols
	NickFormatter          string // mattermost, slack
	NickServNick           string // IRC
	NickServPassword       string // IRC
	NicksPerRow            int    // mattermost, slack
	NoTLS                  bool   // mattermost
	Password               string // IRC,mattermost,XMPP
	PrefixMessagesWithNick bool   // mattemost, slack
	Protocol               string //all protocols
	MessageQueue           int    // IRC, size of message queue for flood control
	MessageDelay           int    // IRC, time in millisecond to wait between messages
	RemoteNickFormat       string // all protocols
	Server                 string // IRC,mattermost,XMPP,discord
	TLSServerName          string // IRC,mattermost,XMPP,discord
	ShowJoinPart           bool   // all protocols
	SkipTLSVerify          bool   // IRC, mattermost
	Team                   string // mattermost
	Token                  string // gitter, slack, discord
	URL                    string // mattermost, slack
	UseAPI                 bool   // mattermost, slack
	UseSASL                bool   // IRC
	UseTLS                 bool   // IRC
}

type ChannelOptions struct {
	Key string // irc
}

type Bridge struct {
	Account string
	Channel string
	Options ChannelOptions
}

type Gateway struct {
	Name   string
	Enable bool
	In     []Bridge
	Out    []Bridge
	InOut  []Bridge
}

type SameChannelGateway struct {
	Name     string
	Enable   bool
	Channels []string
	Accounts []string
}

type AccountWithChannels struct {
	Account  string
	Channels []string
}

type WebGateway struct {
	Name     string
	Enable   bool
	Accounts []AccountWithChannels
}

type Config struct {
	Api                map[string]Protocol
	IRC                map[string]Protocol
	Mattermost         map[string]Protocol
	Matrix             map[string]Protocol
	Slack              map[string]Protocol
	Gitter             map[string]Protocol
	Xmpp               map[string]Protocol
	Discord            map[string]Protocol
	Telegram           map[string]Protocol
	Rocketchat         map[string]Protocol
	Web                map[string]Protocol
	Disk               map[string]Protocol
	General            Protocol
	Gateway            []Gateway
	WebGateway         WebGateway
	SameChannelGateway []SameChannelGateway
}

func NewUser(id, account, name string) User {
	return User{
		ID:      fmt.Sprintf("%s:%s", id, account),
		User:    id,
		Account: account,
		Name:    name,
	}
}

func NewChannel(id, account, name string) Channel {
	return Channel{
		ID:      fmt.Sprintf("%s:%s", id, account),
		Channel: id,
		Name:    name,
		Account: account,
	}
}

func NewConfig(cfgfile string) *Config {
	var cfg Config
	if _, err := toml.DecodeFile(cfgfile, &cfg); err != nil {
		log.Fatal(err)
	}
	return &cfg
}

func OverrideCfgFromEnv(cfg *Config, protocol string, account string) {
	var protoCfg Protocol
	val := reflect.ValueOf(cfg).Elem()
	// loop over the Config struct
	for i := 0; i < val.NumField(); i++ {
		typeField := val.Type().Field(i)
		// look for the protocol map (both lowercase)
		if strings.ToLower(typeField.Name) == protocol {
			// get the Protocol struct from the map
			data := val.Field(i).MapIndex(reflect.ValueOf(account))
			protoCfg = data.Interface().(Protocol)
			protoStruct := reflect.ValueOf(&protoCfg).Elem()
			// loop over the found protocol struct
			for i := 0; i < protoStruct.NumField(); i++ {
				typeField := protoStruct.Type().Field(i)
				// build our environment key (eg MATTERBRIDGE_MATTERMOST_WORK_LOGIN)
				key := "matterbridge_" + protocol + "_" + account + "_" + typeField.Name
				key = strings.ToUpper(key)
				// search the environment
				res := os.Getenv(key)
				// if it exists and the current field is a string
				// then update the current field
				if res != "" {
					fieldVal := protoStruct.Field(i)
					if fieldVal.Kind() == reflect.String {
						log.Printf("config: overriding %s from env with %s\n", key, res)
						fieldVal.Set(reflect.ValueOf(res))
					}
				}
			}
			// update the map with the modified Protocol (cfg.Protocol[account] = Protocol)
			val.Field(i).SetMapIndex(reflect.ValueOf(account), reflect.ValueOf(protoCfg))
			break
		}
	}
}

func GetIconURL(msg *Message, cfg *Protocol) string {
	iconURL := cfg.IconURL
	info := strings.Split(msg.Account, ".")
	protocol := info[0]
	name := info[1]
	iconURL = strings.Replace(iconURL, "{NICK}", msg.Username, -1)
	iconURL = strings.Replace(iconURL, "{BRIDGE}", name, -1)
	iconURL = strings.Replace(iconURL, "{PROTOCOL}", protocol, -1)
	return iconURL
}

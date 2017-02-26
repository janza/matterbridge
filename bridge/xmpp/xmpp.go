package bxmpp

import (
	"crypto/tls"
	"encoding/xml"
	"github.com/42wim/matterbridge/bridge/config"
	log "github.com/Sirupsen/logrus"
	"github.com/mattn/go-xmpp"
	"strings"
	"time"
)

type Bxmpp struct {
	xc         *xmpp.Client
	xmppMap    map[string]string
	Config     *config.Protocol
	Remote     chan config.Message
	Channels   chan config.Channel
	Users      chan config.User
	Account    string
	KnownUsers map[string]string
}

type Invite struct {
	XMLName xml.Name `xml:"invite"`
	Reason  string   `xml:"reason"`
}

var flog *log.Entry
var protocol = "xmpp"

func init() {
	flog = log.WithFields(log.Fields{"module": protocol})
}

func New(cfg config.Protocol, account string, c config.Comms) *Bxmpp {
	b := &Bxmpp{}
	b.xmppMap = make(map[string]string)
	b.Config = &cfg
	b.Account = account

	b.Remote = c.Messages
	b.Channels = c.Channels
	b.Users = c.Users
	b.KnownUsers = make(map[string]string)
	return b
}

func (b *Bxmpp) Connect() error {
	var err error
	flog.Infof("Connecting %s", b.Config.Server)
	b.xc, err = b.createXMPP()
	if err != nil {
		flog.Debugf("%#v", err)
		return err
	}
	flog.Info("Connection succeeded")
	b.xc.Roster()
	b.xc.RawInformationQuery(b.Config.Jid, b.Config.Muc, "onetwo", "get", "http://jabber.org/protocol/disco#items", "")
	go b.handleXMPP()
	return nil
}

func (b *Bxmpp) Disconnect() error {
	return nil
}

func (b *Bxmpp) JoinChannel(channel string) error {
	fullChannelName := channel + "@" + b.Config.Muc
	b.xc.JoinMUCNoHistory(fullChannelName, b.Config.Nick)
	flog.Debugf("Adding channel %s", channel)
	b.Channels <- config.NewChannel(channel, b.Account, "")
	flog.Debugf("Added channel %s", channel)
	return nil
}

func (b *Bxmpp) Send(msg config.Message) error {
	flog.Debugf("Receiving %#v", msg)
	if strings.Contains(msg.Channel, "@"+b.Config.Muc) {
		flog.Debugf("Sending groupchat to: %s %s", msg.Channel, msg.Text)
		_, err := b.xc.Send(xmpp.Chat{
			Type:   "groupchat",
			Remote: msg.Channel,
			Text:   msg.Username + msg.Text,
		})
		return err
	}

	flog.Debugf("Sending private chat message to: %s %s", msg.Channel, msg.Text)
	_, err := b.xc.Send(xmpp.Chat{
		Type:   "chat",
		Remote: msg.Channel,
		Text:   msg.Username + msg.Text,
	})
	return err
}

func (b *Bxmpp) createXMPP() (*xmpp.Client, error) {
	tc := new(tls.Config)
	tc.InsecureSkipVerify = b.Config.SkipTLSVerify
	tc.ServerName = strings.Split(b.Config.Server, ":")[0]
	if b.Config.TLSServerName != "" {
		tc.ServerName = b.Config.TLSServerName
	}
	options := xmpp.Options{
		Host:      b.Config.Server,
		User:      b.Config.Jid,
		Password:  b.Config.Password,
		NoTLS:     true,
		StartTLS:  true,
		TLSConfig: tc,

		//StartTLS:      false,
		Debug:                        true,
		Session:                      true,
		Status:                       "",
		StatusMessage:                "",
		Resource:                     "",
		InsecureAllowUnencryptedAuth: false,
		//InsecureAllowUnencryptedAuth: true,
	}
	var err error
	b.xc, err = options.NewClient()
	return b.xc, err
}

func (b *Bxmpp) xmppKeepAlive() chan bool {
	done := make(chan bool)
	go func() {
		ticker := time.NewTicker(90 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				b.xc.PingC2S("", "")
			case <-done:
				return
			}
		}
	}()
	return done
}

func (b *Bxmpp) parseJid(jid string) (string, string, string) {
	s := strings.Split(jid, "@")
	node := ""
	domain := ""
	resource := ""
	if len(s) == 2 {
		node, domain = s[0], s[1]
	} else {
		domain = s[0]
	}
	s = strings.Split(domain, "/")
	if len(s) == 2 {
		resource = s[1]
	}
	domain = s[0]
	return node, domain, resource
}

func (b *Bxmpp) getUsername(v xmpp.Chat) string {
	node, domain, resource := b.parseJid(v.Remote)
	if v.Type == "chat" {
		return node + "@" + domain
	}
	knownUserName := b.KnownUsers[resource]
	if knownUserName == "" {
		return resource
	}
	return knownUserName
}

func (b *Bxmpp) getUsernameFromJid(jid string) string {
	node, server, resource := b.parseJid(jid)
	if server == b.Config.Muc {
		return resource
	}
	return node
}

func (b *Bxmpp) getChannel(v xmpp.Chat) string {
	node, domain, _ := b.parseJid(v.Remote)
	return node + "@" + domain
}

func (b *Bxmpp) handleXMPP() error {
	done := b.xmppKeepAlive()
	defer func() {
		close(done)
		b.Connect()
	}()
	for {
		m, err := b.xc.Recv()
		if err != nil {
			flog.Error(err)
			return err
		}

		switch v := m.(type) {
		case xmpp.Chat:
			if v.Type == "groupchat" || v.Type == "chat" {
				nick := b.getUsername(v)
				channel := b.getChannel(v)
				isPriv := v.Type == "chat"

				if nick != b.Config.Nick && v.Text != "" {
					b.Remote <- config.Message{
						Username:  nick,
						Text:      v.Text,
						Timestamp: v.Stamp,
						Channel:   channel,
						Account:   b.Account,
						IsPriv:    isPriv,
					}
				}
			} else {
				for _, innerMsg := range v.OtherElem {
					invite := Invite{}
					err := xml.Unmarshal([]byte(innerMsg.InnerXML), &invite)
					if err == nil {
						b.xc.JoinMUCNoHistory(v.Remote, b.Config.Nick)
					}
				}
			}
		case xmpp.IQ:
			for _, i := range v.ClientQuery.Item {
				b.KnownUsers[i.Name] = i.Jid
				b.Users <- config.NewUser(i.Jid, b.Account, i.Name)
			}
			flog.Info(string(v.Query))
			for _, channel := range v.DiscoQuery.Item {
				b.Channels <- config.NewChannel(channel.Jid, b.Account, channel.Name)
			}
		case xmpp.Presence:
			if v.MucJid == "" {
				continue
			}
			nick := b.getUsernameFromJid(v.From)
			flog.Warnf("Adding to know users %s: %s", nick, v.MucJid)
			b.KnownUsers[nick] = v.MucJid
			b.Users <- config.NewUser(v.MucJid, b.Account, nick)
		}
	}
}

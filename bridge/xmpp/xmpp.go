package bxmpp

import (
	"crypto/tls"
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
	Account    string
	KnownUsers map[string]string
}

var flog *log.Entry
var protocol = "xmpp"

func init() {
	flog = log.WithFields(log.Fields{"module": protocol})
}

func New(cfg config.Protocol, account string, c chan config.Message) *Bxmpp {
	b := &Bxmpp{}
	b.xmppMap = make(map[string]string)
	b.Config = &cfg
	b.Account = account
	b.Remote = c
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
	go b.handleXmpp()
	return nil
}

func (b *Bxmpp) JoinChannel(channel string) error {
	b.xc.JoinMUCNoHistory(channel+"@"+b.Config.Muc, b.Config.Nick)
	return nil
}

func (b *Bxmpp) Send(msg config.Message) error {
	flog.Debugf("Receiving %#v", msg)
	if strings.ContainsRune(msg.Channel, '@') {
		flog.Debugf("Sending private chat message to: %s %s", msg.Channel, msg.Text)
		b.xc.Send(xmpp.Chat{
			Type:   "chat",
			Remote: msg.Channel,
			Text:   msg.Username + msg.Text,
		})
		return nil
	}
	flog.Debugf("Sending groupchat to: %s %s", msg.Channel+"@"+b.Config.Muc, msg.Text)
	b.xc.Send(xmpp.Chat{
		Type:   "groupchat",
		Remote: msg.Channel + "@" + b.Config.Muc,
		Text:   msg.Username + msg.Text,
	})
	return nil
}

func (b *Bxmpp) createXMPP() (*xmpp.Client, error) {
	tc := new(tls.Config)
	tc.InsecureSkipVerify = b.Config.SkipTLSVerify
	tc.ServerName = strings.Split(b.Config.Server, ":")[0]
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
	return b.KnownUsers[resource]
}

func (b *Bxmpp) getChannel(v xmpp.Chat) string {
	node, domain, _ := b.parseJid(v.Remote)
	if v.Type == "chat" {
		return node + "@" + domain
	}
	return node
}

func (b *Bxmpp) handleXmpp() error {
	done := b.xmppKeepAlive()
	defer close(done)
	for {
		m, err := b.xc.Recv()
		if err != nil {
			return err
		}

		switch v := m.(type) {
		case xmpp.Chat:
			if v.Type == "groupchat" || v.Type == "chat" {
				nick := b.getUsername(v)
				channel := b.getChannel(v)
				isPriv := v.Type == "chat"
				flog.Warnf("CHAT: [%s] %s", nick, b.Account)

				if nick != b.Config.Nick && v.Text != "" {
					b.Remote <- config.Message{
						Username: nick,
						Text:     v.Text,
						Channel:  channel,
						Account:  b.Account,
						IsPriv:   isPriv,
					}
				}
			}
		case xmpp.IQ:
			for _, i := range v.ClientQuery.Item {
				b.KnownUsers[i.Name] = i.Jid
				flog.Warnf("Adding to know users %s: %s", i.Name, i.Jid)
			}
		}
	}
}

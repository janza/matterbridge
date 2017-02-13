package bdisk

import (
	"encoding/json"
	"github.com/42wim/matterbridge/bridge/config"
	log "github.com/Sirupsen/logrus"
	"io"
	"os"
	// "io/ioutil"
)

type Bdisk struct {
	Comms   config.Comms
	Account string
	File    config.Comms
}

var (
	flog *log.Entry
)

func init() {
	flog = log.WithFields(log.Fields{"module": "disk"})
}

func New(c config.Comms) *Bdisk {
	b := &Bdisk{}
	b.Comms = c
	b.Account = "disk"
	return b
}

func check(e error) {
	if e != nil {
		panic(e)
	}
}

func (b *Bdisk) WriteToFile(filename string, data interface{}) error {
	f, err := os.OpenFile(filename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	check(err)
	defer f.Close()
	bytes, err := json.Marshal(data)
	_, err = f.WriteString(string(bytes))
	check(err)
	_, err = f.WriteString("\n")
	check(err)
	return nil
}

func (b *Bdisk) Send(msg config.Message) error {
	return b.WriteToFile("messages.json", msg)
}

func (b *Bdisk) Presence(user config.User) error {
	return b.WriteToFile("users.json", user)
}

func (b *Bdisk) Discovery(channel config.Channel) error {
	return b.WriteToFile("channels.json", channel)
}

func (b *Bdisk) HandleCommand(cmd string) error {
	go func() {
		flog.Debugf("Loading message log")
		f, err := os.Open("messages.json")
		if err != nil {
			flog.Warnf("Failed to open message.json: %s", err)
			return
		}
		d := json.NewDecoder(f)
		for {
			var msg config.Message
			if err := d.Decode(&msg); err == io.EOF {
				break // done decoding file
			} else if err != nil {
				flog.Warnf("Failed to load message: %s", err)
				break
			}
			flog.Debugf("Sending message to the log chan %#v", msg)
			b.Comms.MessageLog <- msg
		}

		flog.Debugf("Done loading message log")
	}()
	return nil
}

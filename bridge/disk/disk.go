package bdisk

import (
	"bufio"
	"encoding/json"
	"github.com/42wim/matterbridge/bridge/config"
	"io"
	"os"
	// "io/ioutil"
)

type Bdisk struct {
	Comms   config.Comms
	Account string
	File    config.Comms
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
	f, err := os.Create(filename)
	w := bufio.NewWriter(f)
	bytes, err := json.Marshal(data)
	check(err)
	_, err = w.Write(bytes)
	_, err = w.Write([]byte("\n"))
	check(err)
	w.Flush()
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
	f, err := os.Open("messages.json")
	if err != nil {
		return err
	}
	d := json.NewDecoder(f)
	for {
		var v config.Message
		if err := d.Decode(&v); err == io.EOF {
			break // done decoding file
		} else if err != nil {
			// handle error
		}
		b.Comms.Messages <- v
	}
	return nil
}

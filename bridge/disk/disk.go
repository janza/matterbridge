package bdisk

import (
	"bytes"
	"container/list"
	"encoding/json"
	"github.com/42wim/matterbridge/bridge/config"
	log "github.com/Sirupsen/logrus"
	"io"
	"io/ioutil"
	"os"
)

type Bdisk struct {
	Comms   config.Comms
	Account string
	File    config.Comms
}

type ChannelMap map[string]config.Channel
type UserMap map[string]config.User
type KeyValStore map[string]interface{}

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

func (b *Bdisk) AppendToFile(filename string, data interface{}) error {
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

func (b *Bdisk) StoreKeyValue(filename string, key string, value interface{}) error {
	var keyValueStore KeyValStore
	var keyValueStoreRaw []byte
	keyValueStoreRaw, err := ioutil.ReadFile(filename)
	_, ok := err.(*os.PathError)
	if !ok {
		check(err)
	}
	if string(keyValueStoreRaw) != "" {
		err = json.Unmarshal(keyValueStoreRaw, &keyValueStore)
		check(err)
	} else {
		keyValueStore = KeyValStore{}
	}
	keyValueStore[key] = value
	newBytes, err := json.Marshal(keyValueStore)
	err = ioutil.WriteFile(filename, newBytes, 0644)
	check(err)
	return err
}

func (b *Bdisk) ReadKeyValue(filename string, value interface{}) error {
	var fileContents []byte
	fileContents, err := ioutil.ReadFile(filename)
	_, ok := err.(*os.PathError)
	if err != nil && !ok {
		return err
	}
	if string(fileContents) == "" {
		return nil
	}
	err = json.Unmarshal(fileContents, value)
	return err
}

func (b *Bdisk) ReplayUsers() {
	var userMap UserMap
	b.ReadKeyValue("users.json", &userMap)
	for _, user := range userMap {
		log.Infof("Replaying user: %s", user.ID)
		b.Comms.Users <- user
	}
}

func (b *Bdisk) ReplayChannels() {
	var channelMap ChannelMap
	b.ReadKeyValue("channels.json", &channelMap)
	for _, channel := range channelMap {
		log.Infof("Replaying channel: %s", channel.ID)
		b.Comms.Channels <- channel
	}
}

func lineCounter(r io.Reader) (int, error) {
	buf := make([]byte, 32*1024)
	count := 0
	lineSep := []byte{'\n'}

	for {
		c, err := r.Read(buf)
		count += bytes.Count(buf[:c], lineSep)

		switch {
		case err == io.EOF:
			return count, nil

		case err != nil:
			return count, err
		}
	}
}

func (b *Bdisk) TailLog(filename string, n int, offset int) list.List {
	l := list.New()
	f, err := os.Open(filename)
	if err != nil {
		flog.Warnf("Failed to open message.json: %s", err)
		return *l
	}

	numberOrMessagesInLog, err := lineCounter(f)
	check(err)

	flog.Printf("numberOrMessagesInLog %d", numberOrMessagesInLog)

	d := json.NewDecoder(f)
	f.Seek(0, 0)
	for i := 0; i < numberOrMessagesInLog-offset; i++ {
		var msg config.Message
		err := d.Decode(&msg)
		if err == io.EOF {
			break // done decoding file
		} else if err != nil {
			flog.Warnf("Failed to load message: %s", err)
			break
		}
		l.PushFront(msg)
		if l.Len() > n {
			l.Remove(l.Back())
		}
	}

	return *l
}

func (b *Bdisk) Send(msg config.Message) error {
	return b.AppendToFile("messages.json", msg)
}

func (b *Bdisk) Presence(user config.User) error {
	return b.StoreKeyValue("users.json", user.ID, user)
}

func (b *Bdisk) Discovery(channel config.Channel) error {
	return b.StoreKeyValue("channels.json", channel.ID, channel)
}

func (b *Bdisk) ReplayMessages(numberOfMessages, offset int) {
	l := b.TailLog("messages.json", numberOfMessages, offset)

	for e := l.Front(); e != nil; e = e.Next() {
		msg, ok := e.Value.(config.Message)
		if ok {
			b.Comms.MessageLog <- msg
		} else {
			flog.Warnf("Message is not valid: %#v", e.Value)
		}
	}
}

func (b *Bdisk) HandleCommand(cmd string) error {
	switch cmd {
	case "connected":
		go b.ReplayMessages(100, 0)
		go b.ReplayUsers()
		go b.ReplayChannels()
	case "replay_messages":
		go b.ReplayMessages(100, 0)
	case "get_users":
		go b.ReplayUsers()
	case "get_channels":
		go b.ReplayChannels()
	}

	return nil
}

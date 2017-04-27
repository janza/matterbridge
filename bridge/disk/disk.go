package bdisk

import (
	"bytes"
	"container/list"
	"encoding/json"
	"io"
	"io/ioutil"
	"os"
	"time"

	"github.com/42wim/matterbridge/bridge/config"
	log "github.com/Sirupsen/logrus"
)

var (
	logPrefix = "logs/"
)

type Bdisk struct {
	Comms   config.Comms
	Account string
	File    config.Comms
}

type ChannelMap map[string]config.Channel
type UserMap map[string]config.User
type KeyValStore map[string]interface{}
type ReadStatusMap map[string]config.Message

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
	f, err := os.OpenFile(logPrefix+filename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
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
	keyValueStoreRaw, err := ioutil.ReadFile(logPrefix + filename)
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
	err = ioutil.WriteFile(logPrefix+filename, newBytes, 0644)
	check(err)
	return err
}

func (b *Bdisk) ReadKeyValue(filename string, value interface{}) error {
	var fileContents []byte
	fileContents, err := ioutil.ReadFile(logPrefix + filename)
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

type offsetTime struct {
	to   time.Time
	from time.Time
}

func (b *Bdisk) TailLog(filename string, n int, offset offsetTime) list.List {
	l := list.New()
	f, err := os.Open(logPrefix + filename)
	if err != nil {
		flog.Warnf("Failed to open message.json: %s", err)
		return *l
	}

	d := json.NewDecoder(f)
	f.Seek(0, 0)
	for {
		var msg config.Message
		err := d.Decode(&msg)
		if err == io.EOF {
			break // done decoding file
		} else if err != nil {
			flog.Warnf("Failed to load message: %s", err)
			break
		}
		if !offset.to.IsZero() && (msg.Timestamp.After(offset.to) || msg.Timestamp.Equal(offset.to)) {
			break
		}

		if offset.from.IsZero() {
			l.PushBack(msg)

			if l.Len() > n {
				l.Remove(l.Front())
			}
		} else {
			if offset.from.Before(msg.Timestamp) {
				l.PushBack(msg)
			}
		}
	}

	return *l
}

func (b *Bdisk) Send(msg config.Message) error {
	channelID := config.NewChannel(msg.Channel, msg.Account, "").ID
	return b.AppendToFile(channelID+"_log.json", msg)
}

func (b *Bdisk) MarkRead(msg config.Message) error {
	if msg.Timestamp.IsZero() {
		return nil
	}
	return b.StoreKeyValue("read_status.json", msg.Channel+":"+msg.Account, msg)
}

func (b *Bdisk) Presence(user config.User) error {
	return b.StoreKeyValue("users.json", user.ID, user)
}

func (b *Bdisk) Discovery(channel config.Channel) error {
	return b.StoreKeyValue("channels.json", channel.ID, channel)
}

func (b *Bdisk) ReplayMessages(channel string, numberOfMessages int, offset offsetTime) {
	l := b.TailLog(channel+"_log.json", numberOfMessages, offset)

	for e := l.Front(); e != nil; e = e.Next() {
		msg, ok := e.Value.(config.Message)
		if ok {
			b.Comms.MessageLog <- msg
		} else {
			flog.Warnf("Message is not valid: %#v", e.Value)
		}
	}
}

func (b *Bdisk) GetLastReadMessage(channel string) {
	var readStatusMap ReadStatusMap
	b.ReadKeyValue("read_status.json", &readStatusMap)
	b.Comms.ReadStatus <- readStatusMap[channel]
}

func (b *Bdisk) GetLastReadMessages() {
	var readStatusMap ReadStatusMap
	b.ReadKeyValue("read_status.json", &readStatusMap)
	for _, readMessage := range readStatusMap {
		b.ReplayMessages(readMessage.Channel+":"+readMessage.Account, 0, offsetTime{from: readMessage.Timestamp})
	}
}

func (b *Bdisk) HandleCommand(command interface{}) error {
	switch cmd := command.(type) {
	case config.GetMessagesCommand:
		go b.ReplayMessages(cmd.Channel, 100, offsetTime{to: cmd.Offset})
	case config.GetUsersCommand:
		go b.ReplayUsers()
	case config.GetChannelsCommand:
		go b.ReplayChannels()
	case config.MarkMessageAsRead:
		go b.MarkRead(cmd.Message)
	case config.GetLastReadMessage:
		go b.GetLastReadMessage(cmd.Channel)
	case config.GetLastReadMessages:
		go b.GetLastReadMessages()
	default:
		log.Warn("Unkown command received %#v", command)
	}

	return nil
}

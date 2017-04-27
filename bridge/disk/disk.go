package bdisk

import (
	"bytes"
	"container/list"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/42wim/matterbridge/bridge/config"
	log "github.com/Sirupsen/logrus"
	"github.com/boltdb/bolt"
)

var (
	logPrefix = "logs/"
)

type Bdisk struct {
	Comms   config.Comms
	Account string
	File    config.Comms
	db      *bolt.DB
}

type channelMap map[string]config.Channel
type userMap map[string]config.User

// type keyValStore map[string]interface{}
type readStatusMap map[string]config.Message

var (
	flog *log.Entry
)

func init() {
	flog = log.WithFields(log.Fields{"module": "disk"})
}

func New(c config.Comms, db *bolt.DB) *Bdisk {
	b := &Bdisk{}
	b.Comms = c
	b.Account = "disk"
	b.db = db
	return b
}

func check(e error) {
	if e != nil {
		panic(e)
	}
}

func (b *Bdisk) appendToFile(filename string, data config.Message) error {
	return b.db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(filename))
		if err != nil {
			return fmt.Errorf("create bucket: %s", err)
		}
		bytes, err := json.Marshal(data)
		if err != nil {
			return err
		}
		b.Put([]byte(data.GetKey()), bytes)
		return nil
	})
}

func (b *Bdisk) storeKeyValue(filename string, key string, value interface{}) error {
	return b.db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(filename))
		if err != nil {
			return fmt.Errorf("create bucket: %s", err)
		}
		bytes, err := json.Marshal(value)
		if err != nil {
			return err
		}
		b.Put([]byte(key), bytes)
		return nil
	})
	// var keyValueStore KeyValStore
	// var keyValueStoreRaw []byte
	// keyValueStoreRaw, err := ioutil.ReadFile(logPrefix + filename)
	// _, ok := err.(*os.PathError)
	// if !ok {
	// 	check(err)
	// }
	// if string(keyValueStoreRaw) != "" {
	// 	err = json.Unmarshal(keyValueStoreRaw, &keyValueStore)
	// 	check(err)
	// } else {
	// 	keyValueStore = KeyValStore{}
	// }
	// keyValueStore[key] = value
	// newBytes, err := json.Marshal(keyValueStore)
	// err = ioutil.WriteFile(logPrefix+filename, newBytes, 0644)
	// check(err)
	// return err
}

func (b *Bdisk) readKeyValue(filename, key string, value interface{}) error {
	return b.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(filename))
		if b == nil {
			return fmt.Errorf("bucket %q not found", filename)
		}
		v := b.Get([]byte(key))
		return json.Unmarshal(v, value)
	})
}

func (b *Bdisk) readAllValues(filename string, cb func([]byte) error) error {
	return b.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(filename))
		if b == nil {
			return fmt.Errorf("bucket %q not found", filename)
		}
		return b.ForEach(func(k, v []byte) error {
			flog.Printf("loaded %s", v)
			return cb(v)
		})
	})
}

func (b *Bdisk) replayUsers() {
	b.readAllValues("users.json", func(v []byte) error {
		var user config.User
		err := json.Unmarshal(v, &user)
		if err != nil {
			return err
		}
		log.Infof("Replaying user: %s", user.ID)
		b.Comms.Users <- user
		return nil
	})
}

func (b *Bdisk) replayChannels() {
	b.readAllValues("channels.json", func(v []byte) error {
		var channel config.Channel
		err := json.Unmarshal(v, &channel)
		if err != nil {
			return err
		}
		log.Infof("Replaying user: %s", channel.ID)
		b.Comms.Channels <- channel
		return nil
	})
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

func (b *Bdisk) tailLog(filename string, n int, offset offsetTime) list.List {
	l := list.New()
	err := b.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(filename))
		if bucket == nil {
			return fmt.Errorf("bucket %q not found", filename)
		}
		c := bucket.Cursor()

		min := []byte(offset.from.String()[:19])
		max := []byte(offset.to.String()[:19])

		flog.Printf("min %s %s", offset.from, offset.to)

		if !offset.from.IsZero() {
			k, _ := c.Seek(min)
			if k == nil {
				return nil
			}
			for k, v := c.Next(); k != nil; k, v = c.Next() {
				var msg config.Message
				err := json.Unmarshal(v, &msg)
				if err != nil {
					flog.Error(err)
					return err
				}
				l.PushBack(msg)
			}
			return nil
		}
		k, v := c.Seek(max)
		if k == nil {
			flog.Printf("not found, looking at last entry: %s", max)
			k, v = c.Last()
		} else {
			k, v = c.Prev()
		}
		for ; k != nil && l.Len() < n; k, v = c.Prev() {
			flog.Printf("Loaded %s %s", k, v)
			var msg config.Message
			err := json.Unmarshal(v, &msg)
			if err != nil {
				flog.Error(err)
				return err
			}
			l.PushFront(msg)
		}
		return nil
	})
	if err != nil {
		flog.Warnf("Failed to open message.json: %s", err)
	}
	return *l
}

func (b *Bdisk) Send(msg config.Message) error {
	channelID := config.NewChannel(msg.Channel, msg.Account, "").ID
	return b.appendToFile(channelID+"_log.json", msg)
}

func (b *Bdisk) markRead(msg config.Message) error {
	if msg.Timestamp.IsZero() {
		return nil
	}
	return b.storeKeyValue("read_status.json", msg.Channel+":"+msg.Account, msg)
}

func (b *Bdisk) Presence(user config.User) error {
	return b.storeKeyValue("users.json", user.ID, user)
}

func (b *Bdisk) Discovery(channel config.Channel) error {
	return b.storeKeyValue("channels.json", channel.ID, channel)
}

func (b *Bdisk) replayMessages(channel string, numberOfMessages int, offset offsetTime) {
	l := b.tailLog(channel+"_log.json", numberOfMessages, offset)

	for e := l.Front(); e != nil; e = e.Next() {
		msg, ok := e.Value.(config.Message)
		if ok {
			b.Comms.MessageLog <- msg
		} else {
			flog.Warnf("Message is not valid: %#v", e.Value)
		}
	}
}

func (b *Bdisk) getLastReadMessage(channel string) {
	var m config.Message
	b.readKeyValue("read_status.json", channel, &m)
	b.Comms.ReadStatus <- m
}

func (b *Bdisk) getLastReadMessages() {
	b.readAllValues("read_status.json", func(v []byte) error {
		var m config.Message
		err := json.Unmarshal(v, &m)
		if err != nil {
			return err
		}
		b.replayMessages(m.Channel+":"+m.Account, 0, offsetTime{from: m.Timestamp})
		return nil
	})
}

func (b *Bdisk) HandleCommand(command interface{}) error {
	switch cmd := command.(type) {
	case config.GetMessagesCommand:
		go b.replayMessages(cmd.Channel, 100, offsetTime{to: cmd.Offset})
	case config.GetUsersCommand:
		go b.replayUsers()
	case config.GetChannelsCommand:
		go b.replayChannels()
	case config.MarkMessageAsRead:
		go b.markRead(cmd.Message)
	case config.GetLastReadMessage:
		go b.getLastReadMessage(cmd.Channel)
	case config.GetLastReadMessages:
		go b.getLastReadMessages()
	default:
		log.Warn("Unkown command received %#v", command)
	}

	return nil
}

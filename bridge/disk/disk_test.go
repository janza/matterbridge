package bdisk

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/42wim/matterbridge/bridge/config"
	"github.com/boltdb/bolt"
	"github.com/stretchr/testify/assert"
)

type TestStruct struct {
	Foo string
}

func TestStoreReadKeyValue(t *testing.T) {
	testFile := "test.json"
	os.Remove("test.db")
	db, err := bolt.Open("test.db", 0600, nil)
	assert.NoError(t, err)
	b := &Bdisk{db: db}
	b.storeKeyValue(testFile, "key", config.Channel{
		Channel: "channel",
		Origin:  "irc",
	})

	var c config.Channel
	err = b.readKeyValue(testFile, "key", &c)
	assert.Nil(t, err)

	assert.Equal(t, config.Channel{
		Channel: "channel",
		Origin:  "irc",
	}, c)
}

func TestStoreReadKeyAllValue(t *testing.T) {
	testFile := "test.json"
	os.Remove("test.db")
	db, err := bolt.Open("test.db", 0600, nil)
	assert.NoError(t, err)
	b := &Bdisk{db: db}
	b.storeKeyValue(testFile, "key", config.Channel{
		ID:      "key",
		Channel: "channel",
		Origin:  "irc",
	})

	b.storeKeyValue(testFile, "key2", config.Channel{
		ID:      "key2",
		Channel: "channel2",
		Origin:  "irc2",
	})

	channels := make(map[string]config.Channel)
	err = b.readAllValues(testFile, func(v []byte) error {
		var c config.Channel
		err := json.Unmarshal(v, &c)
		if err != nil {
			return err
		}
		channels[c.ID] = c
		return nil
	})
	assert.Nil(t, err)

	expected := make(map[string]config.Channel)
	expected["key"] = config.Channel{
		ID:      "key",
		Channel: "channel",
		Origin:  "irc",
	}
	expected["key2"] = config.Channel{
		ID:      "key2",
		Channel: "channel2",
		Origin:  "irc2",
	}

	assert.Equal(t, expected, channels)
}

func TestTailLog(t *testing.T) {
	testFile := "test.json"
	os.Remove("test.db")
	db, err := bolt.Open("test.db", 0600, nil)
	assert.NoError(t, err)
	b := &Bdisk{db: db}
	for i := 0; i < 10; i++ {
		b.appendToFile(testFile, config.Message{
			Timestamp: time.Date(2000, 1, 1, 0, 0, i, 0, time.Local),
			Text:      "hi",
		})
	}

	for i := 0; i < 10; i++ {
		b.appendToFile(testFile, config.Message{
			Timestamp: time.Date(2000, 1, 1, 0, 1, i, 0, time.Local),
			Text:      "",
		})
	}

	l := b.tailLog(testFile, 10, offsetTime{to: time.Now()})

	i := 0
	for e := l.Front(); e != nil; e = e.Next() {
		assert.Equal(t, config.Message{
			Timestamp: time.Date(2000, 1, 1, 0, 1, i, 0, time.Local),
			Text:      "",
		}, e.Value, "element in the list matches expected")
		i++
	}

	assert.Equal(t, 10, l.Len(), "there should be 10 elements in list")
}

func TestTailLogWithOffset(t *testing.T) {
	testFile := "test.json"
	os.Remove("test.db")
	db, err := bolt.Open("test.db", 0600, nil)
	assert.NoError(t, err)
	b := &Bdisk{db: db}

	firstBatchTime := time.Date(2010, 1, 1, 0, 0, 0, 0, time.Local)
	for i := 0; i < 10; i++ {
		b.appendToFile(testFile, config.Message{
			Text:      "hi",
			Timestamp: firstBatchTime.Add(time.Second * time.Duration(i)),
		})
	}

	secondBatchTime := time.Date(2020, 1, 2, 0, 0, 0, 0, time.Local)
	for i := 0; i < 10; i++ {
		b.appendToFile(testFile, config.Message{
			Text:      "second",
			Timestamp: secondBatchTime.Add(time.Second * time.Duration(i)),
		})
	}

	l := b.tailLog(testFile, 10, offsetTime{
		to: secondBatchTime,
	})

	i := 0
	for e := l.Front(); e != nil; e = e.Next() {
		assert.Equal(t, config.Message{
			Text:      "hi",
			Timestamp: firstBatchTime.Add(time.Second * time.Duration(i)),
		}, e.Value, "element in the list matches expected")
		i++
	}

	assert.Equal(t, 10, l.Len(), "there should be 10 elements in list")
}

func TestTailLogOutOfBound(t *testing.T) {
	testFile := "test.json"
	os.Remove("test.db")
	db, err := bolt.Open("test.db", 0600, nil)
	assert.NoError(t, err)
	b := &Bdisk{db: db}

	firstBatchTime := time.Date(2000, 1, 1, 0, 0, 0, 0, time.Local)
	for i := 0; i < 10; i++ {
		b.appendToFile(testFile, config.Message{
			Text:      "hi",
			Timestamp: firstBatchTime.Add(time.Second * time.Duration(i)),
		})
	}

	secondBatchTime := time.Date(2000, 1, 2, 0, 0, 0, 0, time.Local)
	for i := 0; i < 10; i++ {
		b.appendToFile(testFile, config.Message{
			Text:      "second",
			Timestamp: secondBatchTime.Add(time.Second * time.Duration(i)),
		})
	}

	l := b.tailLog(testFile, 30, offsetTime{to: secondBatchTime})

	i := 0
	for e := l.Front(); e != nil; e = e.Next() {
		assert.Equal(t, config.Message{
			Text:      "hi",
			Timestamp: firstBatchTime.Add(time.Second * time.Duration(i)),
		}, e.Value, "element in the list matches expected")
		i++
	}

	assert.Equal(t, 10, l.Len(), "there should be 10 elements in list")
}

func TestTailLogWithOffsetFrom(t *testing.T) {
	testFile := "test.json"
	os.Remove("test.db")
	db, err := bolt.Open("test.db", 0600, nil)
	assert.NoError(t, err)
	b := &Bdisk{db: db}

	firstBatchTime := time.Date(2000, 1, 1, 0, 0, 0, 0, time.Local)
	for i := 0; i < 10; i++ {
		b.appendToFile(testFile, config.Message{
			Text:      "hi",
			Timestamp: firstBatchTime.Add(time.Second * time.Duration(i)),
		})
	}

	secondBatchTime := time.Date(2000, 1, 2, 0, 0, 0, 0, time.Local)
	for i := 0; i < 15; i++ {
		b.appendToFile(testFile, config.Message{
			Text:      "second",
			Timestamp: secondBatchTime.Add(time.Second * time.Duration(i)),
		})
	}

	l := b.tailLog(testFile, 0, offsetTime{from: firstBatchTime.Add(time.Second * 9)})

	i := 0
	for e := l.Front(); e != nil; e = e.Next() {
		assert.Equal(t, config.Message{
			Text:      "second",
			Timestamp: secondBatchTime.Add(time.Second * time.Duration(i)),
		}, e.Value, "element in the list matches expected")
		i++
	}

	assert.Equal(t, 15, l.Len(), "there should be 15 elements in list")
}

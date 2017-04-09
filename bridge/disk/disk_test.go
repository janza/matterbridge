package bdisk

import (
	"github.com/42wim/matterbridge/bridge/config"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"os"
	"testing"
	"time"
)

type TestStruct struct {
	Foo string
}

func TestStoreKeyValue(t *testing.T) {
	testFile := "test.json"
	os.Remove(testFile)
	b := &Bdisk{}
	b.StoreKeyValue(testFile, "key", TestStruct{
		Foo: "bar",
	})

	writtenData, err := ioutil.ReadFile(testFile)
	if err != nil {
		t.Errorf("%s File should exist", testFile)
	}

	assert.Equal(t,
		"{\"key\":{\"Foo\":\"bar\"}}",
		string(writtenData))
}

func TestStoreKeyValueUpdate(t *testing.T) {
	testFile := "test.json"
	os.Remove(testFile)
	b := &Bdisk{}
	b.StoreKeyValue(testFile, "key", TestStruct{
		Foo: "bar",
	})

	b.StoreKeyValue(testFile, "key", TestStruct{
		Foo: "bar2",
	})

	b.StoreKeyValue(testFile, "key2", TestStruct{
		Foo: "bar3",
	})

	writtenData, err := ioutil.ReadFile(testFile)
	if err != nil {
		t.Errorf("%s File should exist", testFile)
	}

	assert.Equal(t,
		"{\"key\":{\"Foo\":\"bar2\"},\"key2\":{\"Foo\":\"bar3\"}}",
		string(writtenData))
}

func TestReadKeyValue(t *testing.T) {
	testFile := "test.json"
	os.Remove(testFile)
	b := &Bdisk{}
	b.StoreKeyValue(testFile, "key", config.Channel{
		Channel: "channel",
		Origin:  "irc",
	})

	var contents ChannelMap
	err := b.ReadKeyValue(testFile, &contents)
	assert.Nil(t, err)

	assert.Equal(t, ChannelMap{"key": config.Channel{
		Channel: "channel",
		Origin:  "irc",
	}}, contents)
}

func TestAppendToFile(t *testing.T) {
	testFile := "test.json"
	os.Remove(testFile)
	b := &Bdisk{}
	b.AppendToFile(testFile, TestStruct{
		Foo: "bar",
	})

	writtenData, err := ioutil.ReadFile(testFile)
	if err != nil {
		t.Errorf("%s File should exist", testFile)
	}
	if string(writtenData) != "{\"Foo\":\"bar\"}\n" {
		t.Errorf("File should contain correct data: %s", writtenData)
	}
}

func TestAppendToFileMultiple(t *testing.T) {
	testFile := "test.json"
	os.Remove(testFile)
	b := &Bdisk{}
	b.AppendToFile(testFile, TestStruct{Foo: "bar"})
	b.AppendToFile(testFile, TestStruct{Foo: "bar2"})

	writtenData, err := ioutil.ReadFile(testFile)
	if err != nil {
		t.Errorf("%s File should exist", testFile)
	}
	if string(writtenData) != "{\"Foo\":\"bar\"}\n{\"Foo\":\"bar2\"}\n" {
		t.Errorf("File should contain correct data: %s", writtenData)
	}
}

func TestTailLog(t *testing.T) {
	testFile := "test.json"
	os.Remove(testFile)
	b := &Bdisk{}
	for i := 0; i < 10; i++ {
		b.AppendToFile(testFile, config.Message{Text: "hi"})
	}

	for i := 0; i < 10; i++ {
		b.AppendToFile(testFile, config.Message{})
	}

	l := b.TailLog(testFile, 10, offsetTime{to: time.Time{}})

	for e := l.Front(); e != nil; e = e.Next() {
		assert.Equal(t, config.Message{}, e.Value, "element in the list matches expected")
	}

	assert.Equal(t, 10, l.Len(), "there should be 10 elements in list")
}

func TestTailLogWithOffset(t *testing.T) {
	testFile := "test.json"
	os.Remove(testFile)
	b := &Bdisk{}

	firstBatchTime := time.Date(2000, 1, 1, 0, 0, 0, 0, time.Local)
	for i := 0; i < 10; i++ {
		b.AppendToFile(testFile, config.Message{Text: "hi", Timestamp: firstBatchTime})
	}

	secondBatchTime := time.Date(2000, 1, 2, 0, 0, 0, 0, time.Local)
	for i := 0; i < 10; i++ {
		b.AppendToFile(testFile, config.Message{Text: "second", Timestamp: secondBatchTime})
	}

	l := b.TailLog(testFile, 10, offsetTime{to: secondBatchTime})

	for e := l.Front(); e != nil; e = e.Next() {
		assert.Equal(t, config.Message{Text: "hi", Timestamp: firstBatchTime}, e.Value, "element in the list matches expected")
	}

	assert.Equal(t, 10, l.Len(), "there should be 10 elements in list")
}

func TestTailLogOutOfBound(t *testing.T) {
	testFile := "test.json"
	os.Remove(testFile)
	b := &Bdisk{}

	firstBatchTime := time.Date(2000, 1, 1, 0, 0, 0, 0, time.Local)
	for i := 0; i < 10; i++ {
		b.AppendToFile(testFile, config.Message{Text: "hi", Timestamp: firstBatchTime})
	}

	secondBatchTime := time.Date(2000, 1, 2, 0, 0, 0, 0, time.Local)
	for i := 0; i < 10; i++ {
		b.AppendToFile(testFile, config.Message{Timestamp: secondBatchTime})
	}

	l := b.TailLog(testFile, 30, offsetTime{to: secondBatchTime})

	for e := l.Front(); e != nil; e = e.Next() {
		assert.Equal(t, config.Message{Text: "hi", Timestamp: firstBatchTime}, e.Value, "element in the list matches expected")
	}

	assert.Equal(t, 10, l.Len(), "there should be 10 elements in list")
}

func TestTailLogWithOffsetFrom(t *testing.T) {
	testFile := "test.json"
	os.Remove(testFile)
	b := &Bdisk{}

	firstBatchTime := time.Date(2000, 1, 1, 0, 0, 0, 0, time.Local)
	for i := 0; i < 10; i++ {
		b.AppendToFile(testFile, config.Message{Text: "hi", Timestamp: firstBatchTime})
	}

	secondBatchTime := time.Date(2000, 1, 2, 0, 0, 0, 0, time.Local)
	for i := 0; i < 15; i++ {
		b.AppendToFile(testFile, config.Message{Text: "second", Timestamp: secondBatchTime})
	}

	l := b.TailLog(testFile, 0, offsetTime{from: secondBatchTime})

	for e := l.Front(); e != nil; e = e.Next() {
		assert.Equal(t, config.Message{Text: "second", Timestamp: secondBatchTime}, e.Value, "element in the list matches expected")
	}

	assert.Equal(t, 15, l.Len(), "there should be 15 elements in list")
}

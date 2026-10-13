package storage

import (
	"encoding/binary"
	"io"
	"os"
	"sync"
)

type (
	Appender interface {
		sync.Locker
		io.Closer
		Append(message Record) (startOffset int64, n int, err error)
	}

	fileAppender struct {
		mu   *sync.Mutex
		file *os.File
	}
)

func NewFileAppender(filename string) (Appender, error) {
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	appender := &fileAppender{
		mu:   &sync.Mutex{},
		file: f,
	}

	return appender, err
}

func (a *fileAppender) Append(record Record) (startOffset int64, n int, err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	startOffset, err = a.file.Seek(0, io.SeekEnd)
	if err != nil {
		return
	}

	var buf []byte
	header := newHeader(record)
	buf, err = binary.Append(buf, binary.BigEndian, &header)
	if err != nil {
		return
	}

	buf = append(buf, record...)
	n, err = a.file.Write(buf)
	return
}

func (a *fileAppender) Lock() {
	a.mu.Lock()
}

func (a *fileAppender) Unlock() {
	a.mu.Unlock()
}

func (a *fileAppender) Close() error {
	return a.file.Close()
}

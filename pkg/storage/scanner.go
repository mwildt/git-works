package storage

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"sync"

	"gtihub.com/mwildt/git-works/pkg/common"
)

type (
	LogCoordinate struct {
		offset int64
		whence int
	}

	ReplayConfiguration func(*replayConfiguration)

	Seeker interface {
		Next() LogCoordinate
	}

	Scanner interface {
		sync.Locker
		io.Closer
		Replay(configuration ...ReplayConfiguration) (err error)
	}

	fileScanner struct {
		mu   *sync.Mutex
		file *os.File
	}

	defaultSeeker struct{}

	offsetListSeeker struct {
		offsets []int64
	}

	replayConfiguration struct {
		startCoordinate LogCoordinate
		seeker          Seeker
		recordConsumer  []common.Consumer[OffsetRecord]
	}
)

func (s *defaultSeeker) Next() LogCoordinate {
	return LogCoordinate{0, io.SeekCurrent}
}

func (s *offsetListSeeker) Next() LogCoordinate {
	if len(s.offsets) > 0 {
		next := s.offsets[0]
		s.offsets = s.offsets[1:]
		return LogCoordinate{next, io.SeekStart}
	}
	return LogCoordinate{0, io.SeekEnd}
}

func StartingAt(offset int64, whence int) ReplayConfiguration {
	return func(conf *replayConfiguration) {
		conf.startCoordinate = LogCoordinate{offset, whence}
	}
}

func SeekingEvents(offsets []int64) ReplayConfiguration {
	return func(conf *replayConfiguration) {
		conf.seeker = &offsetListSeeker{offsets}
	}
}

func Consuming(consumer ...common.Consumer[OffsetRecord]) ReplayConfiguration {
	return func(conf *replayConfiguration) {
		conf.recordConsumer = append(conf.recordConsumer, consumer...)
	}
}

func NewFileScanner(filename string) (Scanner, error) {
	f, err := os.OpenFile(filename, os.O_RDONLY, 0644)
	return &fileScanner{
		&sync.Mutex{},
		f,
	}, err
}

func (s *fileScanner) Replay(configuration ...ReplayConfiguration) (err error) {
	config := replayConfiguration{
		startCoordinate: LogCoordinate{0, io.SeekStart},
		seeker:          &defaultSeeker{},
	}

	for _, conf := range configuration {
		conf(&config)
	}

	if _, err = s.file.Seek(config.startCoordinate.offset, config.startCoordinate.whence); err != nil {
		return
	}

	for {
		offset := config.seeker.Next()
		byteOffset, err := s.file.Seek(offset.offset, offset.whence)
		if err != nil {
			return err
		}
		header := recordHeader{}
		if err = binary.Read(s.file, binary.BigEndian, &header); err != nil {
			return err
		}
		record := make(Record, header.Length)
		if n, err := s.file.Read(record); err != nil {
			return fmt.Errorf("failed to read event data: %w", err)
		} else if n != int(header.Length) {
			return fmt.Errorf("expected to read %d bytes, but only read %d bytes", header.Length, n)
		} else if len(config.recordConsumer) > 0 {
			for _, consumer := range config.recordConsumer {
				if err = consumer(OffsetRecord{record, byteOffset}); err != nil {
					return err
				}
			}
		}
	}
}

func (s *fileScanner) Lock() {
	s.mu.Lock()
}

func (s *fileScanner) Unlock() {
	s.mu.Unlock()
}

func (s *fileScanner) Close() error {
	return s.file.Close()
}

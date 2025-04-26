package daemonize

import (
	"io"
)

type Logger interface {
	io.Writer
	io.Closer
	ReadLine(offset int64) (ss []string, err error)
	Lines() int64
}

func NewMemoryLogger() Logger {
	lines := make([]string, 0, 1024)
	return &memoryLogger{
		lines:    lines,
		maxLines: 1024,
	}
}

type memoryLogger struct {
	lines    []string
	maxLines int64
}

func (m *memoryLogger) Write(p []byte) (n int, err error) {
	line := string(p)
	if len(line) > 0 && line[len(line)-1] == '\n' {
		line = line[:len(line)-1]
	}
	m.lines = append(m.lines, line)
	if int64(len(m.lines)) > m.maxLines {
		m.lines = m.lines[1:]
	}
	return len(p), nil
}

func (m *memoryLogger) ReadLine(offset int64) (ss []string, err error) {
	if offset < 0 || offset >= int64(len(m.lines)) {
		return nil, io.EOF
	}
	if offset >= int64(len(m.lines)) {
		return nil, nil
	}
	ss = m.lines[offset:]
	m.lines = m.lines[:offset]
	return ss, nil
}

func (m *memoryLogger) Lines() int64 {
	return int64(len(m.lines))
}

func (m *memoryLogger) Close() error {
	return nil
}

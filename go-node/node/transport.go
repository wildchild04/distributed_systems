package node

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sync"
)

type Transport interface {
	Recv() ([]byte, error)
	Send(data []byte) error
	Close() error
}

type StdioTransport struct {
	scanner *bufio.Scanner
	mu      *sync.Mutex
	out     io.Writer
}

func NewStdioTransport() Transport {
	return &StdioTransport{
		scanner: bufio.NewScanner(os.Stdin),
		mu:      &sync.Mutex{},
		out:     os.Stdout,
	}
}

// Close implements [Transport].
func (s StdioTransport) Close() error {
	return nil
}

// Recv implements [Transport].
func (s StdioTransport) Recv() ([]byte, error) {
	if s.scanner.Scan() {
		data := make([]byte, len(s.scanner.Bytes()))
		copy(data, s.scanner.Bytes())
		return data, nil
	}

	return nil, fmt.Errorf("No data scanned")
}

// Send implements [Transport].
func (s StdioTransport) Send(data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.out.Write(data)
	if err != nil {
		return err
	}
	_, err = s.out.Write([]byte("\n"))

	return err
}

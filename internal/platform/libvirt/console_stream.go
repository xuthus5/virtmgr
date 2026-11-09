package libvirtbackend

import (
	"errors"
	"io"
	"sync"
	"sync/atomic"

	libvirt "libvirt.org/go/libvirt"
)

// domainConsoleStream adapts a libvirt character stream into the io.ReadWriteCloser
// expected by the WebSocket bridge. libvirt streams support one reader and one
// writer at a time, so read and write calls are serialized independently while
// Close aborts the stream to wake a blocked reader.
type domainConsoleStream struct {
	stream    *libvirt.Stream
	readMu    sync.Mutex
	writeMu   sync.Mutex
	closeOnce sync.Once
	closed    atomic.Bool
}

func newDomainConsoleStream(stream *libvirt.Stream) *domainConsoleStream {
	return &domainConsoleStream{stream: stream}
}

func (s *domainConsoleStream) Read(buffer []byte) (int, error) {
	s.readMu.Lock()
	defer s.readMu.Unlock()
	if s.closed.Load() {
		return 0, io.EOF
	}
	count, err := s.stream.Recv(buffer)
	if count == -2 || count == -3 {
		return 0, nil
	}
	if err != nil {
		if s.closed.Load() || errors.Is(err, io.EOF) {
			return 0, io.EOF
		}
		return 0, err
	}
	return count, nil
}

func (s *domainConsoleStream) Write(buffer []byte) (int, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if s.closed.Load() {
		return 0, io.ErrClosedPipe
	}
	written := 0
	for written < len(buffer) {
		count, err := s.stream.Send(buffer[written:])
		if count == -2 {
			continue
		}
		if count > 0 {
			written += count
		}
		if err != nil {
			return written, err
		}
		if count == 0 {
			return written, io.ErrShortWrite
		}
	}
	return written, nil
}

func (s *domainConsoleStream) Close() error {
	var closeErr error
	s.closeOnce.Do(func() {
		s.closed.Store(true)
		if err := s.stream.Abort(); err != nil {
			closeErr = err
		}
		s.readMu.Lock()
		s.readMu.Unlock()
		s.writeMu.Lock()
		s.writeMu.Unlock()
		if err := s.stream.Free(); err != nil && closeErr == nil {
			closeErr = err
		}
	})
	return closeErr
}

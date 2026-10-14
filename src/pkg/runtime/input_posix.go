//go:build !windows

package runtime

import (
	stderrors "errors"
	"io"
	"os"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/sys/unix"

	"github.com/Southclaws/sampctl/src/pkg/infrastructure/print"
)

func duplicateRuntimeFile(file *os.File) (int, error) {
	raw, err := file.SyscallConn()
	if err != nil {
		return -1, err
	}

	fd := -1
	var dupErr error
	err = raw.Control(func(original uintptr) {
		fd, dupErr = unix.FcntlInt(original, unix.F_DUPFD_CLOEXEC, 0)
	})
	if err != nil {
		return -1, err
	}
	return fd, dupErr
}

func pollableRuntimePty(terminal *os.File) (*os.File, error) {
	fd, err := duplicateRuntimeFile(terminal)
	if err != nil {
		return nil, err
	}

	// PTY setup can leave the descriptor blocking. Only this owned descriptor
	// may change modes; caller input must retain its flags and deadlines.
	if err = unix.SetNonblock(fd, true); err != nil {
		return nil, stderrors.Join(err, unix.Close(fd))
	}
	pollable := os.NewFile(uintptr(fd), terminal.Name())
	if err = pollable.SetDeadline(time.Time{}); err != nil {
		return nil, stderrors.Join(err, pollable.Close())
	}
	if err = terminal.Close(); err != nil {
		return nil, stderrors.Join(err, pollable.Close())
	}
	return pollable, nil
}

func startRuntimeFileInput(terminal, input *os.File) (func(), error) {
	fd, err := duplicateRuntimeFile(input)
	if err != nil {
		return nil, err
	}

	wakeReader, wakeWriter, err := os.Pipe()
	if err != nil {
		return nil, stderrors.Join(err, unix.Close(fd))
	}
	wakeFD := int(wakeReader.Fd())
	done := make(chan struct{})

	go func() {
		defer close(done)
		defer func() {
			if errClose := unix.Close(fd); errClose != nil {
				print.Verb("failed to close server input descriptor:", errClose)
			}
			if errClose := wakeReader.Close(); errClose != nil {
				print.Verb("failed to close server input wake pipe:", errClose)
			}
		}()

		errCopy := copyRuntimeFileInput(terminal, input, fd, wakeFD)
		if errCopy != nil && !isBenignPtyCopyError(errCopy) {
			print.Verb("failed to forward server input:", errCopy)
		}
	}()

	return func() {
		if errClose := wakeWriter.Close(); errClose != nil {
			print.Verb("failed to stop server input polling:", errClose)
		}
		if errDeadline := terminal.SetWriteDeadline(time.Now()); errDeadline != nil {
			print.Verb("failed to interrupt server input write:", errDeadline)
		}
		<-done
	}, nil
}

func copyRuntimeFileInput(terminal, input *os.File, inputFD, wakeFD int) error {
	fds := []unix.PollFd{
		{Fd: int32(inputFD), Events: unix.POLLIN},
		{Fd: int32(wakeFD), Events: unix.POLLIN},
	}
	buffer := make([]byte, 32*1024)

	for {
		_, err := unix.Poll(fds, -1)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return err
		}
		if fds[1].Revents != 0 {
			return nil
		}
		if fds[0].Revents == 0 {
			continue
		}

		// Execution has exclusive read access to input. Polling a duplicate
		// avoids changing the caller's blocking mode or read deadline.
		n, errRead := input.Read(buffer)
		if n > 0 {
			if _, err = terminal.Write(buffer[:n]); err != nil {
				if errors.Is(err, os.ErrDeadlineExceeded) {
					return nil
				}
				return err
			}
		}
		if errors.Is(errRead, unix.EINTR) || errors.Is(errRead, unix.EAGAIN) {
			continue
		}
		if errors.Is(errRead, io.EOF) {
			return nil
		}
		if errRead != nil {
			return errRead
		}
		if n == 0 {
			return nil
		}
	}
}

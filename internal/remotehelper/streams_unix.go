//go:build unix

package remotehelper

import (
	"os"
	"syscall"
)

// Inherited stdio descriptors may have been opened in blocking mode. Closing an
// os.File cannot interrupt a blocking syscall already in flight. Duplicate pipe
// descriptors and enroll them in Go's poller before starting the I/O goroutines.
func interruptibleStream(stream any) (any, error) {
	file, ok := stream.(*os.File)
	if !ok {
		return stream, nil
	}
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Mode()&(os.ModeNamedPipe|os.ModeSocket) == 0 {
		return stream, nil
	}
	fd, err := syscall.Dup(int(file.Fd()))
	if err != nil {
		return nil, err
	}
	syscall.CloseOnExec(fd)
	if err = syscall.SetNonblock(fd, true); err != nil {
		syscall.Close(fd)
		return nil, err
	}
	return os.NewFile(uintptr(fd), file.Name()), nil
}

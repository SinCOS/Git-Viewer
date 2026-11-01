package sshfs

import "sync"

// Authentication output may arrive while the SFTP request reports an error.
// Bound it and synchronize reads with exec's stderr writer.
type diagnosticOutput struct {
	mu   sync.Mutex
	data []byte
}

func (d *diagnosticOutput) Write(p []byte) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.data = append(d.data, p[:min(len(p), 64*1024-len(d.data))]...)
	return len(p), nil
}
func (d *diagnosticOutput) bytes() []byte {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]byte(nil), d.data...)
}

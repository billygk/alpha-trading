package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"sync"
)

// Rotator implements io.Writer and handles log file rotation based on size.
type Rotator struct {
	Filename   string
	MaxSize    int64 // Bytes
	MaxBackups int
	file       *os.File
	size       int64
	mu         sync.Mutex
}

// Setup initializes the standard logger to write to both stdout and a rotating file.
func Setup(filename string, maxSizeMB int64, maxBackups int) {
	rotator := &Rotator{
		Filename:   filename,
		MaxSize:    maxSizeMB * 1024 * 1024,
		MaxBackups: maxBackups,
	}

	if err := rotator.openExistingOrNew(); err != nil {
		log.Printf("Failed to open log file, using stdout only: %v", err)
		return
	}

	// MultiWriter writes to both stdout and the rotator
	mw := io.MultiWriter(os.Stdout, rotator)
	log.SetOutput(mw)
	// Remove default flags as we rely on our own formatting or system logs usually,
	// but user asked for robust logging with timestamps.
	// We'll keep standard flags (date/time) for now.
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func (r *Rotator) openExistingOrNew() error {
	info, err := os.Stat(r.Filename)
	if os.IsNotExist(err) {
		return r.openNew()
	}
	if err != nil {
		return err
	}

	// File exists, open it in append mode
	f, err := os.OpenFile(r.Filename, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	r.file = f
	r.size = info.Size()
	return nil
}

func (r *Rotator) openNew() error {
	f, err := os.OpenFile(r.Filename, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	r.file = f
	r.size = 0
	return nil
}

// Write satisfies the io.Writer interface. It checks size and rotates if needed.
func (r *Rotator) Write(p []byte) (n int, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	writeLen := int64(len(p))

	if r.file == nil {
		if err = r.openExistingOrNew(); err != nil {
			return 0, err
		}
	}

	if r.size+writeLen > r.MaxSize {
		if err := r.rotate(); err != nil {
			// If rotation fails, try to write anyway or return error?
			// Let's write anyway to avoid losing logs if possible, or return error.
			// Better to log internal error to stderr and try to write.
			fmt.Fprintf(os.Stderr, "Log rotation failed: %v\n", err)
		}
	}

	n, err = r.file.Write(p)
	r.size += int64(n)
	return n, err
}

// rotate closes the current file, renames backups, and opens a new file.
func (r *Rotator) rotate() error {
	if r.file != nil {
		r.file.Close()
	}

	// Rename old backups
	// Example: log.2 -> log.3, log.1 -> log.2, log -> log.1
	for i := r.MaxBackups - 1; i >= 1; i-- {
		oldPath := fmt.Sprintf("%s.%d", r.Filename, i)
		newPath := fmt.Sprintf("%s.%d", r.Filename, i+1)

		// If oldPath doesn't exist, skip
		if _, err := os.Stat(oldPath); os.IsNotExist(err) {
			continue
		}

		// If newPath exists, it will be overwritten
		os.Rename(oldPath, newPath)
	}

	// Rename current log to .1
	if _, err := os.Stat(r.Filename); err == nil {
		os.Rename(r.Filename, fmt.Sprintf("%s.1", r.Filename))
	}

	return r.openNew()
}

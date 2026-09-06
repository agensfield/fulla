package store

import (
	"bytes"
	"context"
	"io"
)

// os/exec serializes writes and joins its output copier before Run returns.
// Retain at most limit bytes and cancel the owned command on the first excess.
type gitOutputBuffer struct {
	buffer   bytes.Buffer
	limit    int
	cancel   context.CancelFunc
	exceeded bool
}

func (b *gitOutputBuffer) Bytes() []byte { return b.buffer.Bytes() }

func (b *gitOutputBuffer) Write(data []byte) (int, error) {
	remaining := b.limit - b.buffer.Len()
	if len(data) <= remaining {
		return b.buffer.Write(data)
	}
	b.exceeded = true
	b.cancel()
	return 0, io.ErrShortBuffer
}

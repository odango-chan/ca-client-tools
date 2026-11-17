package payload

import (
	"context"
	"errors"
	"io"
	"testing"
)

// scriptedRead is one scripted Read result.
type scriptedRead struct {
	data []byte
	err  error
}

// scriptedReader returns scripted Read results in order. Exhausted scripts
// report io.EOF, so probes terminate deterministically without timing.
type scriptedReader struct {
	steps []scriptedRead
	calls int
}

func (r *scriptedReader) Read(buffer []byte) (int, error) {
	index := r.calls
	r.calls++
	if index >= len(r.steps) {
		return 0, io.EOF
	}
	step := r.steps[index]
	n := copy(buffer, step.data)
	return n, step.err
}

// zeroProgressReader always reports (0, nil), emulating a reader that can make
// legal but endless no progress.
type zeroProgressReader struct {
	calls int
}

func (r *zeroProgressReader) Read([]byte) (int, error) {
	r.calls++
	return 0, nil
}

// cancelOnRead cancels the supplied context from inside Read, then reports
// (0, nil), so cancellation happens mid-probe rather than before it.
type cancelOnRead struct {
	cancel context.CancelFunc
}

func (r *cancelOnRead) Read([]byte) (int, error) {
	r.cancel()
	return 0, nil
}

func TestDecodeResponseKindNoneProbesAcrossNoProgressReads(t *testing.T) {
	// A single (0, nil) read is legal and does not prove the body is empty: a
	// later read may still deliver bytes, which must never be silently
	// discarded for an empty-body response.
	reader := &scriptedReader{steps: []scriptedRead{
		{nil, nil},
		{[]byte("x"), nil},
	}}
	if _, err := DecodeResponse(context.Background(), "", reader, ResponseSpec{Kind: KindNone}, nil, Limits{}); !errors.Is(err, ErrInvalidPayload) {
		t.Fatalf("expected bytes after a no-progress read to be rejected, got %v", err)
	}
	if reader.calls != 2 {
		t.Fatalf("expected the probe to read twice, got %d calls", reader.calls)
	}
}

func TestDecodeResponseKindNoneAcceptsImmediateEOF(t *testing.T) {
	reader := &scriptedReader{steps: []scriptedRead{{nil, io.EOF}}}
	if _, err := DecodeResponse(context.Background(), "", reader, ResponseSpec{Kind: KindNone}, nil, Limits{}); err != nil {
		t.Fatalf("immediate EOF must be accepted for KindNone, got %v", err)
	}
}

func TestDecodeResponseKindNoneRejectsFirstByte(t *testing.T) {
	reader := &scriptedReader{steps: []scriptedRead{{[]byte("x"), io.EOF}}}
	if _, err := DecodeResponse(context.Background(), "", reader, ResponseSpec{Kind: KindNone}, nil, Limits{}); !errors.Is(err, ErrInvalidPayload) {
		t.Fatalf("expected a byte in a KindNone body to be rejected, got %v", err)
	}
}

func TestDecodeResponseKindNonePropagatesReadError(t *testing.T) {
	readErr := errors.New("scripted read failure")
	reader := &scriptedReader{steps: []scriptedRead{{nil, readErr}}}
	_, err := DecodeResponse(context.Background(), "", reader, ResponseSpec{Kind: KindNone}, nil, Limits{})
	if !errors.Is(err, readErr) {
		t.Fatalf("expected the read error to be propagated, got %v", err)
	}
}

func TestDecodeResponseKindNoneFailsBoundedWithoutProgress(t *testing.T) {
	reader := &zeroProgressReader{}
	_, err := DecodeResponse(context.Background(), "", reader, ResponseSpec{Kind: KindNone}, nil, Limits{})
	if !errors.Is(err, ErrInvalidPayload) {
		t.Fatalf("expected endless no-progress reads to fail with ErrInvalidPayload, got %v", err)
	}
	if reader.calls == 0 || reader.calls > 64 {
		t.Fatalf("expected bounded no-progress probing, got %d calls", reader.calls)
	}
}

func TestDecodeResponseKindNonePropagatesCancellation(t *testing.T) {
	t.Run("cancelled before probe", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := DecodeResponse(ctx, "", &zeroProgressReader{}, ResponseSpec{Kind: KindNone}, nil, Limits{})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled before the probe, got %v", err)
		}
	})

	t.Run("cancelled during probe", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		_, err := DecodeResponse(ctx, "", &cancelOnRead{cancel: cancel}, ResponseSpec{Kind: KindNone}, nil, Limits{})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled from the probe, got %v", err)
		}
	})
}

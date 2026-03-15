package framing

import (
	"bytes"
	"encoding/binary"
	"testing"
)

type frameSample struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

func TestRoundTrip(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	want := frameSample{Value: "hello", Count: 7}

	if err := Write(&buf, want); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	var got frameSample
	if err := Read(&buf, &got); err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	if got != want {
		t.Fatalf("Read() = %#v, want %#v", got, want)
	}
}

func TestReadWithLimitRejectsLargeFrame(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	if err := binary.Write(&buf, binary.BigEndian, uint32(32)); err != nil {
		t.Fatalf("binary.Write() error = %v", err)
	}

	buf.Write(bytes.Repeat([]byte("x"), 32))

	var got frameSample
	if err := ReadWithLimit(&buf, &got, 8); err == nil {
		t.Fatal("ReadWithLimit() unexpectedly succeeded")
	}
}

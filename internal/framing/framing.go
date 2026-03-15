package framing

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const MaxPayloadSize = 1 << 20

func Write(w io.Writer, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}

	if len(payload) > MaxPayloadSize {
		return fmt.Errorf("frame too large: %d", len(payload))
	}

	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(payload)))

	if _, err := w.Write(header[:]); err != nil {
		return err
	}

	_, err = w.Write(payload)
	return err
}

func Read(r io.Reader, value any) error {
	return ReadWithLimit(r, value, MaxPayloadSize)
}

func ReadWithLimit(r io.Reader, value any, maxPayload uint32) error {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return err
	}

	size := binary.BigEndian.Uint32(header[:])
	if size > maxPayload {
		return fmt.Errorf("frame too large: %d", size)
	}

	payload := make([]byte, size)
	if _, err := io.ReadFull(r, payload); err != nil {
		return err
	}

	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(value); err != nil {
		return err
	}

	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("unexpected trailing JSON")
	}

	return nil
}

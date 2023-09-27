package utils

import (
	"crypto/rand"
	"encoding/base64"
	"io"

	"github.com/nats-io/nats.go"
)

var (
	correlationIDHeaderKey = "C"
	lastChunkHeaderKey     = "L"
)

func ChunkID(nm *nats.Msg) string {
	if nm.Header == nil {
		return ""
	}
	return nm.Header.Get(correlationIDHeaderKey)
}

func IsLastChunk(nm *nats.Msg) bool {
	if nm.Header == nil {
		return false
	}
	return nm.Header.Get(lastChunkHeaderKey) == lastChunkHeaderKey
}

func SplitIntoMsgs(payload []byte, subject string, lim int) ([]*nats.Msg, *nats.Msg) {
	chunks := split(payload, lim)
	correlationID := correlationID()
	var msgs []*nats.Msg
	for i, chunk := range chunks {
		msg := nats.NewMsg(subject)
		msg.Data = chunk
		msg.Header.Set(correlationIDHeaderKey, correlationID)
		if i == len(chunks)-1 { // last chunk indicator
			msg.Header.Set(lastChunkHeaderKey, lastChunkHeaderKey)
		}
		msgs = append(msgs, msg)
	}
	return msgs[:len(msgs)-1], msgs[len(msgs)-1]
}

func correlationID() string {
	var rndData [4]byte
	data := rndData[:]
	_, _ = io.ReadFull(rand.Reader, data)
	var encoded [6]byte
	base64.RawURLEncoding.Encode(encoded[:], data)
	return string(encoded[:])
}

func split(buf []byte, lim int) [][]byte {
	var chunk []byte
	chunks := make([][]byte, 0, len(buf)/lim+1)
	for len(buf) >= lim {
		chunk, buf = buf[:lim], buf[lim:]
		chunks = append(chunks, chunk)
	}
	if len(buf) > 0 {
		chunks = append(chunks, buf)
	}
	return chunks
}

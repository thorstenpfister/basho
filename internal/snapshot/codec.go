package snapshot

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/klauspost/compress/zstd"
)

// The zstd coders are safe for concurrent EncodeAll/DecodeAll and costly to
// create; the app only ever needs the decoder, so both are made on first use.
var (
	encoder = sync.OnceValue(func() *zstd.Encoder {
		e, _ := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedBestCompression), zstd.WithEncoderConcurrency(1))
		return e
	})
	decoder = sync.OnceValue(func() *zstd.Decoder {
		d, _ := zstd.NewReader(nil, zstd.WithDecoderConcurrency(1))
		return d
	})
)

// versioned is embedded in every file so a reader can refuse other schemas.
type versioned struct {
	V int `json:"v"`
}

func (v versioned) check() error {
	if v.V != Schema {
		return fmt.Errorf("schema %d, want %d", v.V, Schema)
	}
	return nil
}

func encode(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return encoder().EncodeAll(b, nil), nil
}

func decode(b []byte, v any) error {
	raw, err := decoder().DecodeAll(b, nil)
	if err != nil {
		return fmt.Errorf("zstd: %w", err)
	}
	return json.Unmarshal(raw, v)
}

package state

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
)

// ── gzip helpers (shared by baseline and pvc-usage) ────────────

const maxDecompressedJSONBytes = 8 << 20

func gzJSON(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(raw); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func gunzipJSON(b []byte, out any) error {
	zr, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return err
	}
	defer zr.Close()
	raw, err := io.ReadAll(io.LimitReader(zr, maxDecompressedJSONBytes+1))
	if err != nil {
		return err
	}
	if len(raw) > maxDecompressedJSONBytes {
		return fmt.Errorf("decompressed state exceeds %d bytes", maxDecompressedJSONBytes)
	}
	return json.Unmarshal(raw, out)
}

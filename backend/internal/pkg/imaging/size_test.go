package imaging

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/png"
	"testing"
)

// PNG 1×1 nhưng IHDR khai w×h: giải nén thật cần w*h*4 byte RAM.
func fakeSize(t *testing.T, w, h uint32) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewGray(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	b := buf.Bytes()
	binary.BigEndian.PutUint32(b[16:], w)
	binary.BigEndian.PutUint32(b[20:], h)
	binary.BigEndian.PutUint32(b[29:], crc32.ChecksumIEEE(b[12:29]))
	return b
}

func TestTooBigRejectedBeforeDecode(t *testing.T) {
	for _, c := range []struct{ w, h uint32 }{{50000, 50000}, {20000, 10}, {9000, 9000}} {
		b := fakeSize(t, c.w, c.h)
		if _, _, err := Probe(b); !errors.Is(err, ErrTooBig) {
			t.Errorf("Probe %dx%d: %v", c.w, c.h, err)
		}
		if _, _, _, err := Open(b); !errors.Is(err, ErrTooBig) {
			t.Errorf("Open %dx%d: %v", c.w, c.h, err)
		}
		if _, err := MaskJPEG(b, nil); !errors.Is(err, ErrTooBig) {
			t.Errorf("MaskJPEG %dx%d: %v", c.w, c.h, err)
		}
	}
	// ảnh thường vẫn mở được
	var buf bytes.Buffer
	_ = png.Encode(&buf, image.NewGray(image.Rect(0, 0, 4000, 3000)))
	if _, _, _, err := Open(buf.Bytes()); err != nil {
		t.Fatalf("ảnh 12MP: %v", err)
	}
	if _, _, err := Probe([]byte("không phải ảnh")); !errors.Is(err, ErrNotImage) {
		t.Fatalf("file rác: %v", err)
	}
}

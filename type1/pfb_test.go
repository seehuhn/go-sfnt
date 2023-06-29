package type1

import (
	"bytes"
	"io"
	"testing"
)

// TestPFB1 tests normal operation of a PDF decoder.
func TestPFB1(t *testing.T) {
	r := bytes.NewReader([]byte{
		0x80, 0x01, 0x02, 0x00, 0x00, 0x00, // ASCII section
		0x41, 0x42,
		0x80, 0x02, 0x01, 0x00, 0x00, 0x00, // binary section
		0xab,
		0x80, 0x03, // EOF
	})
	pfb := DecodePFB(r)
	data, err := io.ReadAll(pfb)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "ABab" {
		t.Errorf("wrong data: %q", data)
	}
}

// TestPDF2 tests that we can handle a PDF with a missing EOF marker.
func TestPFB2(t *testing.T) {
	r := bytes.NewReader([]byte{
		0x80, 0x01, 0x02, 0x00, 0x00, 0x00, // ASCII section
		0x43, 0x44,
		0x80, 0x02, 0x01, 0x00, 0x00, 0x00, // binary section
		0x01,
		// missing EOF
	})
	pfb := DecodePFB(r)
	data, err := io.ReadAll(pfb)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "CD01" {
		t.Errorf("wrong data: %q", data)
	}
}

// TestPDF3 tests that r.state == -1 works.
func TestPFB3(t *testing.T) {
	r := bytes.NewReader([]byte{
		0x80, 0x02, 0x02, 0x00, 0x00, 0x00, // binary section
		0x01, 0x23,
		0x80, 0x03, // EOF
	})
	pfb := DecodePFB(r)

	var buf [1]byte
	for i := '0'; i < '4'; i++ {
		n, err := pfb.Read(buf[:])
		if err != nil {
			t.Fatal(string(i), err)
		}
		if n != 1 {
			t.Fatalf("Read %c: n=%d, want 1", i, n)
		}
		if buf[0] != byte(i) {
			t.Fatalf("Read: buf=%q, want %q", buf[:n], i)
		}
	}
	n, err := pfb.Read(buf[:])
	if n != 0 || err != io.EOF {
		t.Fatalf("Read: n=%d, err=%v, want 0, EOF", n, err)
	}
}

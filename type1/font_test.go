package type1

import (
	"os"
	"testing"
)

func TestXXX(t *testing.T) {
	fname := "/Users/voss/project/pdf/type1/NimbusRoman-Regular.pfa"
	fd, err := os.Open(fname)
	if err != nil {
		t.Fatal(err)
	}
	defer fd.Close()

	font, err := Read(fd)
	if err != nil {
		t.Fatal(err)
	}

	_ = font
}

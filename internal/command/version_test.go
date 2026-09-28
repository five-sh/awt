package command

import (
	"bytes"
	"testing"
)

func TestPrintVersion(t *testing.T) {
	var buf bytes.Buffer
	if err := PrintVersion(&buf); err != nil {
		t.Fatal(err)
	}
	if got, want := buf.String(), "awt 0.2.0\n"; got != want {
		t.Errorf("PrintVersion = %q, want %q", got, want)
	}
}

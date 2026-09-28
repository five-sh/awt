package command

import (
	"fmt"
	"io"
)

// Version is awt's release version.
const Version = "0.2.0"

// PrintVersion writes the version line `awt version` prints.
func PrintVersion(w io.Writer) error {
	_, err := fmt.Fprintf(w, "awt %s\n", Version)
	return err
}

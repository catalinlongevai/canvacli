package output

import (
	"os"

	"golang.org/x/term"
)

func IsTTY(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}

func StdoutIsTTY() bool { return IsTTY(os.Stdout) }

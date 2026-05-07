package auth

import (
	"os/exec"
	"runtime"
)

// OpenBrowser opens the default browser at the given URL. Returns an error
// only if the command failed to spawn. Does not guarantee the user actually
// saw the page.
func OpenBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

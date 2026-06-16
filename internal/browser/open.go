package browser

import (
	"os/exec"
	"runtime"
)

// Open launches the system's default browser to url.
// Errors are silently ignored — browser open is best-effort.
func Open(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

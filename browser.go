package main

import (
	"fmt"
	"os/exec"
	"runtime"
)

func openBrowser(url string) error {
	return browserLauncher(url)
}

var browserLauncher = launchDefaultBrowser

func launchDefaultBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("could not open browser automatically: %w; open %s manually", err, url)
	}
	return nil
}

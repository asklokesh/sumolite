package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
)

const releaseBase = "https://github.com/asklokesh/sumolite/releases/latest/download"

func runUpdate() {
	asset := fmt.Sprintf("sumolite-%s-%s", runtime.GOOS, runtime.GOARCH)
	url := releaseBase + "/" + asset
	tmp := os.TempDir() + "/sumolite.new"

	fmt.Println("downloading", url)
	resp, err := http.Get(url)
	if err != nil {
		fail("download: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		fail("download: HTTP %d", resp.StatusCode)
	}
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		fail("open temp: %v", err)
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		fail("write: %v", err)
	}
	f.Close()

	self, err := os.Executable()
	if err != nil {
		fail("locate self: %v", err)
	}
	if err := os.Rename(tmp, self); err != nil {
		fail("install: %v (try `sudo sumolite update`)", err)
	}
	fmt.Println("installed. restarting...")
	cmd := exec.Command(self, "version")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	_ = cmd.Run()
}

func fail(f string, a ...any) {
	fmt.Fprintf(os.Stderr, "update failed: "+f+"\n", a...)
	os.Exit(1)
}

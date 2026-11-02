package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestVerifyRemote(t *testing.T) {
	for _, name := range []string{"ENDPOINT", "ACCESS_KEY", "SECRET_KEY"} {
		t.Setenv("SEAWEEDFS_S3_"+name, "secret-marker")
	}
	t.Setenv("SEAWEEDFS_CLOUD_IMAGES_BUCKET", "cloud-images")
	t.Setenv("PATH", "/test-path")
	digest := strings.Repeat("a", 64)
	calls := 0
	var output bytes.Buffer
	err := verifyRemote([]string{"ubuntu-24.04-base.img"}, &output, func(args []string, opt CommandOptions) (CommandResult, error) {
		calls++
		if args[0] != "rclone" || args[1] != "cat" || opt.Env["PATH"] != "/test-path" || opt.Env["RCLONE_CONFIG_SEAWEEDFS_SECRET_ACCESS_KEY"] != "secret-marker" {
			t.Fatal("CLI environment policy changed")
		}
		if opt.Digest && opt.Timeout != 4*time.Hour {
			t.Fatal("CLI image timeout changed")
		}
		return CommandResult{Output: []byte(digest + "\n"), Digest: digest}, nil
	})
	if err != nil || calls != 3 || !strings.Contains(output.String(), `"ok":true`) || strings.Contains(output.String(), "secret-marker") {
		t.Fatal("CLI verification output failed")
	}
}

func TestVerifyRemoteRejectsInvalidArguments(t *testing.T) {
	for _, args := range [][]string{nil, {"../image.img"}, {"image.img", "extra"}} {
		err := verifyRemote(args, &bytes.Buffer{}, func([]string, CommandOptions) (CommandResult, error) {
			t.Fatal("unexpected remote access")
			return CommandResult{}, errors.New("unused")
		})
		if err == nil {
			t.Fatal("accepted invalid arguments")
		}
	}
}

package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestVerifyRemote(t *testing.T) {
	digest := strings.Repeat("a", 64)
	other := strings.Repeat("b", 64)
	for _, tc := range []struct {
		name, before, after, image string
		failAt                     int
		ok                         bool
	}{
		{"valid", digest, digest, digest, 0, true},
		{"mismatch", digest, digest, other, 0, false},
		{"changed_sidecar", digest, other, digest, 0, false},
		{"invalid_sidecar", "secret-marker", digest, digest, 0, false},
		{"missing_sidecar", digest, digest, digest, 1, false},
		{"missing_image", digest, digest, digest, 2, false},
		{"failed_final_read", digest, digest, digest, 3, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, name := range []string{"ENDPOINT", "ACCESS_KEY", "SECRET_KEY"} {
				t.Setenv("SEAWEEDFS_S3_"+name, "secret-marker")
			}
			t.Setenv("SEAWEEDFS_CLOUD_IMAGES_BUCKET", "cloud-images")
			calls := 0
			invoke := func(args []string, opt CommandOptions) (CommandResult, error) {
				calls++
				if len(args) != 3 || args[0] != "rclone" || args[1] != "cat" || opt.Timeout <= 0 {
					t.Fatal("unexpected operation")
				}
				path := "seaweedfs:cloud-images/ubuntu-24.04-base.img"
				if calls != 2 {
					path += ".sha256"
				}
				if args[2] != path || opt.Digest != (calls == 2) || opt.Capture != (calls != 2) {
					t.Fatal("unexpected read configuration")
				}
				if calls == tc.failAt {
					return CommandResult{}, fail("command_failed")
				}
				value := tc.before
				if calls == 3 {
					value = tc.after
				}
				return CommandResult{Output: []byte(value + "\n"), Digest: tc.image}, nil
			}
			var output bytes.Buffer
			err := verifyRemote([]string{"ubuntu-24.04-base.img"}, &output, invoke)
			if (err == nil) != tc.ok || strings.Contains(output.String(), "secret-marker") {
				t.Fatal("unexpected verification result")
			}
			if tc.ok && (!strings.Contains(output.String(), `"ok":true`) || calls != 3) {
				t.Fatal("missing verification result")
			}
		})
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

package remoteimage

import (
	"strings"
	"testing"
	"time"

	"github.com/takanao14/homelab/scripts/imagebuilder/internal/command"
	"github.com/takanao14/homelab/scripts/imagebuilder/internal/protocol"
)

func TestRemoteImagePair(t *testing.T) {
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
			calls := 0
			invoke := func(args []string, opt command.Options) (command.Result, error) {
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
					return command.Result{}, protocol.Fail("command_failed")
				}
				value := tc.before
				if calls == 3 {
					value = tc.after
				}
				return command.Result{Output: []byte(value + "\n"), Digest: tc.image}, nil
			}
			actual, err := VerifyPair("seaweedfs:cloud-images/ubuntu-24.04-base.img", map[string]string{"PATH": "/test"}, 4*time.Hour, invoke)
			if (err == nil) != tc.ok || strings.Contains(protocol.SafeCode(err), "secret-marker") {
				t.Fatal("unexpected verification result")
			}
			if tc.ok && (actual != digest || calls != 3) {
				t.Fatal("missing verification result")
			}
		})
	}
}

func TestRemoteImagePathRejectsInvalidNames(t *testing.T) {
	for _, pair := range [][2]string{{"../bucket", "valid.img"}, {"cloud-images", "../image.img"}, {"bucket:other", "valid.img"}} {
		if _, err := Path(pair[0], pair[1]); err == nil {
			t.Fatal("invalid remote path accepted")
		}
	}
}

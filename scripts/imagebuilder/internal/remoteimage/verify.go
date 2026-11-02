package remoteimage

import (
	"encoding/json"
	"io"
	"os"
	"time"

	"github.com/takanao14/homelab/scripts/imagebuilder/internal/command"
	"github.com/takanao14/homelab/scripts/imagebuilder/internal/protocol"
)

func Verify(args []string, output io.Writer, invoke command.Func) error {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		_, err := io.WriteString(output, "Usage: verify-s3.sh IMAGE.img\nRead-only verification; streams the entire remote image (up to 4 hours). Do not run during publication.\nRequires rclone and SEAWEEDFS_S3_ENDPOINT/ACCESS_KEY/SECRET_KEY.\n")
		return err
	}
	if len(args) != 1 || !protocol.ImagePattern.MatchString(args[0]) {
		return protocol.Fail("invalid_arguments")
	}
	remote, err := Path(os.Getenv("SEAWEEDFS_CLOUD_IMAGES_BUCKET"), args[0])
	if err != nil {
		return err
	}
	env := protocol.BaseEnv()
	if path := os.Getenv("PATH"); path != "" {
		env["PATH"] = path
	}
	for _, name := range []string{"ENDPOINT", "ACCESS_KEY", "SECRET_KEY"} {
		env["SEAWEEDFS_S3_"+name] = os.Getenv("SEAWEEDFS_S3_" + name)
	}
	env, err = Environment(env)
	if err != nil {
		return err
	}
	digest, err := VerifyPair(remote, env, 4*time.Hour, invoke)
	if err != nil {
		return err
	}
	return json.NewEncoder(output).Encode(protocol.Object{"ok": true, "image": args[0], "sha256": digest})
}

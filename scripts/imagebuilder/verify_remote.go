package main

import (
	"encoding/json"
	"io"
	"os"
	"time"
)

func verifyRemote(args []string, output io.Writer, invoke CommandFunc) error {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		_, err := io.WriteString(output, "Usage: verify-s3.sh IMAGE.img\nRead-only verification; streams the entire remote image (up to 4 hours). Do not run during publication.\nRequires rclone and SEAWEEDFS_S3_ENDPOINT/ACCESS_KEY/SECRET_KEY.\n")
		return err
	}
	if len(args) != 1 || !workerImagePattern.MatchString(args[0]) {
		return fail("invalid_arguments")
	}
	remote, err := remoteImagePath(os.Getenv("SEAWEEDFS_CLOUD_IMAGES_BUCKET"), args[0])
	if err != nil {
		return err
	}
	env := baseEnv()
	if path := os.Getenv("PATH"); path != "" {
		env["PATH"] = path
	}
	for _, name := range []string{"ENDPOINT", "ACCESS_KEY", "SECRET_KEY"} {
		env["SEAWEEDFS_S3_"+name] = os.Getenv("SEAWEEDFS_S3_" + name)
	}
	env, err = remoteEnvironment(env)
	if err != nil {
		return err
	}
	digest, err := verifyRemotePair(remote, env, 4*time.Hour, invoke)
	if err != nil {
		return err
	}
	return json.NewEncoder(output).Encode(Object{"ok": true, "image": args[0], "sha256": digest})
}

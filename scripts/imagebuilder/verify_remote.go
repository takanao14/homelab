package main

import (
	"encoding/json"
	"io"
	"os"
	"regexp"
	"strings"
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
	bucket := os.Getenv("SEAWEEDFS_CLOUD_IMAGES_BUCKET")
	if bucket == "" {
		bucket = "cloud-images"
	}
	if !regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`).MatchString(bucket) {
		return fail("invalid_arguments")
	}
	env := baseEnv()
	if path := os.Getenv("PATH"); path != "" {
		env["PATH"] = path
	}
	for _, name := range []string{"ENDPOINT", "ACCESS_KEY", "SECRET_KEY"} {
		env["SEAWEEDFS_S3_"+name] = os.Getenv("SEAWEEDFS_S3_" + name)
	}
	env, err := workerRemoteEnvironment(env)
	if err != nil {
		return err
	}
	remote := "seaweedfs:" + bucket + "/" + args[0]
	readChecksum := func() (string, error) {
		result, err := invoke([]string{"rclone", "cat", remote + ".sha256"}, CommandOptions{Env: env, Capture: true, Timeout: time.Minute})
		if err != nil {
			return "", err
		}
		value := strings.TrimSpace(string(result.Output))
		if !shaPattern.MatchString(value) {
			return "", fail("invalid_artifact")
		}
		return value, nil
	}
	before, err := readChecksum()
	if err != nil {
		return err
	}
	image, err := invoke([]string{"rclone", "cat", remote}, CommandOptions{Env: env, Digest: true, Timeout: 4 * time.Hour})
	if err != nil {
		return err
	}
	after, err := readChecksum()
	if err != nil {
		return err
	}
	if before != after || image.Digest != before {
		return fail("remote_checksum_mismatch")
	}
	return json.NewEncoder(output).Encode(Object{"ok": true, "image": args[0], "sha256": before})
}

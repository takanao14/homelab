package remoteimage

import (
	"regexp"
	"strings"
	"time"

	"github.com/takanao14/homelab/scripts/imagebuilder/internal/command"
	"github.com/takanao14/homelab/scripts/imagebuilder/internal/protocol"
)

func Environment(env map[string]string) (map[string]string, error) {
	result := protocol.CopyEnv(env)
	result["RCLONE_CONFIG_SEAWEEDFS_TYPE"] = "s3"
	result["RCLONE_CONFIG_SEAWEEDFS_PROVIDER"] = "Other"
	result["RCLONE_CONFIG_SEAWEEDFS_REGION"] = "us-east-1"
	result["RCLONE_CONFIG_SEAWEEDFS_NO_CHECK_BUCKET"] = "true"
	result["RCLONE_CONFIG_SEAWEEDFS_USE_MULTIPART_UPLOADS"] = "false"
	for destination, source := range map[string]string{"ENDPOINT": "ENDPOINT", "ACCESS_KEY_ID": "ACCESS_KEY", "SECRET_ACCESS_KEY": "SECRET_KEY"} {
		value := env["SEAWEEDFS_S3_"+source]
		if value == "" {
			return nil, protocol.Fail("missing_credentials")
		}
		result["RCLONE_CONFIG_SEAWEEDFS_"+destination] = value
	}
	return result, nil
}

var BucketPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)

func Path(bucket, image string) (string, error) {
	if bucket == "" {
		bucket = "cloud-images"
	}
	if !BucketPattern.MatchString(bucket) || !protocol.ImagePattern.MatchString(image) {
		return "", protocol.Fail("invalid_arguments")
	}
	return "seaweedfs:" + bucket + "/" + image, nil
}

func readRemoteChecksum(path string, env map[string]string, invoke command.Func) (string, error) {
	result, err := invoke([]string{"rclone", "cat", path + ".sha256"}, command.Options{Env: env, Capture: true, Timeout: time.Minute})
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(string(result.Output))
	if !protocol.SHAPattern.MatchString(value) {
		return "", protocol.Fail("invalid_artifact")
	}
	return value, nil
}

func remoteImageDigest(path string, env map[string]string, timeout time.Duration, invoke command.Func) (string, error) {
	result, err := invoke([]string{"rclone", "cat", path}, command.Options{Env: env, Digest: true, Timeout: timeout})
	if err != nil {
		return "", err
	}
	if !protocol.SHAPattern.MatchString(result.Digest) {
		return "", protocol.Fail("invalid_command_output")
	}
	return result.Digest, nil
}

func VerifyPair(path string, env map[string]string, timeout time.Duration, invoke command.Func) (string, error) {
	before, err := readRemoteChecksum(path, env, invoke)
	if err != nil {
		return "", err
	}
	digest, err := remoteImageDigest(path, env, timeout, invoke)
	if err != nil {
		return "", err
	}
	after, err := readRemoteChecksum(path, env, invoke)
	if err != nil {
		return "", err
	}
	if before != after || digest != before {
		return "", protocol.Fail("remote_checksum_mismatch")
	}
	return digest, nil
}

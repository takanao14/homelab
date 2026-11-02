package main

import (
	"regexp"
	"strings"
	"time"
)

func remoteEnvironment(env map[string]string) (map[string]string, error) {
	result := copyEnv(env)
	result["RCLONE_CONFIG_SEAWEEDFS_TYPE"] = "s3"
	result["RCLONE_CONFIG_SEAWEEDFS_PROVIDER"] = "Other"
	result["RCLONE_CONFIG_SEAWEEDFS_REGION"] = "us-east-1"
	result["RCLONE_CONFIG_SEAWEEDFS_NO_CHECK_BUCKET"] = "true"
	result["RCLONE_CONFIG_SEAWEEDFS_USE_MULTIPART_UPLOADS"] = "false"
	for destination, source := range map[string]string{"ENDPOINT": "ENDPOINT", "ACCESS_KEY_ID": "ACCESS_KEY", "SECRET_ACCESS_KEY": "SECRET_KEY"} {
		value := env["SEAWEEDFS_S3_"+source]
		if value == "" {
			return nil, fail("missing_credentials")
		}
		result["RCLONE_CONFIG_SEAWEEDFS_"+destination] = value
	}
	return result, nil
}

var remoteBucketPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)

func remoteImagePath(bucket, image string) (string, error) {
	if bucket == "" {
		bucket = "cloud-images"
	}
	if !remoteBucketPattern.MatchString(bucket) || !workerImagePattern.MatchString(image) {
		return "", fail("invalid_arguments")
	}
	return "seaweedfs:" + bucket + "/" + image, nil
}

func readRemoteChecksum(path string, env map[string]string, invoke CommandFunc) (string, error) {
	result, err := invoke([]string{"rclone", "cat", path + ".sha256"}, CommandOptions{Env: env, Capture: true, Timeout: time.Minute})
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(string(result.Output))
	if !shaPattern.MatchString(value) {
		return "", fail("invalid_artifact")
	}
	return value, nil
}

func remoteImageDigest(path string, env map[string]string, timeout time.Duration, invoke CommandFunc) (string, error) {
	result, err := invoke([]string{"rclone", "cat", path}, CommandOptions{Env: env, Digest: true, Timeout: timeout})
	if err != nil {
		return "", err
	}
	if !shaPattern.MatchString(result.Digest) {
		return "", fail("invalid_command_output")
	}
	return result.Digest, nil
}

func verifyRemotePair(path string, env map[string]string, timeout time.Duration, invoke CommandFunc) (string, error) {
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
		return "", fail("remote_checksum_mismatch")
	}
	return digest, nil
}

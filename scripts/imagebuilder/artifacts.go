package main

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"
)

var workerImagePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]+\.img$`)

type workerArtifact struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
}

func workerFileDigest(path string) (string, error) {
	file, err := openNoFollow(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return "", fail("invalid_artifact")
	}
	defer file.Close()
	if info, err := file.Stat(); err != nil || !info.Mode().IsRegular() {
		return "", fail("invalid_artifact")
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fail("invalid_artifact")
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func workerReadSmallFile(path string, maximum int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maximum {
		return nil, fail("invalid_artifact")
	}
	file, err := openNoFollow(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, fail("invalid_artifact")
	}
	defer file.Close()
	if info, err := file.Stat(); err != nil || !info.Mode().IsRegular() {
		return nil, fail("invalid_artifact")
	}
	data, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil || int64(len(data)) > maximum {
		return nil, fail("invalid_artifact")
	}
	return data, nil
}

func (r *Runtime) workerArtifacts(checkout string) ([]workerArtifact, error) {
	r.CurrentStep = "artifact-check"
	images, err := filepath.Glob(filepath.Join(checkout, "packer", "images", "*.img"))
	if err != nil {
		return nil, fail("invalid_artifact")
	}
	sort.Strings(images)
	result := make([]workerArtifact, 0, len(images))
	for _, image := range images {
		info, err := os.Lstat(image)
		if err != nil || !info.Mode().IsRegular() || !workerImagePattern.MatchString(filepath.Base(image)) {
			return nil, fail("invalid_artifact")
		}
		digest, err := workerFileDigest(image)
		if err != nil {
			return nil, err
		}
		sidecar, err := workerReadSmallFile(image+".sha256", 65)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(string(sidecar)) != digest {
			return nil, fail("checksum_mismatch")
		}
		if _, err := r.Invoke([]string{"qemu-img", "check", "-f", "qcow2", image}, CommandOptions{Timeout: 5 * time.Minute}); err != nil {
			return nil, err
		}
		result = append(result, workerArtifact{Name: filepath.Base(image), SHA256: digest})
	}
	if len(result) == 0 {
		return nil, fail("no_artifacts")
	}
	return result, nil
}

func workerManifest(value any) ([]workerArtifact, error) {
	items, ok := value.([]any)
	if !ok || len(items) == 0 {
		return nil, fail("invalid_artifact")
	}
	result := make([]workerArtifact, 0, len(items))
	previous := ""
	for _, item := range items {
		entry, ok := item.(Object)
		if !ok || len(entry) != 2 || !keysAllowed(entry, "name", "sha256") ||
			!workerImagePattern.MatchString(s(entry, "name")) || !shaPattern.MatchString(s(entry, "sha256")) ||
			s(entry, "name") <= previous {
			return nil, fail("invalid_artifact")
		}
		previous = s(entry, "name")
		result = append(result, workerArtifact{Name: previous, SHA256: s(entry, "sha256")})
	}
	return result, nil
}

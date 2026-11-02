package builder

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/takanao14/homelab/scripts/imagebuilder/internal/command"
	"github.com/takanao14/homelab/scripts/imagebuilder/internal/protocol"
	"github.com/takanao14/homelab/scripts/imagebuilder/internal/store"
)

type imageArtifact struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
}

func fileDigest(path string) (string, error) {
	file, err := store.OpenNoFollow(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return "", protocol.Fail("invalid_artifact")
	}
	defer file.Close()
	if info, err := file.Stat(); err != nil || !info.Mode().IsRegular() {
		return "", protocol.Fail("invalid_artifact")
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", protocol.Fail("invalid_artifact")
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func readSmallFile(path string, maximum int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maximum {
		return nil, protocol.Fail("invalid_artifact")
	}
	file, err := store.OpenNoFollow(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, protocol.Fail("invalid_artifact")
	}
	defer file.Close()
	if info, err := file.Stat(); err != nil || !info.Mode().IsRegular() {
		return nil, protocol.Fail("invalid_artifact")
	}
	data, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil || int64(len(data)) > maximum {
		return nil, protocol.Fail("invalid_artifact")
	}
	return data, nil
}

func (r *Runtime) scanArtifacts(checkout string) ([]imageArtifact, error) {
	r.CurrentStep = "artifact-check"
	images, err := filepath.Glob(filepath.Join(checkout, "packer", "images", "*.img"))
	if err != nil {
		return nil, protocol.Fail("invalid_artifact")
	}
	sort.Strings(images)
	result := make([]imageArtifact, 0, len(images))
	for _, image := range images {
		info, err := os.Lstat(image)
		if err != nil || !info.Mode().IsRegular() || !protocol.ImagePattern.MatchString(filepath.Base(image)) {
			return nil, protocol.Fail("invalid_artifact")
		}
		digest, err := fileDigest(image)
		if err != nil {
			return nil, err
		}
		sidecar, err := readSmallFile(image+".sha256", 65)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(string(sidecar)) != digest {
			return nil, protocol.Fail("checksum_mismatch")
		}
		if _, err := r.Invoke([]string{"qemu-img", "check", "-f", "qcow2", image}, command.Options{Timeout: 5 * time.Minute}); err != nil {
			return nil, err
		}
		result = append(result, imageArtifact{Name: filepath.Base(image), SHA256: digest})
	}
	if len(result) == 0 {
		return nil, protocol.Fail("no_artifacts")
	}
	return result, nil
}

func parseManifest(value any) ([]imageArtifact, error) {
	items, ok := value.([]any)
	if !ok || len(items) == 0 {
		return nil, protocol.Fail("invalid_artifact")
	}
	result := make([]imageArtifact, 0, len(items))
	previous := ""
	for _, item := range items {
		entry, ok := item.(protocol.Object)
		if !ok || len(entry) != 2 || !protocol.KeysAllowed(entry, "name", "sha256") ||
			!protocol.ImagePattern.MatchString(protocol.String(entry, "name")) || !protocol.SHAPattern.MatchString(protocol.String(entry, "sha256")) ||
			protocol.String(entry, "name") <= previous {
			return nil, protocol.Fail("invalid_artifact")
		}
		previous = protocol.String(entry, "name")
		result = append(result, imageArtifact{Name: previous, SHA256: protocol.String(entry, "sha256")})
	}
	return result, nil
}

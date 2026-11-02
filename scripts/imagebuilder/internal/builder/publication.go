package builder

import (
	"path/filepath"

	"github.com/takanao14/homelab/scripts/imagebuilder/internal/command"
	"github.com/takanao14/homelab/scripts/imagebuilder/internal/protocol"
	"github.com/takanao14/homelab/scripts/imagebuilder/internal/remoteimage"
)

func (r *Runtime) publishArtifacts(checkout, target string, manifest []imageArtifact, env map[string]string) error {
	r.CurrentStep = "publish"
	remoteEnv, err := remoteimage.Environment(env)
	if err != nil {
		return err
	}
	timeout, err := r.stepTimeout(0)
	if err != nil {
		return err
	}
	if _, err := r.Invoke([]string{"bash", "./push.sh", target}, command.Options{Dir: filepath.Join(checkout, "packer"), Env: env, Timeout: timeout}); err != nil {
		return err
	}
	for _, artifact := range manifest {
		remote, err := remoteimage.Path(env["SEAWEEDFS_CLOUD_IMAGES_BUCKET"], artifact.Name)
		if err != nil {
			return err
		}
		timeout, err := r.stepTimeout(0)
		if err != nil {
			return err
		}
		digest, err := remoteimage.VerifyPair(remote, remoteEnv, timeout, r.Invoke)
		if err != nil {
			if protocol.SafeCode(err) == "invalid_artifact" {
				return protocol.Fail("remote_checksum_mismatch")
			}
			return err
		}
		if digest != artifact.SHA256 {
			return protocol.Fail("remote_checksum_mismatch")
		}
	}
	return nil
}

package main

import (
	"path/filepath"
)

func (r *Runtime) workerPublish(checkout, target string, manifest []workerArtifact, env map[string]string) error {
	r.CurrentStep = "publish"
	remoteEnv, err := remoteEnvironment(env)
	if err != nil {
		return err
	}
	timeout, err := r.stepTimeout(0)
	if err != nil {
		return err
	}
	if _, err := r.Invoke([]string{"bash", "./push.sh", target}, CommandOptions{Dir: filepath.Join(checkout, "packer"), Env: env, Timeout: timeout}); err != nil {
		return err
	}
	for _, artifact := range manifest {
		remote, err := remoteImagePath(env["SEAWEEDFS_CLOUD_IMAGES_BUCKET"], artifact.Name)
		if err != nil {
			return err
		}
		timeout, err := r.stepTimeout(0)
		if err != nil {
			return err
		}
		digest, err := verifyRemotePair(remote, remoteEnv, timeout, r.Invoke)
		if err != nil {
			if safeCode(err) == "invalid_artifact" {
				return fail("remote_checksum_mismatch")
			}
			return err
		}
		if digest != artifact.SHA256 {
			return fail("remote_checksum_mismatch")
		}
	}
	return nil
}

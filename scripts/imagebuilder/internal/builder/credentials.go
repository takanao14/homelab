package builder

import (
	"regexp"
	"strings"

	"github.com/takanao14/homelab/scripts/imagebuilder/internal/protocol"
	"github.com/takanao14/homelab/scripts/imagebuilder/internal/remoteimage"
)

var credentialSet = protocol.Set("PKR_VAR_user_password", "SEAWEEDFS_S3_ENDPOINT", "SEAWEEDFS_S3_ACCESS_KEY", "SEAWEEDFS_S3_SECRET_KEY", "SEAWEEDFS_CLOUD_IMAGES_BUCKET")

var endpointPattern = regexp.MustCompile(`^https://[A-Za-z0-9.-]+(?::[0-9]+)?/?$`)

func credentials(value any, publish bool) (map[string]string, error) {
	o, ok := value.(map[string]any)
	if !ok {
		return nil, protocol.Fail("invalid_request")
	}
	r := map[string]string{}
	for k, v := range o {
		x, ok := v.(string)
		if !credentialSet[k] || !ok || len(x) == 0 || len(x) > 16384 || strings.ContainsRune(x, 0) {
			return nil, protocol.Fail("invalid_request")
		}
		r[k] = x
	}
	if publish {
		for _, k := range []string{"SEAWEEDFS_S3_ENDPOINT", "SEAWEEDFS_S3_ACCESS_KEY", "SEAWEEDFS_S3_SECRET_KEY"} {
			if r[k] == "" {
				return nil, protocol.Fail("missing_credentials")
			}
		}
		if !endpointPattern.MatchString(r["SEAWEEDFS_S3_ENDPOINT"]) {
			return nil, protocol.Fail("invalid_request")
		}
		bucket := r["SEAWEEDFS_CLOUD_IMAGES_BUCKET"]
		if bucket == "" {
			bucket = "cloud-images"
		}
		if !remoteimage.BucketPattern.MatchString(bucket) {
			return nil, protocol.Fail("invalid_request")
		}
	}
	return r, nil
}

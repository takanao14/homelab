package protocol

import "regexp"

var LogStepSet = Set("checkout", "ssh-setup", "build", "packer-init", "packer-build", "import", "compress", "artifact-check", "smoke-boot", "publish")

var logEventSet = Set("started", "completed", "failed", "packer-error", "artifact-count")

var logCategorySet = Set("command-exit", "timeout", "packer-error", "hcl-validation", "ssh-timeout", "provisioner-exit", "hcl-unsupported-argument", "hcl-missing-required-argument", "hcl-unsupported-attribute", "hcl-invalid-expression", "hcl-unknown-variable", "hcl-invalid-reference", "hcl-invalid-function-argument")

var LogSourcePattern = regexp.MustCompile(`^packer/(?:[A-Za-z0-9_-][A-Za-z0-9._-]*\.pkr\.hcl|vars/[A-Za-z0-9_-][A-Za-z0-9._-]*\.pkrvars\.hcl)$`)

func ValidateLogRecord(o Object) (Object, error) {
	if !LogStepSet[String(o, "step")] || !logEventSet[String(o, "event")] {
		return nil, Fail("invalid_state")
	}
	r := Object{"step": String(o, "step"), "event": String(o, "event")}
	for _, k := range []string{"sequence", "time"} {
		v, ok := Int(o[k])
		if !ok || v < 0 || (k == "sequence" && v == 0) {
			return nil, Fail("invalid_state")
		}
		r[k] = v
	}
	for _, k := range []string{"exit_code", "duration_ms", "count", "provisioner_exit_code", "line", "column"} {
		if raw, exists := o[k]; exists {
			v, ok := Int(raw)
			if !ok || (k == "exit_code" && (v < -255 || v > 255)) || (k != "exit_code" && v < 0) || (k == "provisioner_exit_code" && v > 255) || ((k == "line" || k == "column") && v == 0) {
				return nil, Fail("invalid_state")
			}
			r[k] = v
		}
	}
	if _, exists := o["category"]; exists {
		if !logCategorySet[String(o, "category")] {
			return nil, Fail("invalid_state")
		}
		r["category"] = String(o, "category")
	}
	if _, exists := o["source"]; exists {
		src := String(o, "source")
		if len(src) > 512 || !LogSourcePattern.MatchString(src) || IntField(r, "line") == 0 {
			return nil, Fail("invalid_state")
		}
		r["source"] = src
	}
	if (IntField(r, "line") != 0 || IntField(r, "column") != 0) && String(r, "source") == "" {
		return nil, Fail("invalid_state")
	}
	return r, nil
}

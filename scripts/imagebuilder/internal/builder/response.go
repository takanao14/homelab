package builder

import "github.com/takanao14/homelab/scripts/imagebuilder/internal/protocol"

// PublicJobStatus cannot carry credentials or private control state.
type PublicJobStatus struct {
	JobID    string `json:"job_id"`
	Revision string `json:"revision"`
	Target   string `json:"target"`
	Branch   string `json:"branch,omitempty"`
	JobResult
	Publish  bool  `json:"publish"`
	Created  int64 `json:"created"`
	Finished bool  `json:"finished"`
}

func publicJobStatus(id string, record JobRecord, result JobResult) protocol.Object {
	status := PublicJobStatus{JobID: id, Revision: record.Revision, Target: record.Target, Branch: record.Branch,
		JobResult: result, Publish: record.Publish, Created: record.Created, Finished: record.Finished}
	return status.Object()
}

func (status PublicJobStatus) Object() protocol.Object {
	data := status.JobResult.Object()
	data["ok"], data["job_id"], data["revision"], data["target"] = true, status.JobID, status.Revision, status.Target
	data["publish"], data["created"], data["finished"] = status.Publish, status.Created, status.Finished
	if status.Branch != "" {
		data["branch"] = status.Branch
	}
	return data
}

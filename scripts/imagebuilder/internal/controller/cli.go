package controller

import (
	"bytes"
	"io"

	"github.com/alecthomas/kong"
	"github.com/takanao14/homelab/scripts/imagebuilder/internal/protocol"
)

type options struct {
	command, target, revision, branch, jobID, enabled string
	publish, detach, follow, keepRunning              bool
	after                                             int64
}

type commandLine struct {
	Run struct {
		Target   string `arg:"" help:"Build target."`
		Revision string `xor:"source" required:"" help:"Full repository commit SHA."`
		Branch   string `xor:"source" required:"" help:"Branch to resolve to a commit SHA."`
		Publish  bool   `help:"Publish verified images to S3."`
		Detach   bool   `help:"Return after admission."`
	} `cmd:"" help:"Build and verify an image. Specify exactly one of --revision or --branch."`
	Status struct {
		JobID string `arg:"" optional:""`
	} `cmd:"" help:"Show builder or job status."`
	Logs struct {
		JobID  string `arg:""`
		Follow bool
		After  int64
	} `cmd:"" help:"Read safe job diagnostics."`
	PublishRetry struct {
		JobID  string `arg:""`
		Detach bool
	} `cmd:"" help:"Republish retained verified artifacts."`
	Start       struct{ KeepRunning bool } `cmd:"" help:"Start the builder."`
	Maintenance struct {
		Enabled string `arg:"" enum:"on,off"`
	} `cmd:"" help:"Enable or disable maintenance."`
	Shutdown struct{} `cmd:"" help:"Shut down an idle builder."`
}

func parseCLI(args []string, output io.Writer) (options, bool, error) {
	var cli commandLine
	var help bytes.Buffer
	helpRequested := false
	parser, err := kong.New(&cli, kong.Name("image-build.sh"), kong.Writers(&help, io.Discard), kong.Exit(func(int) { helpRequested = true }))
	if err != nil {
		return options{}, false, protocol.Fail("internal_error")
	}
	ctx, err := parser.Parse(args)
	if helpRequested {
		if _, err := io.Copy(output, &help); err != nil {
			return options{}, false, protocol.Fail("controller_failed")
		}
		return options{}, true, nil
	}
	if err != nil {
		return options{}, false, protocol.Fail("invalid_arguments")
	}
	r := options{}
	switch ctx.Command() {
	case "run <target>":
		r = options{command: "run", target: cli.Run.Target, revision: cli.Run.Revision, branch: cli.Run.Branch, publish: cli.Run.Publish, detach: cli.Run.Detach}
	case "status", "status <job-id>":
		r = options{command: "status", jobID: cli.Status.JobID}
	case "logs <job-id>":
		r = options{command: "logs", jobID: cli.Logs.JobID, follow: cli.Logs.Follow, after: cli.Logs.After}
	case "publish-retry <job-id>":
		r = options{command: "publish-retry", jobID: cli.PublishRetry.JobID, detach: cli.PublishRetry.Detach}
	case "start":
		r = options{command: "start", keepRunning: cli.Start.KeepRunning}
	case "maintenance <enabled>":
		r = options{command: "maintenance", enabled: cli.Maintenance.Enabled}
	case "shutdown":
		r.command = "shutdown"
	default:
		return r, false, protocol.Fail("invalid_arguments")
	}
	if r.command == "run" && r.revision == "" && r.branch == "" {
		return r, false, protocol.Fail("invalid_arguments")
	}
	for _, rule := range []struct {
		valid bool
		code  string
	}{
		{r.command != "run" || protocol.TargetPattern.MatchString(r.target), "invalid_target"},
		{r.revision == "" || protocol.RevisionPattern.MatchString(r.revision), "invalid_revision"},
		{r.branch == "" || validBranch(r.branch), "invalid_branch"},
		{r.jobID == "" && r.command != "logs" && r.command != "publish-retry" || protocol.ValidJobID(r.jobID), "invalid_job_id"},
	} {
		if !rule.valid {
			return r, false, protocol.Fail(rule.code)
		}
	}
	if r.after < 0 {
		return r, false, protocol.Fail("invalid_arguments")
	}
	return r, false, nil
}

func parseArgs(args []string) (options, error) {
	parsed, _, err := parseCLI(args, io.Discard)
	return parsed, err
}

// Main returns only fixed diagnostic errors; callers decide exit status.
func Main(args []string, environ map[string]string, out io.Writer) error {
	parsed, help, err := parseCLI(args, out)
	if err != nil || help {
		return err
	}
	control, err := newBuilderClient(environ, out)
	if err != nil {
		return err
	}
	result, err := control.execute(parsed)
	if err != nil {
		return err
	}
	return control.emit(result)
}

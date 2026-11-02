package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"log/syslog"
	"os"
	"time"

	"github.com/takanao14/homelab/scripts/imagebuilder/internal/builder"
	"github.com/takanao14/homelab/scripts/imagebuilder/internal/command"
	"github.com/takanao14/homelab/scripts/imagebuilder/internal/controller"
	"github.com/takanao14/homelab/scripts/imagebuilder/internal/protocol"
	"github.com/takanao14/homelab/scripts/imagebuilder/internal/remoteimage"
	"github.com/takanao14/homelab/scripts/imagebuilder/internal/store"
)

// invocation holds only allowlisted values so journal records stay free of request data.
type invocation struct{ command, op, jobID string }

var runtimeCommands = protocol.Set("control", "boot", "idle", "cache-prune", "run", "finish")
var controlOps = protocol.Set("resolve", "submit", "publish-retry", "ready", "status", "logs", "maintenance", "shutdown")

// Polling operations and idle ticks run every few seconds; only their failures are logged.
var quietOps = protocol.Set("resolve", "ready", "status", "logs")

func (inv invocation) log(logger *slog.Logger, elapsed time.Duration, err error) {
	if inv.command == "" || (err == nil && (inv.command == "idle" || quietOps[inv.op])) {
		return
	}
	attrs := []slog.Attr{slog.String("command", inv.command), slog.Int64("duration_ms", elapsed.Milliseconds())}
	if inv.op != "" {
		attrs = append(attrs, slog.String("op", inv.op))
	}
	if inv.jobID != "" {
		attrs = append(attrs, slog.String("job_id", inv.jobID))
	}
	if err != nil {
		attrs = append(attrs, slog.String("code", protocol.SafeCode(err)))
		logger.LogAttrs(context.Background(), slog.LevelError, "failed", attrs...)
		return
	}
	logger.LogAttrs(context.Background(), slog.LevelInfo, "completed", attrs...)
}

// journalLogger uses syslog because control stderr returns over SSH to the discarding controller.
func journalLogger(inv invocation) *slog.Logger {
	var output io.Writer = os.Stderr
	if inv.command != "" {
		if writer, err := syslog.New(syslog.LOG_INFO|syslog.LOG_DAEMON, "imagebuilder"); err == nil {
			output = writer
		}
	}
	return slog.New(slog.NewJSONHandler(output, nil))
}

func dispatch(args []string, input io.Reader, output io.Writer, inv *invocation) error {
	if len(args) == 0 {
		return protocol.Fail("invalid_arguments")
	}
	if runtimeCommands[args[0]] {
		inv.command = args[0]
		if len(args) == 2 && protocol.ValidJobID(args[1]) {
			inv.jobID = args[1]
		}
	}
	if args[0] == "verify-remote" {
		return remoteimage.Verify(args[1:], output, command.Run)
	}
	if args[0] == "controller" {
		env := map[string]string{}
		for _, line := range os.Environ() {
			for i, c := range line {
				if c == '=' {
					env[line[:i]] = line[i+1:]
					break
				}
			}
		}
		return controller.Main(args[1:], env, output)
	}
	cfg, e := builder.LoadConfig("/etc/imagebuilder/config.json")
	if e != nil {
		return e
	}
	r, e := builder.NewRuntime(cfg)
	if e != nil {
		return e
	}
	if len(args) == 1 {
		switch args[0] {
		case "control":
			raw, e := io.ReadAll(io.LimitReader(input, 262145))
			if e != nil || len(raw) > 262144 {
				return protocol.Fail("invalid_request")
			}
			req, e := store.DecodeObject(raw)
			if e != nil {
				return protocol.Fail("invalid_request")
			}
			if op := protocol.String(req, "op"); controlOps[op] {
				inv.op = op
			}
			if id := protocol.String(req, "job_id"); protocol.ValidJobID(id) {
				inv.jobID = id
			}
			response, e := r.Control(req)
			if e != nil {
				return e
			}
			if id := protocol.String(response, "job_id"); protocol.ValidJobID(id) {
				inv.jobID = id
			}
			if json.NewEncoder(output).Encode(response) != nil {
				return protocol.Fail("internal_error")
			}
			return nil
		case "boot":
			return r.Boot()
		case "idle":
			return r.Idle()
		case "cache-prune":
			return r.PruneCache()
		}
	}
	if len(args) == 2 && protocol.ValidJobID(args[1]) {
		switch args[0] {
		case "run":
			return r.Run(args[1])
		case "finish":
			return r.Finish(args[1])
		}
	}
	return protocol.Fail("invalid_request")
}

func main() {
	started := time.Now()
	var inv invocation
	e := dispatch(os.Args[1:], os.Stdin, os.Stdout, &inv)
	inv.log(journalLogger(inv), time.Since(started), e)
	if e != nil {
		_ = json.NewEncoder(os.Stdout).Encode(protocol.Object{"ok": false, "error": protocol.SafeCode(e)})
		os.Exit(1)
	}
}

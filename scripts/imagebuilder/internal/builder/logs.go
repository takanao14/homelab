package builder

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/takanao14/homelab/scripts/imagebuilder/internal/command"
	"github.com/takanao14/homelab/scripts/imagebuilder/internal/protocol"
	"github.com/takanao14/homelab/scripts/imagebuilder/internal/store"
)

const maxLogRecords = 1000

const maxLogBytes = 196608

var hclHeaders = map[string]string{"Error: Unsupported argument": "hcl-unsupported-argument", "Error: Missing required argument": "hcl-missing-required-argument", "Error: Unsupported attribute": "hcl-unsupported-attribute", "Error: Invalid expression": "hcl-invalid-expression", "Error: Unknown variable": "hcl-unknown-variable", "Error: Invalid reference": "hcl-invalid-reference", "Error: Invalid function argument": "hcl-invalid-function-argument"}

var markerPattern = regexp.MustCompile(`^IMAGEBUILDER_STEP=(packer-init|packer-build|compress)$`)

var uiPrefixPattern = regexp.MustCompile(`^(?:==> )?qemu\.custom(?: \(shell-local\))?:\s*`)

var sourceLocationPattern = regexp.MustCompile(`^on ([A-Za-z0-9_./-]+\.pkr(?:vars)?\.hcl) line ([0-9]{1,9})(?:, column ([0-9]{1,9}))?(?:[,:].*)?$`)

var csvSequencePattern = regexp.MustCompile(`^[0-9]+$`)

var artifactCountPattern = regexp.MustCompile(`^[0-9]{1,5}$`)

var provisionExitPattern = regexp.MustCompile(`(?:^|: )Script exited with non-zero exit status: ([0-9]{1,3})(?:[. ]|$)`)

func (r *Runtime) knownSources(id string) map[string]string {
	root := filepath.Join(r.JobDir(id), "repo", "packer")
	result := map[string]string{}
	for _, pattern := range []string{"*.pkr.hcl", "vars/*.pkrvars.hcl"} {
		paths, _ := filepath.Glob(filepath.Join(root, pattern))
		for _, path := range paths {
			info, e := os.Lstat(path)
			if e != nil || !info.Mode().IsRegular() {
				continue
			}
			rel, e := filepath.Rel(root, path)
			if e == nil && protocol.LogSourcePattern.MatchString("packer/"+filepath.ToSlash(rel)) {
				result[filepath.ToSlash(rel)] = "packer/" + filepath.ToSlash(rel)
			}
		}
	}
	return result
}

func (r *Runtime) diagnostics(id string) (int64, []protocol.Object, error) {
	path := r.DiagnosticsPath(id)
	if _, e := os.Lstat(path); os.IsNotExist(e) {
		return 1, []protocol.Object{}, nil
	}
	data, e := store.ReadJSON(path)
	if e != nil {
		return 0, nil, e
	}
	next, ok := protocol.Int(data["next_sequence"])
	raw, arr := data["records"].([]any)
	if !ok || next <= 0 || !arr || len(raw) > maxLogRecords {
		return 0, nil, protocol.Fail("invalid_state")
	}
	rows := make([]protocol.Object, 0, len(raw))
	previous := int64(0)
	sources := r.knownSources(id)
	known := map[string]bool{}
	for _, v := range sources {
		known[v] = true
	}
	for _, v := range raw {
		o, ok := v.(map[string]any)
		if !ok {
			return 0, nil, protocol.Fail("invalid_state")
		}
		row, e := protocol.ValidateLogRecord(o)
		if e != nil {
			return 0, nil, e
		}
		seq := protocol.IntField(row, "sequence")
		if seq <= previous || seq >= next || (protocol.String(row, "source") != "" && !known[protocol.String(row, "source")]) {
			return 0, nil, protocol.Fail("invalid_state")
		}
		previous = seq
		rows = append(rows, row)
	}
	return next, rows, nil
}

func (r *Runtime) appendDiagnostic(id, step, event string, fields protocol.Object) error {
	next, rows, e := r.diagnostics(id)
	if e != nil {
		return e
	}
	if next == int64(^uint64(0)>>1) {
		return protocol.Fail("invalid_state")
	}
	o := protocol.Object{"step": step, "event": event, "sequence": next, "time": r.now().Unix()}
	for k, v := range fields {
		o[k] = v
	}
	row, e := protocol.ValidateLogRecord(o)
	if e != nil {
		return e
	}
	if src := protocol.String(row, "source"); src != "" {
		found := false
		for _, v := range r.knownSources(id) {
			found = found || v == src
		}
		if !found {
			return protocol.Fail("invalid_state")
		}
	}
	rows = append(rows, row)
	if len(rows) > maxLogRecords {
		rows = rows[len(rows)-maxLogRecords:]
	}
	data := protocol.Object{"next_sequence": next + 1, "records": rows}
	for {
		encoded, e := json.Marshal(data)
		if e != nil {
			return protocol.Fail("invalid_state")
		}
		if len(encoded) <= maxLogBytes {
			break
		}
		if len(rows) == 0 {
			return protocol.Fail("invalid_state")
		}
		rows = rows[1:]
		data["records"] = rows
	}
	return store.WriteJSON(r.DiagnosticsPath(id), data, 0600)
}

func (r *Runtime) DiagnosticPage(id string, after int64) (protocol.Object, error) {
	if after < 0 {
		return nil, protocol.Fail("invalid_request")
	}
	_, rows, e := r.diagnostics(id)
	if e != nil {
		return nil, e
	}
	page := []any{}
	cursor := after
	more := false
	for _, row := range rows {
		if protocol.IntField(row, "sequence") <= after {
			continue
		}
		if len(page) == 256 {
			more = true
			break
		}
		page = append(page, row)
		cursor = protocol.IntField(row, "sequence")
	}
	return protocol.Object{"records": page, "cursor": cursor, "has_more": more, "truncated": len(rows) > 0 && after < protocol.IntField(rows[0], "sequence")-1}, nil
}

type logContext struct {
	Step    string
	Substep bool
	Started time.Time
}

func (r *Runtime) diagnosticOutput(id string, ctx *logContext, raw []byte) error {
	if !utf8.Valid(raw) {
		return nil
	}
	text := strings.TrimSpace(string(raw))
	marker := markerPattern.FindStringSubmatch(text)
	message := ""
	if marker == nil {
		reader := csv.NewReader(bytes.NewReader([]byte(text)))
		reader.FieldsPerRecord = -1
		reader.LazyQuotes = true
		fields, e := reader.Read()
		if e == nil && len(fields) >= 4 && csvSequencePattern.MatchString(fields[0]) && (fields[1] == "" || fields[1] == "qemu.custom") {
			if fields[2] == "artifact-count" && len(fields) == 4 && artifactCountPattern.MatchString(fields[3]) {
				count, _ := strconv.ParseInt(fields[3], 10, 64)
				return r.appendDiagnostic(id, ctx.Step, "artifact-count", protocol.Object{"count": count})
			}
			if fields[2] == "ui" && len(fields) >= 5 {
				value := strings.Join(fields[4:], ",")
				for _, pair := range [][2]string{{"%!(PACKER_COMMA)", ","}, {`\n`, "\n"}, {`\r`, "\r"}, {"%!(PACKER_LF)", "\n"}, {"%!(PACKER_CR)", "\r"}} {
					value = strings.ReplaceAll(value, pair[0], pair[1])
				}
				value = uiPrefixPattern.ReplaceAllString(strings.TrimSpace(value), "")
				marker = markerPattern.FindStringSubmatch(value)
				if fields[3] == "error" {
					message = value
				}
			}
		}
		if message == "" && (hclHeaders[text] != "" || sourceLocationPattern.MatchString(text)) {
			message = text
		}
	}
	if marker != nil {
		now := time.Now()
		if ctx.Substep {
			if e := r.appendDiagnostic(id, ctx.Step, "completed", protocol.Object{"exit_code": 0, "duration_ms": now.Sub(ctx.Started).Milliseconds()}); e != nil {
				return e
			}
		}
		ctx.Step = marker[1]
		ctx.Started = now
		ctx.Substep = true
		return r.appendDiagnostic(id, ctx.Step, "started", nil)
	}
	if message == "" {
		return nil
	}
	fields := protocol.Object{"category": "packer-error"}
	sources := r.knownSources(id)
	for _, line := range strings.Split(message, "\n") {
		line = strings.TrimSpace(line)
		if category := hclHeaders[line]; category != "" {
			fields["category"] = category
		} else if line == "Timeout waiting for SSH." {
			fields["category"] = "ssh-timeout"
		}
		if m := provisionExitPattern.FindStringSubmatch(line); m != nil {
			v, _ := strconv.ParseInt(m[1], 10, 64)
			if v <= 255 {
				fields["category"] = "provisioner-exit"
				fields["provisioner_exit_code"] = v
			}
		}
		if m := sourceLocationPattern.FindStringSubmatch(line); m != nil {
			src := sources[m[1]]
			for _, v := range sources {
				if m[1] == v {
					src = v
				}
			}
			ln, _ := strconv.ParseInt(m[2], 10, 64)
			if src != "" && ln > 0 {
				fields["source"] = src
				fields["line"] = ln
				if m[3] != "" {
					col, _ := strconv.ParseInt(m[3], 10, 64)
					if col > 0 {
						fields["column"] = col
					}
				}
				if fields["category"] == "packer-error" {
					fields["category"] = "hcl-validation"
				}
			}
		}
	}
	return r.appendDiagnostic(id, ctx.Step, "packer-error", fields)
}

func (r *Runtime) Invoke(args []string, opt command.Options) (command.Result, error) {
	exec := r.Exec
	if exec == nil {
		exec = command.Run
	}
	if r.CurrentJob == "" || r.CurrentStep == "" || opt.Quiet {
		return exec(args, opt)
	}
	if !protocol.LogStepSet[r.CurrentStep] || !protocol.ValidJobID(r.CurrentJob) {
		return command.Result{}, protocol.Fail("invalid_state")
	}
	id, step := r.CurrentJob, r.CurrentStep
	ctx := &logContext{Step: step}
	external := opt.Observer
	opt.Observer = func(event command.Event) error {
		if external != nil {
			if e := external(event); e != nil {
				return e
			}
		}
		if event.Event == "output" {
			if !opt.Digest && protocol.Set("build", "packer-init", "packer-build", "compress")[step] {
				return r.diagnosticOutput(id, ctx, event.Line)
			}
			return nil
		}
		fields := protocol.Object{}
		if event.ExitCode != nil {
			fields["exit_code"] = *event.ExitCode
		}
		if event.Category != "" {
			fields["category"] = event.Category
		}
		if event.Event == "completed" || event.Event == "failed" {
			fields["duration_ms"] = event.DurationMS
			if ctx.Substep {
				sub := protocol.Object{}
				for k, v := range fields {
					sub[k] = v
				}
				sub["duration_ms"] = time.Since(ctx.Started).Milliseconds()
				if e := r.appendDiagnostic(id, ctx.Step, event.Event, sub); e != nil {
					return e
				}
			}
		}
		return r.appendDiagnostic(id, step, event.Event, fields)
	}
	return exec(args, opt)
}

package main

import (
	"encoding/json"
	"io"
	"os"

	"github.com/takanao14/homelab/scripts/imagebuilder/internal/builder"
	"github.com/takanao14/homelab/scripts/imagebuilder/internal/command"
	"github.com/takanao14/homelab/scripts/imagebuilder/internal/controller"
	"github.com/takanao14/homelab/scripts/imagebuilder/internal/protocol"
	"github.com/takanao14/homelab/scripts/imagebuilder/internal/remoteimage"
	"github.com/takanao14/homelab/scripts/imagebuilder/internal/store"
)

func dispatch(args []string, input io.Reader, output io.Writer) error {
	if len(args) == 0 {
		return protocol.Fail("invalid_arguments")
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
			response, e := r.Control(req)
			if e != nil {
				return e
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
	if e := dispatch(os.Args[1:], os.Stdin, os.Stdout); e != nil {
		_ = json.NewEncoder(os.Stdout).Encode(protocol.Object{"ok": false, "error": protocol.SafeCode(e)})
		os.Exit(1)
	}
}

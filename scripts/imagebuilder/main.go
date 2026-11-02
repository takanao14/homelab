package main

import (
	"encoding/json"
	"io"
	"os"
)

func dispatch(args []string, input io.Reader, output io.Writer) error {
	if len(args) == 0 {
		return fail("invalid_arguments")
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
		return ControllerMain(args[1:], env, output)
	}
	cfg, e := LoadConfig("/etc/imagebuilder/config.json")
	if e != nil {
		return e
	}
	r, e := NewRuntime(cfg)
	if e != nil {
		return e
	}
	if len(args) == 1 {
		switch args[0] {
		case "control":
			raw, e := io.ReadAll(io.LimitReader(input, 262145))
			if e != nil || len(raw) > 262144 {
				return fail("invalid_request")
			}
			req, e := decodeObject(raw)
			if e != nil {
				return fail("invalid_request")
			}
			response, e := r.Control(req)
			if e != nil {
				return e
			}
			if json.NewEncoder(output).Encode(response) != nil {
				return fail("internal_error")
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
	if len(args) == 2 && validJobID(args[1]) {
		switch args[0] {
		case "run":
			return r.Run(args[1])
		case "finish":
			return r.Finish(args[1])
		}
	}
	return fail("invalid_request")
}
func main() {
	if e := dispatch(os.Args[1:], os.Stdin, os.Stdout); e != nil {
		_ = json.NewEncoder(os.Stdout).Encode(Object{"ok": false, "error": safeCode(e)})
		os.Exit(1)
	}
}

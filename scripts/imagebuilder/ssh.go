package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"time"
)

type controllerSSH func(context.Context, []string, []string, []byte) ([]byte, int, error)

// HTTP responses and SSH output are decoded and projected onto the public protocol.
func controllerDecode(raw []byte) (map[string]any, error) {
	var data map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&data); err != nil || data == nil {
		return nil, fail("invalid_builder_response")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, fail("invalid_builder_response")
	}
	return data, nil
}

func controllerSSHCommand(ctx context.Context, args, environment []string, input []byte) ([]byte, int, error) {
	command := exec.CommandContext(ctx, args[0], args[1:]...)
	command.Env = environment
	command.Stdin = bytes.NewReader(input)
	output := &controllerLimitedBuffer{limit: 262145}
	command.Stdout = output
	command.Stderr = io.Discard
	err := command.Run()
	if len(output.data) > 262144 {
		return nil, 1, fail("invalid_builder_response")
	}
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return output.data, exit.ExitCode(), nil
		}
		return nil, 1, fail("builder_unreachable")
	}
	return output.data, 0, nil
}

type controllerLimitedBuffer struct {
	data  []byte
	limit int
}

func (b *controllerLimitedBuffer) Write(data []byte) (int, error) {
	size := len(data)
	if remaining := b.limit - len(b.data); remaining > 0 {
		if len(data) > remaining {
			data = data[:remaining]
		}
		b.data = append(b.data, data...)
	}
	return size, nil
}

func (c *imageController) remote(request map[string]any) (map[string]any, error) {
	args := []string{"ssh", "-o", "BatchMode=yes", "-o", "ForwardAgent=no", "-o", "SendEnv=-*", "-o", "ConnectTimeout=10", "-o", "StrictHostKeyChecking=yes", "-T", c.user + "@" + c.host, "sudo -n /usr/local/libexec/imagebuilder control"}
	environment := []string{}
	for _, key := range []string{"HOME", "PATH", "SSH_AUTH_SOCK"} {
		if value, exists := c.env[key]; exists {
			environment = append(environment, key+"="+value)
		}
	}
	input, err := json.Marshal(request)
	if err != nil {
		return nil, fail("invalid_arguments")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	raw, status, err := c.ssh(ctx, args, environment, input)
	if err != nil {
		if err.Error() == "invalid_builder_response" {
			return nil, fail("invalid_builder_response")
		}
		return nil, fail("builder_unreachable")
	}
	if len(raw) > 262144 {
		return nil, fail("invalid_builder_response")
	}
	data, err := controllerDecode(raw)
	if err != nil {
		return nil, fail("builder_unreachable")
	}
	safe, err := controllerSafeResponse(data)
	if err != nil {
		return nil, err
	}
	if status != 0 {
		return nil, fail("builder_request_failed")
	}
	return safe, nil
}

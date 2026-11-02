package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

func decodeValue(raw []byte) (any, error) {
	if len(raw) > 262144 {
		return nil, fail("invalid_state")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var v any
	if d.Decode(&v) != nil {
		return nil, fail("invalid_state")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return nil, fail("invalid_state")
	}
	return v, nil
}
func decodeObject(raw []byte) (Object, error) {
	v, e := decodeValue(raw)
	if e != nil {
		return nil, e
	}
	o, ok := v.(map[string]any)
	if !ok {
		return nil, fail("invalid_state")
	}
	return o, nil
}
func readValue(path string) (any, error) {
	info, e := os.Lstat(path)
	if e != nil || !info.Mode().IsRegular() || info.Size() > 262144 {
		return nil, fail("invalid_state")
	}
	f, e := openNoFollow(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if e != nil {
		return nil, fail("invalid_state")
	}
	defer f.Close()
	info, e = f.Stat()
	if e != nil || !info.Mode().IsRegular() || info.Size() > 262144 {
		return nil, fail("invalid_state")
	}
	raw, e := io.ReadAll(io.LimitReader(f, 262145))
	if e != nil {
		return nil, fail("invalid_state")
	}
	return decodeValue(raw)
}
func readJSON(path string) (Object, error) {
	v, e := readValue(path)
	if e != nil {
		return nil, e
	}
	o, ok := v.(map[string]any)
	if !ok {
		return nil, fail("invalid_state")
	}
	return o, nil
}
func writeJSON(path string, value any, mode os.FileMode) error {
	raw, e := json.Marshal(value)
	if e != nil {
		return fail("invalid_state")
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".imagebuilder-*.new")
	if e != nil {
		return fail("invalid_state")
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if f.Chmod(mode) != nil {
		f.Close()
		return fail("invalid_state")
	}
	if _, e = f.Write(raw); e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil || ce != nil {
		return fail("invalid_state")
	}
	if os.Rename(tmp, path) != nil {
		return fail("invalid_state")
	}
	return nil
}

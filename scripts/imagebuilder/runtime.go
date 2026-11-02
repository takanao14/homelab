package main

import (
	"os"
	"time"
)

type Runtime struct {
	Cfg                     Config
	UID, GID                int
	UserName, Home          string
	CurrentJob, CurrentStep string
	Exec                    CommandFunc
	Now                     func() time.Time
	Chown                   func(string, int, int) error
	ReadyHook               func() (Object, error)
	SmokeHook               func(string, string, map[string]string) error
}

func (r *Runtime) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}
func (r *Runtime) chown(path string, uid, gid int) error {
	if r.Chown != nil {
		return r.Chown(path, uid, gid)
	}
	return os.Chown(path, uid, gid)
}

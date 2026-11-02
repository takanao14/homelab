package protocol

import (
	"testing"
)

func TestProtocolUnknownErrorsAreValueFree(t *testing.T) {
	if SafeCode(Fail("SECRET_MARKER")) != "internal_error" {
		t.Fatal("unapproved error exposed")
	}
	if SafeCode(Fail("invalid_proxmox_ca")) != "invalid_proxmox_ca" {
		t.Fatal("approved error lost")
	}
}

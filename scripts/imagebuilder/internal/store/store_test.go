package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/takanao14/homelab/scripts/imagebuilder/internal/protocol"
)

func TestStorePreservesJSONIntegersAndRejectsNonIntegers(t *testing.T) {
	data, e := DecodeObject([]byte(`{"sequence":9223372036854775807,"enabled":true}`))
	if e != nil {
		t.Fatal("legacy JSON rejected")
	}
	v, ok := protocol.Int(data["sequence"])
	if !ok || v != 9223372036854775807 {
		t.Fatal("integer precision lost")
	}
	for _, raw := range []any{true, json.Number("1.0"), json.Number("1e2"), json.Number("9223372036854775808")} {
		if _, ok := protocol.Int(raw); ok {
			t.Fatal("invalid integer accepted")
		}
	}
}

func TestStoreRejectsSymlinkAndFIFOState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state")
	if os.Symlink("/etc/passwd", path) != nil {
		t.Fatal("fixture failed")
	}
	if _, e := ReadJSON(path); protocol.SafeCode(e) != "invalid_state" {
		t.Fatal("symlink followed")
	}
	if os.Remove(path) != nil || syscall.Mkfifo(path, 0600) != nil {
		t.Fatal("fixture failed")
	}
	if _, e := ReadJSON(path); protocol.SafeCode(e) != "invalid_state" {
		t.Fatal("FIFO accepted")
	}
}

func TestDecodeErrorsAreValueFree(t *testing.T) {
	if _, e := DecodeObject([]byte(`{"ok":true} SECRET_MARKER`)); e == nil || strings.Contains(e.Error(), "SECRET_MARKER") {
		t.Fatal("invalid input leaked")
	}
}

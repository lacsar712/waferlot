package mask_test

import (
	"bytes"
	"testing"

	"github.com/lacsar712/waferlot/internal/mask"
)

func TestJSONMasksSecrets(t *testing.T) {
	in := []byte(`{"lot_id":"LOT-1001","password":"hunter2","nested":{"token":"abc"}}`)
	out := mask.JSON(in)
	if bytes.Contains(out, []byte("hunter2")) || bytes.Contains(out, []byte("abc")) {
		t.Fatalf("secret leaked: %s", out)
	}
	if !bytes.Contains(out, []byte("LOT-1001")) {
		t.Fatalf("non-sensitive field missing: %s", out)
	}
}

func TestJSONMasksNestedToken(t *testing.T) {
	in := []byte(`{"payload":{"nested":{"token":"abc"}}}`)
	out := mask.JSON(in)
	if bytes.Contains(out, []byte("abc")) {
		t.Fatalf("nested token leaked: %s", out)
	}
}

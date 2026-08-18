package step_test

import (
	"testing"

	"github.com/lacsar712/waferlot/internal/step"
)

func TestFlowOrderAndPrograms(t *testing.T) {
	f := step.Seed()
	if !f.Allows("litho.photo", "PP-LITHO-193NM-V3") {
		t.Fatal("photo step must allow photo process program")
	}
	if f.Allows("litho.photo", "PP-ETCH-POLY-V2") {
		t.Fatal("photo step must not allow etch process program")
	}
	next, ok := f.Next("litho.photo")
	if !ok || next.ID != "etch.poly" {
		t.Fatalf("next after photo: %+v", next)
	}
	if err := f.ValidateMove("litho.photo", "cmp.oxide"); err == nil {
		t.Fatal("skipping etch must fail")
	}
}

package program_test

import (
	"testing"

	"github.com/lacsar712/waferlot/internal/program"
)

func TestUsableRejectsDraft(t *testing.T) {
	c := program.Seed()
	if err := c.Usable("PP-LITHO-193NM-V3"); err != nil {
		t.Fatal(err)
	}
	if err := c.Usable("PP-LITHO-DRAFT-V9"); err == nil {
		t.Fatal("draft process program must not be usable")
	}
	if !c.QualifiedOn("PP-LITHO-193NM-V3", "LITHO-01") {
		t.Fatal("expected qualified on LITHO-01")
	}
	if c.QualifiedOn("PP-LITHO-193NM-V3", "ETCH-07") {
		t.Fatal("photo process program is not qualified on etcher")
	}
}

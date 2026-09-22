package issue

import "testing"

func TestParseMarkedBlocks(t *testing.T) {
	blocks, err := Parse("```kwatch-resources\nkind: Pod\n```")
	if err != nil {
		t.Fatal(err)
	}
	if blocks.Resources != "kind: Pod" {
		t.Fatalf("unexpected resources block %q", blocks.Resources)
	}
}

func TestParseRejectsUnclosedBlock(t *testing.T) {
	if _, err := Parse("```kwatch-config\nfoo: bar"); err == nil {
		t.Fatal("expected unclosed block error")
	}
}

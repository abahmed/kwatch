package slack

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func TestNoteBlocksKeepFixWithinSectionLimit(t *testing.T) {
	blocks := noteBlocks(providertest.ProFix())
	body, err := json.Marshal(blocks.BlockSet[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "--limits=memory=384Mi") {
		t.Fatalf("the fix was cut from the note section: %s", body)
	}
}

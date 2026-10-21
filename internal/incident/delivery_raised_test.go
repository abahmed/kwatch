package incident

import "testing"

func TestDeliveryTierRaisedComparesWithThePreviousDecision(t *testing.T) {
	var d Delivery
	if d.TierRaised() {
		t.Fatal("nothing decided yet")
	}
	d.RecordSent(Notify, "n")
	if d.TierRaised() {
		t.Fatal("the first decision has nothing before it")
	}
	d.RecordSent(Page, "p")
	if !d.TierRaised() {
		t.Fatal("notify to page is a rise")
	}
	d.RecordSent(Page, "p")
	if d.TierRaised() {
		t.Fatal("the same tier is not a rise")
	}
}

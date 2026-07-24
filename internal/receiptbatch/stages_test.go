package receiptbatch

import "testing"

func TestNextStage(t *testing.T) {
	cases := map[string]string{
		StageExtract:  StageVendor,
		StageVendor:   StageClassify,
		StageClassify: StageDone,
		StageDone:     StageDone,
	}
	for from, want := range cases {
		if got := NextStage(from); got != want {
			t.Fatalf("NextStage(%q) = %q, want %q", from, got, want)
		}
	}
}

func TestKindForStage(t *testing.T) {
	if KindForStage(StageExtract) != KindExtract || KindForStage(StageClassify) != KindClassify {
		t.Fatal("AI stages must map to their kind")
	}
	if KindForStage(StageDone) != "" || KindForStage(StageReview) != "" {
		t.Fatal("non-AI stages have no kind")
	}
}

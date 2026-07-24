// Package receiptbatch wires the receipt pipeline into the generic aibatch framework: each AI stage
// (extract, vendor-match, classify-account) is an aibatch.Kind that gathers receipts pending at that
// stage, submits their requests as one provider batch, and on collect parses/gates the result and
// advances the receipt. build-entry (deterministic) runs inline when classify completes. This is the
// async/bulk path (PIPELINE_MODE=decomposed-batch); the synchronous worker path is unchanged.
package receiptbatch

// Batched-pipeline stages (receipt_pipeline_state.stage) and the aibatch kind names.
const (
	StageExtract  = "extract"
	StageVendor   = "vendor"
	StageClassify = "classify"
	StageDone     = "done"
	StageReview   = "review"

	KindExtract  = "receipt-extract"
	KindVendor   = "receipt-vendor"
	KindClassify = "receipt-classify"
)

// NextStage is the stage a receipt moves to after the given stage gates clean. classify is terminal
// (build-entry + draft happen inline in its apply) → done.
func NextStage(stage string) string {
	switch stage {
	case StageExtract:
		return StageVendor
	case StageVendor:
		return StageClassify
	case StageClassify:
		return StageDone
	default:
		return StageDone
	}
}

// KindForStage maps a stage to its aibatch kind name ("" for non-AI stages).
func KindForStage(stage string) string {
	switch stage {
	case StageExtract:
		return KindExtract
	case StageVendor:
		return KindVendor
	case StageClassify:
		return KindClassify
	default:
		return ""
	}
}

package main

import (
	"testing"
)

func TestImportUploadSlotsApplyBackpressure(t *testing.T) {
	api := NewImportAPI(nil).WithUploads(nil, 1)
	if !api.acquireUploadSlot() {
		t.Fatal("first upload slot was rejected")
	}
	if api.acquireUploadSlot() {
		t.Fatal("second concurrent upload slot was accepted")
	}
	api.releaseUploadSlot()
	if !api.acquireUploadSlot() {
		t.Fatal("released upload slot was not reusable")
	}
	api.releaseUploadSlot()
}

package main

import (
	"testing"

	"github.com/google/uuid"
)

func TestImportUploadSlotsApplyBackpressure(t *testing.T) {
	api := NewImportAPI(nil).WithUploads(nil, uuid.New(), 1)
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

package main

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/governance"
)

func TestImportInitiatorCanReadMachineProposal(t *testing.T) {
	wikiID, authorID, initiatorID := uuid.New(), uuid.New(), uuid.New()
	proposal := &governance.Proposal{WikiID: wikiID, CreatedBy: authorID, ImportInitiatedBy: &initiatorID}
	api := &GovernanceAPI{}
	if err := api.authorizeProposalRead(context.Background(), proposal, initiatorID, wikiID); err != nil {
		t.Fatalf("initiator without review/apply roles lost read access: %v", err)
	}
	if err := api.authorizeProposalRead(context.Background(), proposal, uuid.New(), wikiID); !errors.Is(err, governance.ErrPermissionDenied) {
		t.Fatalf("unrelated Actor gained import read access: %v", err)
	}
	if err := api.authorizeProposalRead(context.Background(), proposal, initiatorID, uuid.New()); !errors.Is(err, governance.ErrProposalNotFound) {
		t.Fatalf("initiator read across Wiki contexts: %v", err)
	}
}

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

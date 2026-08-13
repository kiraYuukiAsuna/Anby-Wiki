package knowledge

import (
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestValidateClaimEndpointsRejectsSelfReference(t *testing.T) {
	subjectID := uuid.New()
	targetID := subjectID
	if err := validateClaimEndpoints(subjectID, &targetID); !errors.Is(err, ErrSelfReferentialClaim) {
		t.Fatalf("error=%v, want ErrSelfReferentialClaim", err)
	}

	otherID := uuid.New()
	if err := validateClaimEndpoints(subjectID, &otherID); err != nil {
		t.Fatalf("different endpoint rejected: %v", err)
	}
	if err := validateClaimEndpoints(subjectID, nil); err != nil {
		t.Fatalf("scalar claim rejected: %v", err)
	}
}

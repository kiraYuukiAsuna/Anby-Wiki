package governance

import (
	"encoding/json"

	"github.com/anby/wiki/backend/internal/ast"
)

type resolutionPayload struct {
	Choice string `json:"choice"`
}

// applyConflictResolutions converts explicit user decisions into the
// operations that may be replayed against the supplied Current document.
func applyConflictResolutions(
	current *ast.Document,
	operations []OperationV1,
	conflicts []MergeConflict,
) ([]OperationV1, error) {
	byBlock := make(map[string]resolutionPayload)
	for _, conflict := range conflicts {
		if conflict.TargetBlockID == nil || conflict.Status == ConflictOpen ||
			len(conflict.Resolution) == 0 {
			continue
		}
		var resolution resolutionPayload
		if err := json.Unmarshal(conflict.Resolution, &resolution); err != nil {
			return nil, err
		}
		byBlock[*conflict.TargetBlockID] = resolution
	}
	result := make([]OperationV1, 0, len(operations))
	for _, operation := range operations {
		if operation.Target.BlockID == nil {
			result = append(result, operation)
			continue
		}
		resolution, ok := byBlock[*operation.Target.BlockID]
		if !ok {
			result = append(result, operation)
			continue
		}
		switch resolution.Choice {
		case ResolutionChooseCurrent, ResolutionDismiss:
			continue
		case ResolutionChooseProposed:
			location, found := ast.FindBlock(current, *operation.Target.BlockID)
			if !found {
				return nil, ErrPatchTargetNotFound
			}
			hash, err := BlockHash(location.Block)
			if err != nil {
				return nil, err
			}
			operation.ExpectedHash = &hash
			result = append(result, operation)
		default:
			return nil, ErrInvalidResolution
		}
	}
	return result, nil
}

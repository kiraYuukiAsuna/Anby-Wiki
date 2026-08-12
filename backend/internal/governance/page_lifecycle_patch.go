package governance

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/anby/wiki/backend/internal/page"
)

type createPageOperationPayload struct {
	Title        string          `json:"title"`
	Language     string          `json:"language"`
	ContentModel string          `json:"content_model"`
	InitialAST   json.RawMessage `json:"initial_ast"`
	Summary      string          `json:"summary"`
}

type renamePageOperationPayload struct {
	NewTitle string `json:"new_title"`
}

type createRedirectOperationPayload struct {
	TargetKind          string     `json:"target_kind"`
	TargetPageID        *uuid.UUID `json:"target_page_id"`
	TargetNamespaceID   *uuid.UUID `json:"target_namespace_id"`
	TargetTitle         string     `json:"target_title"`
	TargetAnchorBlockID *uuid.UUID `json:"target_anchor_block_id"`
	TargetInterwiki     string     `json:"target_interwiki"`
}

type pageLifecycleResult struct {
	PageID     *uuid.UUID
	RevisionID *uuid.UUID
}

func isPageLifecycleOperation(operationType string) bool {
	switch operationType {
	case OpCreatePage, OpRenamePage, OpCreateRedirect:
		return true
	default:
		return false
	}
}

func isPageContentOperation(operationType string) bool {
	switch operationType {
	case OpInsertBlock, OpDeleteBlock, OpMoveBlock, OpReplaceBlock,
		OpInsertPageReference, OpRetargetPageReference,
		OpInsertEntityReference, OpRetargetEntityReference,
		OpInsertClaimReference, OpRetargetClaimReference,
		OpInsertCitationReference, OpRetargetCitationReference,
		OpRetargetExternalLink:
		return true
	default:
		return false
	}
}

func decodeOperationPayload(raw json.RawMessage, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("%w: payload: %v", ErrInvalidOperation, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("%w: payload 包含多余 JSON 值", ErrInvalidOperation)
	}
	return nil
}

func (s *ApplyService) applyPageLifecycleInTx(
	ctx context.Context,
	tx pgx.Tx,
	proposal *Proposal,
	op OperationV1,
	actorID, changeBatchID uuid.UUID,
) (*pageLifecycleResult, error) {
	if s.pages == nil {
		return nil, fmt.Errorf("%w: Page 领域服务未装配", ErrUnsupportedOperation)
	}
	switch op.OperationType {
	case OpCreatePage:
		pageCreateProposal := proposal.TargetType == TargetPage && proposal.TargetID == nil
		wikiCreateProposal := proposal.TargetType == TargetWiki && proposal.TargetID != nil &&
			op.Target.WikiID != nil && *proposal.TargetID == *op.Target.WikiID
		if (!pageCreateProposal && !wikiCreateProposal) || op.Target.WikiID == nil ||
			op.Target.NamespaceID == nil || op.Target.PageID == nil || *op.Target.PageID == uuid.Nil {
			return nil, ErrInvalidOperation
		}
		var payload createPageOperationPayload
		if err := decodeOperationPayload(op.Payload, &payload); err != nil {
			return nil, err
		}
		if strings.TrimSpace(payload.Title) == "" {
			return nil, ErrInvalidOperation
		}
		if s.auth != nil {
			normalized, err := page.NormalizeTitle(payload.Title)
			if err != nil {
				return nil, err
			}
			if err := s.auth.CheckCreateTx(
				ctx, tx, actorID, *op.Target.WikiID, *op.Target.NamespaceID, normalized,
			); err != nil {
				return nil, err
			}
		}
		created, err := s.pages.CreatePageInTx(ctx, tx, page.CreatePageParams{
			PageID: *op.Target.PageID, WikiID: *op.Target.WikiID, NamespaceID: *op.Target.NamespaceID,
			Title: payload.Title, Language: payload.Language,
			ContentModel: payload.ContentModel, ActorID: actorID,
		}, &changeBatchID)
		if err != nil {
			return nil, err
		}
		result := &pageLifecycleResult{PageID: &created.ID}
		if len(payload.InitialAST) != 0 && string(payload.InitialAST) != "null" {
			revision, err := s.pages.PublishInTx(ctx, tx, page.PublishParams{
				PageID: created.ID, ActorID: actorID, AST: payload.InitialAST,
				Summary:       strings.TrimSpace(payload.Summary),
				ChangeBatchID: &changeBatchID,
			})
			if err != nil {
				return nil, err
			}
			result.RevisionID = &revision.ID
		}
		return result, nil

	case OpRenamePage:
		pageTarget := proposal.TargetType == TargetPage && proposal.TargetID != nil &&
			op.Target.PageID != nil && *op.Target.PageID == *proposal.TargetID
		wikiTarget := proposal.TargetType == TargetWiki && proposal.TargetID != nil && op.Target.PageID != nil
		if !pageTarget && !wikiTarget {
			return nil, ErrInvalidOperation
		}
		if wikiTarget {
			if err := s.requirePageInProposalWikiInTx(ctx, tx, proposal, *op.Target.PageID); err != nil {
				return nil, err
			}
		}
		var payload renamePageOperationPayload
		if err := decodeOperationPayload(op.Payload, &payload); err != nil {
			return nil, err
		}
		if s.auth != nil {
			if err := s.auth.CheckTx(
				ctx, tx, actorID, wikiIDForProposal(ctx, tx, s.repo, proposal),
				ActionRename, op.Target.PageID,
			); err != nil {
				return nil, err
			}
		}
		renamed, err := s.pages.RenamePageInTx(
			ctx, tx, *op.Target.PageID, payload.NewTitle, actorID, &changeBatchID,
		)
		if err != nil {
			return nil, err
		}
		return &pageLifecycleResult{PageID: &renamed.ID}, nil

	case OpCreateRedirect:
		pageTarget := proposal.TargetType == TargetPage && proposal.TargetID != nil &&
			op.Target.PageID != nil && *op.Target.PageID == *proposal.TargetID
		wikiTarget := proposal.TargetType == TargetWiki && proposal.TargetID != nil && op.Target.PageID != nil
		if !pageTarget && !wikiTarget {
			return nil, ErrInvalidOperation
		}
		if wikiTarget {
			if err := s.requirePageInProposalWikiInTx(ctx, tx, proposal, *op.Target.PageID); err != nil {
				return nil, err
			}
		}
		var payload createRedirectOperationPayload
		if err := decodeOperationPayload(op.Payload, &payload); err != nil {
			return nil, err
		}
		if s.auth != nil {
			if err := s.auth.CheckTx(
				ctx, tx, actorID, wikiIDForProposal(ctx, tx, s.repo, proposal),
				ActionEdit, op.Target.PageID,
			); err != nil {
				return nil, err
			}
		}
		_, err := s.pages.CreateRedirectInTx(
			ctx, tx, *op.Target.PageID, page.RedirectTarget{
				Kind: payload.TargetKind, PageID: payload.TargetPageID,
				NamespaceID:     payload.TargetNamespaceID,
				Title:           payload.TargetTitle,
				AnchorBlockID:   payload.TargetAnchorBlockID,
				TargetInterwiki: payload.TargetInterwiki,
			}, actorID, &changeBatchID,
		)
		if err != nil {
			return nil, err
		}
		return &pageLifecycleResult{PageID: op.Target.PageID}, nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedOperation, op.OperationType)
	}
}

func (s *ApplyService) requirePageInProposalWikiInTx(
	ctx context.Context,
	tx pgx.Tx,
	proposal *Proposal,
	pageID uuid.UUID,
) error {
	if proposal == nil || proposal.TargetType != TargetWiki ||
		proposal.TargetID == nil || *proposal.TargetID == uuid.Nil || pageID == uuid.Nil {
		return ErrInvalidOperation
	}
	targetPage, err := s.pages.GetPageInTx(ctx, tx, pageID)
	if err != nil {
		return err
	}
	if targetPage.WikiID != *proposal.TargetID || targetPage.DeletedAt != nil {
		return fmt.Errorf(
			"%w: page=%s 不属于 Proposal wiki=%s",
			ErrInvalidOperation, pageID, *proposal.TargetID,
		)
	}
	return nil
}

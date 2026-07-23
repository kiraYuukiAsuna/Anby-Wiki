package evidence

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/anby/wiki/backend/internal/platform/id"
)

// ExternalResourceService 是 external_resource 的系统级领域写入入口。
// Projection Builder 与后续健康检查任务属于 system 行为，不做 Actor 准入；
// 但仍必须经本服务统一执行 URL 规范化、状态校验和存储写入。
type ExternalResourceService struct {
	repo *Repository
	ids  *id.Generator
}

// NewExternalResourceService 装配 external_resource 系统服务。
func NewExternalResourceService(repo *Repository, ids *id.Generator) *ExternalResourceService {
	return &ExternalResourceService{repo: repo, ids: ids}
}

// UpsertInTx 规范化 URL 并在调用方事务内按 normalized_url 幂等 upsert。
// original_url 保留最早创建该资源时的输入值；冲突时返回既有行。
func (s *ExternalResourceService) UpsertInTx(ctx context.Context, tx pgx.Tx, rawURL string) (*ExternalResource, error) {
	normalized, err := NormalizeURL(rawURL)
	if err != nil {
		return nil, err
	}
	if existing, err := s.repo.GetExternalResourceByNormalizedURL(ctx, tx, normalized); err == nil {
		return existing, nil
	} else if !errors.Is(err, ErrExternalResourceNotFound) {
		return nil, err
	}

	u, err := url.Parse(normalized)
	if err != nil {
		return nil, fmt.Errorf("evidence: 解析规范化 URL 失败: %w", err)
	}
	resourceID, err := s.ids.New()
	if err != nil {
		return nil, err
	}
	resource := &ExternalResource{
		ID:            resourceID,
		OriginalURL:   rawURL,
		NormalizedURL: normalized,
		Domain:        strings.ToLower(u.Hostname()),
		Path:          u.Path,
		Status:        ExternalResourceStatusUnknown,
	}
	inserted, err := s.repo.InsertExternalResourceIfAbsent(ctx, tx, resource)
	if err != nil {
		return nil, err
	}
	if inserted {
		return resource, nil
	}
	return s.repo.GetExternalResourceByNormalizedURL(ctx, tx, normalized)
}

// Upsert 在自动提交事务中执行 UpsertInTx。
func (s *ExternalResourceService) Upsert(ctx context.Context, rawURL string) (*ExternalResource, error) {
	return s.UpsertInTx(ctx, nil, rawURL)
}

// UpdateExternalResourceStatusParams 是健康检查结果的可变字段。
// nil 可选字段表示把对应数据库列清为 NULL；Status 必填且须为合法枚举。
type UpdateExternalResourceStatusParams struct {
	Status           string
	HTTPStatus       *int32
	ContentHash      *string
	CanonicalURL     *string
	RedirectTargetID *uuid.UUID
}

// ExternalResourceCheckResult is one health probe result. The service derives
// retry state so callers cannot persist an unbounded or negative schedule.
type ExternalResourceCheckResult struct {
	Status           string
	HTTPStatus       *int32
	ContentHash      *string
	CanonicalURL     *string
	RedirectTargetID *uuid.UUID
	TransientFailure bool
}

const (
	defaultCheckLease   = 15 * time.Minute
	defaultCheckSuccess = 24 * time.Hour
	defaultShortRetry   = 5 * time.Minute
	defaultFailureBase  = time.Hour
	maxFailureBackoff   = 7 * 24 * time.Hour
)

// UpdateExternalResourceStatus 只更新 external_resource 自身状态，不触碰 Page、
// Revision 或 Outbox。它供后续 M9-T05 健康检查任务调用，是 INV-12 的写入基础。
func (s *ExternalResourceService) UpdateExternalResourceStatus(ctx context.Context, resourceID uuid.UUID, params UpdateExternalResourceStatusParams) (*ExternalResource, error) {
	if !IsValidExternalResourceStatus(params.Status) {
		return nil, fmt.Errorf("%w: external_resource status=%q", ErrInvalidSourceInput, params.Status)
	}
	if params.HTTPStatus != nil && (*params.HTTPStatus < 100 || *params.HTTPStatus > 599) {
		return nil, fmt.Errorf("%w: http_status=%d", ErrInvalidSourceInput, *params.HTTPStatus)
	}

	canonicalURL := params.CanonicalURL
	if canonicalURL != nil {
		normalized, err := NormalizeURL(*canonicalURL)
		if err != nil {
			return nil, fmt.Errorf("evidence: canonical_url 非法: %w", err)
		}
		canonicalURL = &normalized
	}
	contentHash := params.ContentHash
	if contentHash != nil {
		trimmed := strings.TrimSpace(*contentHash)
		contentHash = &trimmed
	}

	return s.repo.UpdateExternalResourceStatus(ctx, resourceID, params.Status,
		params.HTTPStatus, contentHash, canonicalURL, params.RedirectTargetID)
}

// ClaimDue leases a bounded batch of resources for probing.
func (s *ExternalResourceService) ClaimDue(ctx context.Context, limit int) ([]ExternalResource, error) {
	if limit <= 0 || limit > 100 {
		return nil, fmt.Errorf("%w: claim limit=%d", ErrInvalidSourceInput, limit)
	}
	return s.repo.ClaimDueExternalResources(ctx, limit, int64(defaultCheckLease/time.Second), uuid.New())
}

// CompleteCheck validates and stores a probe result with deterministic bounded
// backoff: success resets failures; broken/blocked doubles from one hour to
// seven days.
func (s *ExternalResourceService) CompleteCheck(ctx context.Context, resource ExternalResource, result ExternalResourceCheckResult) (*ExternalResource, error) {
	params := UpdateExternalResourceStatusParams{
		Status: result.Status, HTTPStatus: result.HTTPStatus, ContentHash: result.ContentHash,
		CanonicalURL: result.CanonicalURL, RedirectTargetID: result.RedirectTargetID,
	}
	if err := validateExternalResourceStatusParams(&params); err != nil {
		return nil, err
	}

	failures := 0
	next := defaultCheckSuccess
	if result.Status == ExternalResourceStatusBroken || result.Status == ExternalResourceStatusBlocked {
		failures = resource.ConsecutiveFailures + 1
		if result.Status == ExternalResourceStatusBroken && result.TransientFailure {
			if failures == 1 {
				next = defaultShortRetry
			} else {
				next = failureBackoff(failures - 1)
			}
		} else {
			next = failureBackoff(failures)
		}
	}
	if resource.LeaseToken == nil {
		return nil, fmt.Errorf("%w: id=%s token 为空", ErrExternalResourceLeaseLost, resource.ID)
	}
	return s.repo.CompleteExternalResourceCheck(ctx, resource.ID, result.Status, result.HTTPStatus,
		params.ContentHash, params.CanonicalURL, result.RedirectTargetID, failures, int64(next/time.Second),
		*resource.LeaseToken)
}

// RetryCheck schedules a short retry without replacing the persisted probe
// result. It is used when a later orchestration step fails after probe commit.
func (s *ExternalResourceService) RetryCheck(ctx context.Context, resource ExternalResource) error {
	if resource.LeaseToken == nil {
		return fmt.Errorf("%w: id=%s token 为空", ErrExternalResourceLeaseLost, resource.ID)
	}
	return s.repo.RetryExternalResourceCheck(ctx, resource.ID, *resource.LeaseToken,
		int64(defaultShortRetry/time.Second))
}

func failureBackoff(failures int) time.Duration {
	if failures <= 1 {
		return defaultFailureBase
	}
	backoff := defaultFailureBase
	for i := 1; i < failures && backoff < maxFailureBackoff; i++ {
		backoff *= 2
		if backoff >= maxFailureBackoff {
			return maxFailureBackoff
		}
	}
	return backoff
}

func validateExternalResourceStatusParams(params *UpdateExternalResourceStatusParams) error {
	if !IsValidExternalResourceStatus(params.Status) {
		return fmt.Errorf("%w: external_resource status=%q", ErrInvalidSourceInput, params.Status)
	}
	if params.HTTPStatus != nil && (*params.HTTPStatus < 100 || *params.HTTPStatus > 599) {
		return fmt.Errorf("%w: http_status=%d", ErrInvalidSourceInput, *params.HTTPStatus)
	}
	if params.CanonicalURL != nil {
		normalized, err := NormalizeURL(*params.CanonicalURL)
		if err != nil {
			return fmt.Errorf("evidence: canonical_url 非法: %w", err)
		}
		params.CanonicalURL = &normalized
	}
	if params.ContentHash != nil {
		trimmed := strings.TrimSpace(*params.ContentHash)
		params.ContentHash = &trimmed
	}
	return nil
}

// UpsertExternalResource 保持 Evidence 聚合服务的既有公开入口。
func (s *Service) UpsertExternalResource(ctx context.Context, rawURL string) (*ExternalResource, error) {
	return NewExternalResourceService(s.repo, s.ids).Upsert(ctx, rawURL)
}

// UpdateExternalResourceStatus 由 Evidence 聚合服务转发到系统级资源服务。
func (s *Service) UpdateExternalResourceStatus(ctx context.Context, resourceID uuid.UUID, params UpdateExternalResourceStatusParams) (*ExternalResource, error) {
	return NewExternalResourceService(s.repo, s.ids).UpdateExternalResourceStatus(ctx, resourceID, params)
}

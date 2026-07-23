package evidence

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
	"github.com/anby/wiki/backend/internal/platform/storage"
)

// Service Evidence 领域服务：asset 域（M4-T04）与 source/citation 域（M4-T05）
// 的唯一权威写入入口。source/citation 域方法见 source_service.go。
//
// StoreAsset 写入路径：
//  1. 校验 Actor 与入参；
//  2. 流式 SHA-256（TeeReader：内容只读一遍，同时入缓冲与摘要）；
//  3. 去重预检（同事务外只读）：同 wiki 同名 active asset 已有同 content_hash
//     的 asset_revision 时跳过对象存储 Put（Put 幂等，预检未命中时最坏多 Put
//     一次同内容，无害）；
//  4. 需要时先 Put 对象，再开 DB 事务——避免事务提交后对象缺失
//     （Put 成功而事务失败只留无害的残留对象，同内容下次上传可复用）；
//  5. 事务内锁定/创建 asset，复查去重，插入 asset_revision 并更新
//     asset.current_revision_id（上传使该内容成为 current）。
//
// source / source_version / source_chunk / citation 的服务属 M4-T05，不在本包。
type Service struct {
	repo  *Repository
	pages *page.Repository
	store storage.Store
	env   string
	txm   *db.TxManager
	ids   *id.Generator
}

// NewService 装配 Evidence 领域服务。
// pages 用于复用 page 模块的 Actor 准入校验（规则只维护一份）；
// env 为对象键的环境段（ADR-0004：{env}/asset/{hash前2位}/{hash}）。
func NewService(repo *Repository, pages *page.Repository, store storage.Store, env string, txm *db.TxManager, ids *id.Generator) *Service {
	return &Service{repo: repo, pages: pages, store: store, env: env, txm: txm, ids: ids}
}

// StoreAssetParams StoreAsset 的入参。
type StoreAssetParams struct {
	WikiID   uuid.UUID
	Name     string
	Content  io.Reader
	MimeType string
	ActorID  uuid.UUID
}

// StoreAssetResult StoreAsset 的结果。
type StoreAssetResult struct {
	Asset    *Asset
	Revision *AssetRevision
	// Reused 为 true 表示同 asset 同 content_hash 去重命中：
	// 复用既有 asset_revision 行，未重复 Put 对象。
	Reused bool
}

// StoreAsset 上传媒体资产：内容寻址写入对象存储，asset + asset_revision
// 同事务落库；同 asset 同内容重复上传去重（复用 revision 行、不重复 Put），
// 同名新内容则新增 asset_revision 并把 current_revision_id 指向它。
func (s *Service) StoreAsset(ctx context.Context, params StoreAssetParams) (*StoreAssetResult, error) {
	if err := s.pages.CheckWriteActor(ctx, nil, params.ActorID); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(params.Name)
	if name == "" {
		return nil, fmt.Errorf("%w: name 为空", ErrInvalidAssetInput)
	}
	mimeType := strings.TrimSpace(params.MimeType)
	if mimeType == "" {
		return nil, fmt.Errorf("%w: mime_type 为空", ErrInvalidAssetInput)
	}
	if params.Content == nil {
		return nil, fmt.Errorf("%w: content 为 nil", ErrInvalidAssetInput)
	}

	// 流式 SHA-256：TeeReader 让内容只读一遍，同时进缓冲（供 Put）与摘要。
	h := sha256.New()
	buf, err := io.ReadAll(io.TeeReader(params.Content, h))
	if err != nil {
		return nil, fmt.Errorf("evidence: 读取资产内容失败: %w", err)
	}
	contentHash := hex.EncodeToString(h.Sum(nil))
	size := int64(len(buf))
	storageKey, err := storage.ContentKey(s.env, DomainAsset, contentHash)
	if err != nil {
		return nil, err
	}

	// 去重预检（只读）：命中则跳过 Put；并发下预检可能漏判，
	// 最坏多 Put 一次同 key 同内容（幂等无害），事务内会复查。
	needPut := true
	if asset, err := s.repo.GetActiveAssetByName(ctx, nil, params.WikiID, name); err == nil {
		if _, err := s.repo.GetAssetRevisionByHash(ctx, nil, asset.ID, contentHash); err == nil {
			needPut = false
		} else if !errors.Is(err, ErrAssetRevisionNotFound) {
			return nil, err
		}
	} else if !errors.Is(err, ErrAssetNotFound) {
		return nil, err
	}

	if needPut {
		if err := s.store.Put(ctx, storageKey, bytes.NewReader(buf), size, mimeType); err != nil {
			return nil, err
		}
	}

	var result *StoreAssetResult
	err = s.txm.InTx(ctx, func(tx pgx.Tx) error {
		asset, err := s.repo.GetActiveAssetByNameForUpdate(ctx, tx, params.WikiID, name)
		if errors.Is(err, ErrAssetNotFound) {
			assetID, err := s.ids.New()
			if err != nil {
				return err
			}
			asset = &Asset{
				ID:     assetID,
				WikiID: params.WikiID,
				Name:   name,
				Status: AssetStatusActive,
			}
			if err := s.repo.InsertAsset(ctx, tx, asset); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}

		// 复查去重：预检后可能有并发上传抢先落库。
		rev, err := s.repo.GetAssetRevisionByHash(ctx, tx, asset.ID, contentHash)
		reused := true
		if errors.Is(err, ErrAssetRevisionNotFound) {
			revisionID, err := s.ids.New()
			if err != nil {
				return err
			}
			rev = &AssetRevision{
				ID:          revisionID,
				AssetID:     asset.ID,
				StorageKey:  storageKey,
				ContentHash: contentHash,
				MimeType:    mimeType,
				SizeBytes:   size,
				ActorID:     params.ActorID,
			}
			if err := s.repo.InsertAssetRevision(ctx, tx, rev); err != nil {
				return err
			}
			reused = false
		} else if err != nil {
			return err
		}

		// 上传（含去重命中）使该内容版本成为 current。
		if err := s.repo.SetAssetCurrentRevision(ctx, tx, asset.ID, rev.ID); err != nil {
			return err
		}
		revID := rev.ID
		asset.CurrentRevisionID = &revID
		result = &StoreAssetResult{Asset: asset, Revision: rev, Reused: reused}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

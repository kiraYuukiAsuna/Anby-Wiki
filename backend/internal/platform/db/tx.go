package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Querier 是 pgxpool.Pool 与 pgx.Tx 共有的最小查询接口。
// Repository 方法依赖它，从而同一份 SQL 既可在裸连接池上执行，
// 也可在领域服务编排的事务内执行（ADR-0002）。
type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// TxManager 在 pgxpool 上提供声明式事务边界。
type TxManager struct {
	pool *pgxpool.Pool
}

// NewTxManager 创建基于连接池的 TxManager。
func NewTxManager(pool *pgxpool.Pool) *TxManager {
	return &TxManager{pool: pool}
}

// InTx 在单个事务内执行 fn：fn 返回 nil 则提交，否则回滚并透传错误。
// fn 收到的事务可传给各 Repository 方法，实现跨表原子写入。
func (m *TxManager) InTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := m.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("db: 开启事务失败: %w", err)
	}
	defer func() {
		// 提交后回滚是 no-op；此处兜底未提交路径（fn 出错或 panic 后 recover 的场景）。
		_ = tx.Rollback(ctx)
	}()
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("db: 提交事务失败: %w", err)
	}
	return nil
}

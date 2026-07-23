// Package db 提供 pgxpool 连接池构造与健康检查。
package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Connect 按连接串创建 pgxpool 连接池并立即 Ping 验证连通性。
func Connect(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("db: 解析连接串失败: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("db: 连接数据库失败: %w", err)
	}
	return pool, nil
}

// Ping 检查已有连接池的连通性（供 /readyz 使用）。
func Ping(ctx context.Context, pool *pgxpool.Pool) error {
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("db: Ping 失败: %w", err)
	}
	return nil
}

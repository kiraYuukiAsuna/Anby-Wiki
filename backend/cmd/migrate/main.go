// Command migrate 是 golang-migrate 的 CLI 包装。
// 子命令：up / down N / version / check EXPECTED MIN MAX；
// 迁移目录 backend/migrations；数据源取 DATABASE_URL。
package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"

	"github.com/anby/wiki/backend/internal/platform/config"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// migrationsPath 迁移文件目录（file source 相对路径）。
const migrationsPath = "file://migrations"

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("migrate 失败", slog.Any("error", err))
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New("用法: migrate <up|down N|version|check EXPECTED MIN MAX>")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	m, err := migrate.New(migrationsPath, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("初始化迁移器失败: %w", err)
	}
	defer func() {
		srcErr, dbErr := m.Close()
		if srcErr != nil || dbErr != nil {
			slog.Warn("关闭迁移器异常", slog.Any("source", srcErr), slog.Any("database", dbErr))
		}
	}()

	switch args[0] {
	case "up":
		// ErrNoChange 表示已是最新，视为成功。
		if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			return fmt.Errorf("up 执行失败: %w", err)
		}
		slog.Info("迁移已应用到最新")
	case "down":
		if len(args) != 2 {
			return errors.New("用法: migrate down N")
		}
		n, err := strconv.Atoi(args[1])
		if err != nil || n <= 0 {
			return fmt.Errorf("down 步数非法: %q", args[1])
		}
		if err := m.Steps(-n); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			return fmt.Errorf("down %d 执行失败: %w", n, err)
		}
		slog.Info("已回滚迁移", slog.Int("steps", n))
	case "version":
		v, dirty, err := m.Version()
		if errors.Is(err, migrate.ErrNilVersion) {
			fmt.Println("version: 无（空库）")
			return nil
		}
		if err != nil {
			return fmt.Errorf("查询版本失败: %w", err)
		}
		fmt.Printf("version: %d (dirty=%v)\n", v, dirty)
	case "check":
		if len(args) != 4 {
			return errors.New("用法: migrate check EXPECTED MIN MAX")
		}
		expected, err := positiveVersion("EXPECTED", args[1])
		if err != nil {
			return err
		}
		minCompatible, err := positiveVersion("MIN", args[2])
		if err != nil {
			return err
		}
		maxCompatible, err := positiveVersion("MAX", args[3])
		if err != nil {
			return err
		}
		current, dirty, err := m.Version()
		if errors.Is(err, migrate.ErrNilVersion) {
			return errors.New("migration gate: 数据库为空，未达到预期版本")
		}
		if err != nil {
			return fmt.Errorf("migration gate: 查询版本失败: %w", err)
		}
		if err := validateGate(current, dirty, expected, minCompatible, maxCompatible); err != nil {
			return err
		}
		fmt.Printf("migration gate: pass (current=%d expected=%d compatible=%d..%d dirty=false)\n",
			current, expected, minCompatible, maxCompatible)
	default:
		return fmt.Errorf("未知子命令: %s（支持 up / down N / version / check EXPECTED MIN MAX）", args[0])
	}
	return nil
}

func positiveVersion(name, raw string) (uint, error) {
	value, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || value == 0 {
		return 0, fmt.Errorf("migration gate: %s 版本非法: %q", name, raw)
	}
	return uint(value), nil
}

func validateGate(current uint, dirty bool, expected, minCompatible, maxCompatible uint) error {
	if minCompatible > maxCompatible {
		return fmt.Errorf("migration gate: 兼容窗口非法: min=%d max=%d", minCompatible, maxCompatible)
	}
	if expected < minCompatible || expected > maxCompatible {
		return fmt.Errorf("migration gate: 预期版本 %d 不在镜像兼容窗口 %d..%d", expected, minCompatible, maxCompatible)
	}
	if dirty {
		return fmt.Errorf("migration gate: 数据库版本 %d 为 dirty，停止发布并执行前向修复", current)
	}
	if current != expected {
		return fmt.Errorf("migration gate: 数据库版本 %d 与预期版本 %d 不一致", current, expected)
	}
	return nil
}

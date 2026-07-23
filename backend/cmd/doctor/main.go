// Command doctor inspects database consistency and is read-only by default.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/anby/wiki/backend/internal/doctor"
	"github.com/anby/wiki/backend/internal/platform/db"
)

const (
	exitOK      = 0
	exitIssues  = 1
	exitUsage   = 2
	exitRuntime = 3
)

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(stderr)
	format := fs.String("format", "human", "报告格式: human|json")
	stuckAfter := fs.Duration("claimed-stuck-after", 5*time.Minute, "Outbox claimed 卡死阈值")
	repairExpiredAuth := fs.Bool("repair-expired-auth", false, "显式删除过期 OIDC 登录临时态和认证会话")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() != 0 || (*format != "human" && *format != "json") || *stuckAfter <= 0 {
		fmt.Fprintln(stderr, "用法: doctor [-format human|json] [-claimed-stuck-after 5m] [-repair-expired-auth]")
		return exitUsage
	}
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		fmt.Fprintln(stderr, "doctor: DATABASE_URL 未配置")
		return exitRuntime
	}
	pool, err := db.Connect(ctx, databaseURL)
	if err != nil {
		fmt.Fprintf(stderr, "doctor: 连接数据库失败: %v\n", err)
		return exitRuntime
	}
	defer pool.Close()

	now := time.Now().UTC()
	var repairs *doctor.RepairSummary
	if *repairExpiredAuth {
		summary, err := doctor.CleanupExpiredAuth(ctx, pool, now)
		if err != nil {
			fmt.Fprintf(stderr, "doctor: 修复过期 auth 临时态失败: %v\n", err)
			return exitRuntime
		}
		repairs = &summary
	}
	report, err := doctor.New(pool, doctor.Options{
		Now: now, ClaimedStuckAfter: *stuckAfter,
	}).Run(ctx)
	if err != nil {
		fmt.Fprintf(stderr, "doctor: 巡检失败: %v\n", err)
		return exitRuntime
	}
	if repairs != nil {
		report.ReadOnly = false
		report.Repairs = repairs
	}
	if *format == "json" {
		err = doctor.WriteJSON(stdout, report)
	} else {
		err = doctor.WriteHuman(stdout, report)
	}
	if err != nil {
		fmt.Fprintf(stderr, "doctor: 写出报告失败: %v\n", err)
		return exitRuntime
	}
	if !report.Healthy() {
		return exitIssues
	}
	return exitOK
}

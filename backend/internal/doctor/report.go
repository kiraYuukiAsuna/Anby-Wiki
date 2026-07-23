// Package doctor implements read-only data consistency inspections.
package doctor

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"
)

type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityError    Severity = "error"
	SeverityCritical Severity = "critical"
)

type Issue struct {
	Code           string            `json:"code"`
	Severity       Severity          `json:"severity"`
	Category       string            `json:"category"`
	Message        string            `json:"message"`
	ResourceType   string            `json:"resource_type,omitempty"`
	ResourceID     string            `json:"resource_id,omitempty"`
	Details        map[string]string `json:"details,omitempty"`
	Recommendation string            `json:"recommendation"`
}

type Summary struct {
	Total    int `json:"total"`
	Info     int `json:"info"`
	Warning  int `json:"warning"`
	Error    int `json:"error"`
	Critical int `json:"critical"`
}

type RepairSummary struct {
	ExpiredLoginAttempts int64 `json:"expired_login_attempts_deleted,omitempty"`
	ExpiredSessions      int64 `json:"expired_sessions_deleted,omitempty"`
}

type Report struct {
	Version     string         `json:"version"`
	GeneratedAt time.Time      `json:"generated_at"`
	ReadOnly    bool           `json:"read_only"`
	Summary     Summary        `json:"summary"`
	Issues      []Issue        `json:"issues"`
	Repairs     *RepairSummary `json:"repairs,omitempty"`
}

func NewReport(now time.Time, readOnly bool, issues []Issue) Report {
	slices.SortFunc(issues, func(a, b Issue) int {
		if n := strings.Compare(a.Code, b.Code); n != 0 {
			return n
		}
		if n := strings.Compare(a.ResourceType, b.ResourceType); n != 0 {
			return n
		}
		return strings.Compare(a.ResourceID, b.ResourceID)
	})
	report := Report{Version: "m9-t08-v1", GeneratedAt: now.UTC(), ReadOnly: readOnly, Issues: issues}
	for _, issue := range issues {
		report.Summary.Total++
		switch issue.Severity {
		case SeverityInfo:
			report.Summary.Info++
		case SeverityWarning:
			report.Summary.Warning++
		case SeverityError:
			report.Summary.Error++
		case SeverityCritical:
			report.Summary.Critical++
		}
	}
	return report
}

func (r Report) Healthy() bool {
	return r.Summary.Error == 0 && r.Summary.Critical == 0
}

func WriteJSON(w io.Writer, report Report) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}

func WriteHuman(w io.Writer, report Report) error {
	mode := "只读"
	if !report.ReadOnly {
		mode = "显式修复"
	}
	if _, err := fmt.Fprintf(w, "Anby Wiki 数据一致性巡检 (%s)\n", mode); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "版本: %s  时间: %s\n", report.Version, report.GeneratedAt.Format(time.RFC3339)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "汇总: total=%d critical=%d error=%d warning=%d info=%d\n",
		report.Summary.Total, report.Summary.Critical, report.Summary.Error,
		report.Summary.Warning, report.Summary.Info); err != nil {
		return err
	}
	for _, issue := range report.Issues {
		resource := ""
		if issue.ResourceType != "" {
			resource = " " + issue.ResourceType + "=" + issue.ResourceID
		}
		if _, err := fmt.Fprintf(w, "[%s] %s%s: %s\n  建议: %s\n",
			strings.ToUpper(string(issue.Severity)), issue.Code, resource,
			issue.Message, issue.Recommendation); err != nil {
			return err
		}
	}
	if report.Repairs != nil {
		_, err := fmt.Fprintf(w, "修复: expired_login_attempts=%d expired_sessions=%d\n",
			report.Repairs.ExpiredLoginAttempts, report.Repairs.ExpiredSessions)
		return err
	}
	return nil
}

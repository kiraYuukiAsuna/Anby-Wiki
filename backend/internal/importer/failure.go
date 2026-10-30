package importer

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/anby/wiki/backend/internal/ai"
	"github.com/anby/wiki/backend/internal/governance"
)

const (
	ErrorUnsafeURL                 = "unsafe_url"
	ErrorSourceTooLarge            = "source_too_large"
	ErrorUnsupportedMIME           = "unsupported_mime"
	ErrorMalwareDetected           = "malware_detected"
	ErrorFetchFailed               = "fetch_failed"
	ErrorAssetStoreFailed          = "asset_store_failed"
	ErrorSourceCreateFailed        = "source_create_failed"
	ErrorParseFailed               = "parse_failed"
	ErrorPDFExtractorUnavailable   = "pdf_extractor_unavailable"
	ErrorPDFTextTooLarge           = "pdf_text_too_large"
	ErrorPDFRasterizerUnavailable  = "pdf_rasterizer_unavailable"
	ErrorPDFPageLimitExceeded      = "pdf_page_limit_exceeded"
	ErrorAdvancedParserUnavailable = "advanced_parser_unavailable"
	ErrorVisionPageAnalysisFailed  = "vision_page_analysis_failed"
	ErrorOCRUnavailable            = "ocr_unavailable"
	ErrorOCRImageTooLarge          = "ocr_image_too_large"
	ErrorOCRTextTooLarge           = "ocr_text_too_large"
	ErrorOCRNoText                 = "ocr_no_text"
	ErrorOCRFailed                 = "ocr_failed"
	ErrorParseArtifactFailed       = "parse_artifact_failed"
	ErrorParseArtifactStoreFailed  = "parse_artifact_store_failed"
	ErrorSourceVersionFailed       = "source_version_failed"
	ErrorExtractionFailed          = "extraction_failed"
	ErrorExtractionInvalidOutput   = "extraction_invalid_output"
	ErrorExtractionOutputTruncated = "extraction_output_truncated"
	ErrorExtractionProviderFailed  = "extraction_provider_failed"
	ErrorExtractionTimeout         = "extraction_timeout"
	ErrorExtractionEvidenceInvalid = "extraction_evidence_invalid"
	ErrorExtractionInputTooLarge   = "extraction_input_too_large"
	ErrorQualityGate               = "quality_gate"
	ErrorPagePlanFailed            = "page_plan_failed"
	ErrorPagePlanInvalidOutput     = "page_plan_invalid_output"
	ErrorPagePlanOutputTruncated   = "page_plan_output_truncated"
	ErrorPagePlanProviderFailed    = "page_plan_provider_failed"
	ErrorPagePlanTimeout           = "page_plan_timeout"
	ErrorPagePlanEvidenceInvalid   = "page_plan_evidence_invalid"
	ErrorPagePlanInputTooLarge     = "page_plan_input_too_large"
	ErrorPagePlanTargetConflict    = "page_plan_target_conflict"
	ErrorPagePlanQualityGate       = "page_plan_quality_gate"
	ErrorNoPagePlan                = "no_page_plan"
	ErrorEntityMatchFailed         = "entity_match_failed"
	ErrorClaimClassifyFailed       = "claim_classification_failed"
	ErrorProposalComposeFailed     = "proposal_compose_failed"
	ErrorReviewSubmitFailed        = "review_submit_failed"
	ErrorInvalidConfig             = "invalid_config"
	ErrorWorkerInterrupted         = "worker_interrupted"
	ErrorCancelled                 = "cancelled"
	ErrorImportTimeout             = "import_timeout"
	ErrorImportFailed              = "import_failed"
	ErrorAuthorUnavailable         = "author_unavailable"
)

// Failure v1 deliberately contains only public diagnostics. Error strings,
// provider response bodies, prompts and source quotations are never serialized.
type Failure struct {
	SchemaVersion int             `json:"schema_version"`
	Stage         string          `json:"stage"`
	Code          string          `json:"code"`
	Message       string          `json:"message"`
	Suggestion    string          `json:"suggestion"`
	Retryable     bool            `json:"retryable"`
	Details       *FailureDetails `json:"details,omitempty"`
}

type FailureDetails struct {
	SourceVersionID         *uuid.UUID `json:"source_version_id,omitempty"`
	ExtractionID            *uuid.UUID `json:"extraction_id,omitempty"`
	Model                   string     `json:"model,omitempty"`
	PromptInjectionDetected *bool      `json:"prompt_injection_detected,omitempty"`
	QualityScore            *float64   `json:"quality_score,omitempty"`
	QualityThreshold        *float64   `json:"quality_threshold,omitempty"`
	EntityCount             *int       `json:"entity_count,omitempty"`
	ClaimCount              *int       `json:"claim_count,omitempty"`
	ExtractionReused        *bool      `json:"extraction_reused,omitempty"`
	ProviderCode            string     `json:"provider_code,omitempty"`
	HTTPStatus              int        `json:"http_status,omitempty"`
	MaxInputTokens          int        `json:"max_input_tokens,omitempty"`
	MaxSourceBytes          int64      `json:"max_source_bytes,omitempty"`
	MaxPages                int        `json:"max_pages,omitempty"`
	Page                    int        `json:"page,omitempty"`
}

type failureDescription struct {
	message    string
	suggestion string
	retryable  bool
}

type sourceHTTPError struct {
	status int
}

func (e *sourceHTTPError) Error() string { return "importer: 来源 HTTP " + strconv.Itoa(e.status) }
func (e *sourceHTTPError) Unwrap() error { return ErrFetchFailed }

type visionPageError struct {
	page  int
	cause error
}

func (e *visionPageError) Error() string {
	return "importer: 视觉页面分析失败，页码 " + strconv.Itoa(e.page)
}
func (e *visionPageError) Unwrap() []error { return []error{ErrVisionPageAnalysisFailed, e.cause} }

var failureDescriptions = map[string]failureDescription{
	ErrorUnsafeURL:                 {"来源网址或重定向被安全策略拒绝。", "请使用可公开访问的 HTTP/HTTPS 地址，检查是否指向内网或重定向过多。", false},
	ErrorSourceTooLarge:            {"来源文件超过下载或上传大小上限。", "请拆分或压缩来源后重新导入。", false},
	ErrorUnsupportedMIME:           {"来源格式不受支持，或文件内容与声明的格式不符。", "请确认文件内容，并转换为支持的文档或图片格式后重新导入。", false},
	ErrorSourceOnlyFormat:          {"仅存储来源模式不支持这个文件格式。", "请选择文本、Markdown、HTML、JSON 或 CSV，或切换为完整导入。", false},
	ErrorMalwareDetected:           {"来源未通过恶意内容扫描。", "请检查并清理原文件，确认安全后重新导入。", false},
	ErrorFetchFailed:               {"无法取得来源内容。", "请检查网址是否可访问、上传对象是否仍存在，以及存储服务状态。", true},
	ErrorAssetStoreFailed:          {"原始来源文件保存失败。", "请检查对象存储服务后重试。", true},
	ErrorSourceCreateFailed:        {"来源记录创建失败。", "请检查服务状态及来源参数后重试。", true},
	ErrorParseFailed:               {"来源内容无法解析。", "请检查文件是否损坏、加密或缺少正文，必要时转换为文本后重新导入。", false},
	ErrorPDFExtractorUnavailable:   {"服务器缺少 PDF 文本提取组件。", "请联系管理员配置 PDF 文本提取组件后重试。", false},
	ErrorPDFTextTooLarge:           {"PDF 解压后的文本超过处理上限。", "请拆分 PDF，或导出所需章节的文本后重新导入。", false},
	ErrorPDFRasterizerUnavailable:  {"服务器缺少 PDF 页面渲染组件。", "请联系管理员配置 PDF 页面渲染组件后重试。", false},
	ErrorPDFPageLimitExceeded:      {"PDF 页数超过高级解析上限。", "请按章节拆分 PDF 后重新导入。", false},
	ErrorAdvancedParserUnavailable: {"高级文档解析器未配置。", "请联系管理员配置解析器，或转换为文本后重新导入。", false},
	ErrorVisionPageAnalysisFailed:  {"PDF 视觉页面分析失败。", "请检查多模态模型的配置和服务状态后重试。", true},
	ErrorOCRUnavailable:            {"服务器缺少 OCR 组件或语言数据。", "请联系管理员配置 OCR 后重试。", false},
	ErrorOCRImageTooLarge:          {"图片像素超过 OCR 安全上限。", "请缩小图片尺寸后重新导入。", false},
	ErrorOCRTextTooLarge:           {"OCR 输出超过处理上限。", "请拆分图片或 PDF 后重新导入。", false},
	ErrorOCRNoText:                 {"图片中未识别到可抽取文本。", "请换用清晰的原图，或整理为文本后重新导入。", false},
	ErrorOCRFailed:                 {"OCR 执行失败。", "请检查图片清晰度及 OCR 服务后重试。", true},
	ErrorParseArtifactFailed:       {"结构化解析制品生成失败。", "请检查文件格式；问题持续出现时请联系管理员。", false},
	ErrorParseArtifactStoreFailed:  {"结构化解析制品保存失败。", "请检查对象存储服务后重试。", true},
	ErrorSourceVersionFailed:       {"解析结果和来源版本保存失败。", "请检查数据库及来源记录后重试。", true},
	ErrorExtractionFailed:          {"事实候选抽取失败。", "请重试；问题持续出现时请联系管理员并提供任务 ID。", true},
	ErrorExtractionInvalidOutput:   {"模型返回的结构不符合事实抽取 Schema。", "请重试或更换支持结构化输出的模型。", true},
	ErrorExtractionOutputTruncated: {"事实抽取输出达到模型长度上限。", "请减少单次来源内容，或使用支持更长输出的模型。", false},
	ErrorExtractionProviderFailed:  {"事实抽取的模型供应商调用失败。", "请检查模型、额度和 API Key 后重试。", true},
	ErrorExtractionTimeout:         {"事实抽取的模型调用超时。", "请稍后重试，或减少来源内容、更换模型。", true},
	ErrorExtractionEvidenceInvalid: {"模型返回的事实引用无法与来源逐字核对。", "请检查来源文本完整性，重试或更换模型。", true},
	ErrorExtractionInputTooLarge:   {"事实抽取输入超过模型窗口预算。", "请提高最大输入 Token 配置，或缩小来源后重新导入。", false},
	ErrorQualityGate:               {"事实抽取结果未通过安全门禁。", "请检查来源内容；已有抽取结果会被复用，直接重试或重新规划通常无法消除拦截。", false},
	ErrorPagePlanFailed:            {"页面规划失败。", "请重试或调整导入要求；问题持续出现时请联系管理员。", true},
	ErrorPagePlanInvalidOutput:     {"模型返回的页面规划结构不合法，自适应拆分后仍无法修复。", "请调整导入要求，或更换支持结构化输出的模型后重新规划。", true},
	ErrorPagePlanOutputTruncated:   {"页面规划输出达到模型长度上限。", "请缩小来源，或使用支持更长输出的模型后重新规划。", false},
	ErrorPagePlanProviderFailed:    {"页面规划的模型供应商调用失败。", "请检查模型、额度和 API Key 后重试。", true},
	ErrorPagePlanTimeout:           {"页面规划的模型调用超时。", "请稍后重试或缩小来源后重新规划。", true},
	ErrorPagePlanEvidenceInvalid:   {"页面规划引用无法通过原文逐字核验。", "请检查来源是否完整，并调整导入要求或更换模型后重新规划。", true},
	ErrorPagePlanInputTooLarge:     {"页面规划输入超过模型窗口预算。", "请提高最大输入 Token 配置，或缩小来源后重新导入。", false},
	ErrorPagePlanTargetConflict:    {"目标页面或新页面标题发生冲突。", "请检查是否已有同名页面，调整标题或目标页面后重新规划。", false},
	ErrorPagePlanQualityGate:       {"页面规划未达到质量门槛。", "请查看质量分项，调整标题、目标页面或导入要求后重新规划。", false},
	ErrorNoPagePlan:                {"来源有价值，但没有形成可审核的页面写入计划。", "请调整页面路由和导入要求后重新规划。", false},
	ErrorEntityMatchFailed:         {"实体匹配失败。", "请检查实体数据及服务状态后重试。", true},
	ErrorClaimClassifyFailed:       {"事实声明分类失败。", "请检查知识数据及服务状态后重试。", true},
	ErrorProposalComposeFailed:     {"审核提案合成失败。", "请检查目标页面和实体状态后重试；持续失败时请联系管理员。", true},
	ErrorReviewSubmitFailed:        {"提案提交审核失败。", "请检查提案状态及审核权限后重试。", true},
	ErrorInvalidConfig:             {"导入任务配置或规划参数不合法。", "请检查来源和路由参数，重新创建导入任务。", false},
	ErrorWorkerInterrupted:         {"Worker 中断，未能完成本次导入。", "任务已自动重新排队，将从已保存的来源继续。", true},
	ErrorCancelled:                 {"导入任务已取消。", "需要继续时可以重试，已保存的来源会被复用。", true},
	ErrorImportTimeout:             {"导入任务超过执行时间限制。", "请稍后重试；大文件可以拆分后重新导入。", true},
	ErrorImportFailed:              {"导入阶段执行或状态保存失败。", "请检查服务状态后重试；持续失败时请联系管理员并提供任务 ID。", true},
	ErrorAuthorUnavailable:         {"选定的机器作者已停用或失去绑定权限。", "请在 AI 信任中恢复 Actor，并确认其所属账户仍有当前 Wiki 的管理员权限，再重试；任务会保留原作者身份。", false},
}

func newFailure(stage, code string, cause error) Failure {
	if errors.Is(cause, ErrAuthorUnavailable) || errors.Is(cause, governance.ErrImportAuthorUnavailable) {
		code = ErrorAuthorUnavailable
	}
	description, ok := failureDescriptions[code]
	if !ok {
		description = failureDescriptions[ErrorImportFailed]
	}
	failure := Failure{SchemaVersion: 1, Stage: stage, Code: code,
		Message: description.message, Suggestion: description.suggestion, Retryable: description.retryable}
	var provider *ai.ProviderError
	if errors.As(cause, &provider) {
		details := &FailureDetails{}
		// A provider may return an arbitrary error code. Only locally understood
		// codes are public; even syntactically valid unknown strings can be secrets.
		switch provider.Code {
		case "transport", "invalid_request", "encode_request", "build_request",
			"response_too_large", "invalid_response", "invalid_json", "output_truncated",
			"configuration_unavailable", "kernel_unavailable", "kernel_response_read",
			"kernel_http_error", "kernel_invalid_response", "invalid_schema",
			"invalid_structured_output", "provider_error", "timeout",
			"authentication_failed", "rate_limited", "provider_unavailable",
			"unauthorized", "invalid_image":
			details.ProviderCode = provider.Code
		default:
			if strings.HasPrefix(provider.Code, "http_") {
				status, err := strconv.Atoi(strings.TrimPrefix(provider.Code, "http_"))
				if err == nil && status >= 400 && status <= 599 && provider.Code == "http_"+strconv.Itoa(status) {
					details.ProviderCode, details.HTTPStatus = provider.Code, status
				}
			}
		}
		failure.Details = details
		switch details.ProviderCode {
		case "authentication_failed", "unauthorized":
			failure.Message = "模型服务拒绝认证或没有模型访问权限。"
			failure.Suggestion = "请检查个人中心的模型 API Key 和访问权限；AI Kernel 认证失败时请联系管理员。修复后重试。"
			failure.Retryable = false
		case "rate_limited":
			failure.Message = "模型服务限流或额度不足。"
			failure.Suggestion = "请检查模型额度，或等待限流解除后重试。"
		case "provider_unavailable", "kernel_unavailable":
			failure.Message = "模型服务暂时不可用。"
			failure.Suggestion = "请稍后重试，或检查模型服务状态。"
		case "invalid_request":
			failure.Message = "模型服务拒绝了请求参数。"
			failure.Suggestion = "请检查模型名称、服务地址及模型是否支持结构化输出或图像输入，修复后重试。"
			failure.Retryable = false
		case "configuration_unavailable":
			failure.Message = "模型配置无法读取或不完整。"
			failure.Suggestion = "请检查个人中心的模型配置；持续失败时请联系管理员。"
			failure.Retryable = false
		}
		switch details.HTTPStatus {
		case 401, 403:
			failure.Message = "模型服务拒绝认证或当前账号没有模型访问权限。"
			failure.Suggestion = "请在个人中心检查模型 API Key 和访问权限，修复后重试。"
			failure.Retryable = false
		case 404:
			failure.Message = "模型服务地址或模型名称不存在。"
			failure.Suggestion = "请在个人中心检查模型名称和服务地址，修复后重试。"
			failure.Retryable = false
		case 429:
			failure.Message = "模型服务限流或额度不足。"
			failure.Suggestion = "请检查模型额度，或等待限流解除后重试。"
		default:
			if details.HTTPStatus >= 500 {
				failure.Message = "模型服务暂时不可用。"
				failure.Suggestion = "请稍后重试，或更换模型服务。"
			}
		}
	}
	var sourceHTTP *sourceHTTPError
	if errors.As(cause, &sourceHTTP) {
		failure.Details = &FailureDetails{HTTPStatus: sourceHTTP.status}
		failure.Message = "来源服务器返回了非成功 HTTP 状态，无法下载内容。"
		failure.Suggestion = "请检查来源地址、访问权限及网站服务状态，确认能访问正文后重试。"
		failure.Retryable = sourceHTTP.status == 429 || sourceHTTP.status >= 500
	}
	var pageError *visionPageError
	if errors.As(cause, &pageError) {
		if failure.Details == nil {
			failure.Details = &FailureDetails{}
		}
		failure.Details.Page = pageError.page
	}
	switch code {
	case ErrorSourceTooLarge:
		failure.Details = &FailureDetails{MaxSourceBytes: DefaultMaxSourceBytes}
	case ErrorPDFPageLimitExceeded:
		failure.Details = &FailureDetails{MaxPages: maxAdvancedPDFPages}
	}
	return failure
}

func (f Failure) JSON() json.RawMessage {
	raw, _ := json.Marshal(f)
	return raw
}

func extractionFailure(extracted *ExtractResult, threshold float64) Failure {
	failure := newFailure(StageExtract, ErrorQualityGate, nil)
	if extracted == nil || extracted.Extraction == nil || extracted.Candidates == nil {
		failure.Message = "事实抽取未返回可校验的候选结果。"
		return failure
	}
	candidates, record := extracted.Candidates, extracted.Extraction
	entities, claims := len(candidates.Entities), len(candidates.Claims)
	failure.Details = &FailureDetails{
		SourceVersionID: &record.SourceVersionID, ExtractionID: &record.ID, Model: record.Model,
		PromptInjectionDetected: &candidates.PromptInjectionDetected,
		QualityScore:            &candidates.QualityScore, QualityThreshold: &threshold,
		EntityCount: &entities, ClaimCount: &claims, ExtractionReused: &extracted.Reused,
	}
	if candidates.PromptInjectionDetected {
		failure.Message = "提取结果标记了疑似提示注入，安全门禁已阻止后续规划。"
		failure.Suggestion = "请检查来源中要求模型扮演角色、忽略规则或执行指令的内容；角色设定可以改为第三人称资料后重新上传。该标记可能误判，需核查原文。相同来源会复用原提取结果，直接重试或重新规划无法清除这个标记。"
	}
	return failure
}

func legacyFailure(raw json.RawMessage) bool {
	var value Failure
	return json.Unmarshal(raw, &value) == nil && value.Code != "" &&
		value.Code != "entity_resolution_required" && value.SchemaVersion == 0
}

func enrichFailure(raw json.RawMessage) json.RawMessage {
	if !legacyFailure(raw) {
		return raw
	}
	var value Failure
	_ = json.Unmarshal(raw, &value)
	return newFailure(value.Stage, value.Code, nil).JSON()
}

// Historical errors are enriched on read only. Each failed stage uses its own
// immutable source checkpoint, so older runs cannot acquire newer diagnostics.
func (s *Service) enrichDetailFailures(ctx context.Context, detail *JobDetail) error {
	extractions := map[uuid.UUID]*Extraction{}
	for index := range detail.Stages {
		stage := &detail.Stages[index]
		if !legacyFailure(stage.Error) {
			continue
		}
		var value Failure
		_ = json.Unmarshal(stage.Error, &value)
		stage.Error = enrichFailure(stage.Error)
		if value.Code != ErrorQualityGate || stage.Stage != StageExtract || stage.InputHash == nil {
			continue
		}
		sourceID, err := uuid.Parse(*stage.InputHash)
		if err != nil {
			continue
		}
		record, found := extractions[sourceID]
		if !found {
			record, err = s.repo.GetExtraction(ctx, sourceID)
			if err != nil && !errors.Is(err, ErrExtractionNotFound) {
				return err
			}
			extractions[sourceID] = record
		}
		if record == nil {
			continue
		}
		var candidates Candidates
		if json.Unmarshal(record.CandidatesJSON, &candidates) != nil {
			continue
		}
		// The exact historical threshold was not recorded at this stage.
		// Do not substitute today's model configuration.
		failure := extractionFailure(&ExtractResult{Extraction: record, Candidates: &candidates,
			Reused: record.CreatedAt.Before(stage.StartedAt)}, DefaultQualityThreshold)
		failure.Details.QualityThreshold = nil
		stage.Error = failure.JSON()
	}
	for index := range detail.Runs {
		run := &detail.Runs[index]
		if !legacyFailure(run.Error) {
			continue
		}
		run.Error = enrichFailure(run.Error)
		for _, stage := range detail.Stages {
			if stage.ImportRunID == run.ID && stage.Status == StageFailed {
				run.Error = stage.Error
			}
		}
	}
	if legacyFailure(detail.Job.Error) {
		detail.Job.Error = enrichFailure(detail.Job.Error)
		if detail.Job.Status == JobFailed && len(detail.Runs) > 0 {
			detail.Job.Error = detail.Runs[len(detail.Runs)-1].Error
		}
	}
	return nil
}

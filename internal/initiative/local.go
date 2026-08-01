package initiative

import (
	"context"
	"fmt"

	"ekbda/internal/planning"
)

type LocalProvider struct{}

func NewLocalProvider() *LocalProvider { return &LocalProvider{} }

func (*LocalProvider) Name() string { return "local-deterministic-project-package-v1" }

func (*LocalProvider) Build(_ context.Context, session planning.Session) (BuildOutput, error) {
	if session.Plan == nil || session.RoleReview == nil {
		return BuildOutput{}, fmt.Errorf("approved planning artifacts are required")
	}
	references := packageReferences(session)
	clarifications := make([]string, 0, len(session.Answers))
	for _, answer := range session.Answers {
		clarifications = append(clarifications, answer.QuestionID+"："+answer.Answer)
	}
	if len(clarifications) == 0 {
		clarifications = append(clarifications, "目标、约束和范围以批准规划会话中的结构化字段为准。")
	}
	steps := make([]string, 0, len(session.Plan.Steps))
	verifications := make([]string, 0)
	for _, step := range session.Plan.Steps {
		steps = append(steps, step.ID+" "+step.Title+"："+step.Description)
		for _, verification := range step.Verification {
			verifications = append(verifications, step.ID+"："+verification)
		}
	}
	risks := make([]string, 0, len(session.Plan.Risks))
	for _, risk := range session.Plan.Risks {
		risks = append(risks, risk.ID+" "+risk.Description+"；缓解："+risk.Mitigation)
	}
	if len(risks) == 0 {
		risks = append(risks, "没有未记录的零风险假设；实施前仍需执行安全、质量和发布门禁。")
	}
	roleConclusions := make([]string, 0, len(session.RoleReview.Reviews))
	for _, review := range session.RoleReview.Reviews {
		roleConclusions = append(roleConclusions, review.Role+"："+review.Summary)
	}
	decisionItems := make([]string, 0, len(session.RoleReview.Coordination.DecisionItems))
	for _, decision := range session.RoleReview.Coordination.DecisionItems {
		decisionItems = append(decisionItems, decision.ID+" "+decision.Topic+"："+decision.Resolution)
	}
	if len(decisionItems) == 0 {
		decisionItems = append(decisionItems, "本版本没有需要单独记录的角色冲突决议。")
	}
	artifacts := []Artifact{
		{Type: ArtifactPRD, Title: session.Title + " PRD", Summary: session.Requirement, Sections: []Section{
			{Name: "目标与价值", Items: []string{session.Requirement, session.Plan.Summary}},
			{Name: "范围与澄清", Items: clarifications},
			{Name: "验收与非目标", Items: append(fallback(session.AcceptanceCriteria, "验收标准以澄清答案和批准计划中的验证项为准。"), fallback(session.OutOfScope, "不扩展批准规划会话之外的能力。")...)},
		}, References: references},
		{Type: ArtifactArchitecture, Title: session.Title + " 架构方案", Summary: "以批准计划的模块边界、兼容策略和企业规范为架构输入。", Sections: []Section{
			{Name: "总体设计", Items: []string{session.Plan.Summary}},
			{Name: "组件与数据流", Items: steps},
			{Name: "角色评审约束", Items: roleConclusions},
		}, References: references},
		{Type: ArtifactAPI, Title: session.Title + " API 与契约", Summary: "记录接口、数据与客户端契约的设计约束，不虚构尚未批准的端点。", Sections: []Section{
			{Name: "契约范围", Items: []string{"只实现批准计划明确影响的接口和数据契约。", "新增或变更端点必须在技术设计中记录请求、响应、错误码、鉴权和幂等语义。"}},
			{Name: "兼容与安全", Items: fallback(session.Constraints, "保持现有调用方兼容，并复用企业身份、授权和审计机制。")},
			{Name: "待实现映射", Items: steps},
		}, References: references},
		{Type: ArtifactTest, Title: session.Title + " 测试方案", Summary: "将验收标准、规范门禁和角色风险转化为可重复验证的测试证据。", Sections: []Section{
			{Name: "验收测试", Items: fallback(session.AcceptanceCriteria, "根据澄清答案建立可自动化的业务验收用例。")},
			{Name: "自动化与回归", Items: verifications},
			{Name: "质量门禁", Items: []string{"执行适用的目录、命名、注释、测试和安全规范。", "阻断级规则、测试失败或未解释回归均不得进入发布。"}},
		}, References: references},
		{Type: ArtifactDeployment, Title: session.Title + " 部署与回滚方案", Summary: "通过企业既有 CI/CD 和人工审批完成分阶段发布。", Sections: []Section{
			{Name: "发布前提", Items: []string{"固定批准的提交、配置和制品版本。", "测试、规范、安全和人工发布门禁全部通过。"}},
			{Name: "部署步骤", Items: []string{"在非生产环境验证迁移、配置和健康检查。", "按企业发布窗口分阶段部署并观察核心指标。", "由授权人员确认后继续扩大流量。"}},
			{Name: "回滚", Items: []string{"保留上一稳定制品和配置。", "定义触发阈值、回滚负责人、数据兼容步骤和回滚后验证。"}},
		}, References: references},
		{Type: ArtifactMonitoring, Title: session.Title + " 监控方案", Summary: "用业务、服务和发布指标验证上线结果并支持快速回滚。", Sections: []Section{
			{Name: "指标", Items: []string{"业务成功率、处理量和验收指标。", "延迟、错误率、资源使用率和依赖健康度。", "发布版本、流量比例和回滚触发指标。"}},
			{Name: "告警", Items: []string{"为错误率、延迟、容量和关键业务失败设置分级告警。", "每条告警必须有负责人、处置手册和升级路径。"}},
			{Name: "上线观察", Items: []string{"发布前保存基线，发布后按窗口对比。", "异常达到批准阈值时停止推进并执行回滚。"}},
		}, References: references},
		{Type: ArtifactRisk, Title: session.Title + " 风险与决策记录", Summary: session.RoleReview.Coordination.Summary, Sections: []Section{
			{Name: "实施风险", Items: risks},
			{Name: "多角色共识", Items: fallback(session.RoleReview.Coordination.Consensus, "五角色评审已完成且没有额外共识条目。")},
			{Name: "人工决策", Items: decisionItems},
		}, References: references},
	}
	return BuildOutput{Artifacts: artifacts, Traceability: localTraceability(artifacts)}, nil
}

func localTraceability(artifacts []Artifact) []TraceRecord {
	byType := make(map[string]Artifact, len(artifacts))
	for _, artifact := range artifacts {
		byType[artifact.Type] = artifact
	}
	prd := byType[ArtifactPRD]
	result := make([]TraceRecord, 0)
	seen := make(map[string]struct{})
	for _, section := range prd.Sections {
		for _, item := range section.Items {
			if _, found := seen[item]; found {
				continue
			}
			seen[item] = struct{}{}
			result = append(result, TraceRecord{
				RequirementID:        fmt.Sprintf("REQ-%03d", len(result)+1),
				Requirement:          item,
				ArchitectureSections: []string{firstSectionName(byType[ArtifactArchitecture])},
				APIApplicable:        true,
				APISections:          []string{firstSectionName(byType[ArtifactAPI])},
				TestSections:         []string{firstSectionName(byType[ArtifactTest])},
				DeploymentSections:   []string{firstSectionName(byType[ArtifactDeployment])},
			})
		}
	}
	return result
}

func firstSectionName(artifact Artifact) string {
	if len(artifact.Sections) == 0 {
		return ""
	}
	return artifact.Sections[0].Name
}

func packageReferences(session planning.Session) []Reference {
	result := make([]Reference, 0)
	for _, reference := range session.Context.Knowledge {
		result = append(result, Reference{Kind: ReferencePlanKnowledge, ID: reference.ID})
	}
	for _, reference := range session.Context.Standards {
		result = append(result, Reference{Kind: ReferencePlanStandard, ID: reference.ID})
	}
	for _, reference := range session.RoleReview.Context.Knowledge {
		result = append(result, Reference{Kind: ReferenceReviewKnowledge, ID: reference.ID})
	}
	for _, reference := range session.RoleReview.Context.Standards {
		result = append(result, Reference{Kind: ReferenceReviewStandard, ID: reference.ID})
	}
	for _, decision := range session.RoleReview.Coordination.DecisionItems {
		if decision.Resolution != "" {
			result = append(result, Reference{Kind: ReferenceDecision, ID: decision.ID})
		}
	}
	return result
}

func fallback(values []string, fallbackValue string) []string {
	if len(values) == 0 {
		return []string{fallbackValue}
	}
	return append([]string(nil), values...)
}

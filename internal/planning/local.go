package planning

import (
	"context"
	"fmt"
)

type LocalProvider struct{}

func NewLocalProvider() *LocalProvider { return &LocalProvider{} }

func (*LocalProvider) Name() string { return "local-deterministic-planner-v1" }

func (*LocalProvider) ReviewName() string { return "local-deterministic-reviewer-v1" }

func (*LocalProvider) Clarify(_ context.Context, session Session) ([]Question, error) {
	questions := make([]Question, 0, 3)
	if len(session.AcceptanceCriteria) == 0 {
		questions = append(questions, Question{
			ID: "acceptance", Question: "该需求可验收的业务结果和量化标准是什么？",
			Reason: "实施计划必须能映射到可重复验证的交付标准。",
		})
	}
	if len(session.Constraints) == 0 {
		questions = append(questions, Question{
			ID: "constraints", Question: "兼容性、数据迁移、性能、安全和发布时间方面有哪些硬约束？",
			Reason: "约束会直接影响技术方案、风险和回滚策略。",
		})
	}
	if len(session.OutOfScope) == 0 {
		questions = append(questions, Question{
			ID: "scope", Question: "本次明确不包含哪些能力、系统或改造范围？",
			Reason: "显式范围边界可以降低计划膨胀和隐性承诺。",
		})
	}
	return questions, nil
}

func (*LocalProvider) BuildPlan(_ context.Context, session Session) (Plan, error) {
	knowledgeReferences := make([]string, 0, len(session.Context.Knowledge))
	for _, reference := range session.Context.Knowledge {
		knowledgeReferences = append(knowledgeReferences, reference.ID)
	}
	standardReferences := make([]string, 0, len(session.Context.Standards))
	for _, reference := range session.Context.Standards {
		standardReferences = append(standardReferences, reference.ID)
	}
	assumptions := []string{
		"实施仅面向项目 " + session.Project + " 的受控仓库 " + session.Repository + "。",
		"进入编码前必须由人工审批本计划，审批不授予仓库写入或命令执行权限。",
	}
	for _, answer := range session.Answers {
		assumptions = append(assumptions, fmt.Sprintf("澄清 %s：%s", answer.QuestionID, answer.Answer))
	}
	risks := []Risk{
		{ID: "R1", Description: "需求理解与真实业务流程存在偏差。", Mitigation: "以验收标准、企业知识引用和产品负责人审批共同确认范围。"},
		{ID: "R2", Description: "实现违反现行项目规范或缺少回归验证。", Mitigation: "实施时固定使用本会话记录的规范版本，并执行对应测试和门禁。"},
	}
	if session.Context.Repository.Dirty {
		risks = append(risks, Risk{ID: "R3", Description: "规划时受控仓库存在未提交变更。", Mitigation: "编码前确认变更归属，清理工作区并重新生成规划上下文。"})
	}
	return Plan{
		Summary:     "在企业知识、项目规范和仓库快照约束下实施“" + session.Title + "”，先确认设计边界，再编码、验证和发布。",
		Assumptions: assumptions,
		Steps: []PlanStep{
			{ID: "P1", Title: "确认业务与技术设计", Description: "把需求、验收标准、澄清结论和现有业务知识映射为受影响模块、接口与数据流。", Deliverables: []string{"技术设计说明", "影响范围清单", "兼容与迁移策略"}, Verification: []string{"产品负责人确认验收标准", "开发负责人确认模块边界"}, KnowledgeReferences: knowledgeReferences, StandardReferences: standardReferences},
			{ID: "P2", Title: "按企业规范实施变更", Description: "在批准的范围内分批实现后端、前端或配置变更，不扩大权限和项目边界。", Deliverables: []string{"实现代码", "必要的配置或迁移", "变更说明"}, Verification: []string{"目录、命名和注释规范检查", "变更与批准计划逐项对应"}, KnowledgeReferences: knowledgeReferences, StandardReferences: standardReferences},
			{ID: "P3", Title: "完成测试与质量门禁", Description: "围绕验收标准补齐自动化测试，并执行项目适用规范、回归和安全检查。", Deliverables: []string{"单元与集成测试", "规范校验报告", "风险复核记录"}, Verification: []string{"所有阻断规范通过", "验收标准具备测试证据", "无未解释的回归失败"}, StandardReferences: standardReferences},
			{ID: "P4", Title: "部署准备与人工发布确认", Description: "形成部署步骤、观测指标和可执行回滚方案，由授权人员完成发布审批。", Deliverables: []string{"部署清单", "观测与告警清单", "回滚方案"}, Verification: []string{"发布审批完成", "回滚路径已验证或演练", "上线后指标责任人明确"}, StandardReferences: standardReferences},
		},
		Risks: risks, OutOfScope: append([]string(nil), session.OutOfScope...),
	}, nil
}

func (*LocalProvider) ReviewRole(_ context.Context, role string, session Session) (RoleReview, error) {
	knowledgeReferences, standardReferences := reviewReferences(session.RoleReview.Context)
	review := RoleReview{
		Role: role, Recommendation: "approve_with_conditions",
		KnowledgeReferences: knowledgeReferences, StandardReferences: standardReferences,
	}
	switch role {
	case RoleResearchAnalyst:
		review.Summary = "检查需求依据、用户价值与外部假设，现有材料可进入实施评审。"
		review.Findings = []ReviewFinding{{ID: "F1", Severity: "info", Statement: "计划已记录企业知识引用，但没有把外部竞品事实当作已验证结论。", Recommendation: "如商业决策依赖竞品结论，应另行提供可追溯调研证据。"}}
	case RoleProductManager:
		review.Summary = "检查目标、验收标准和范围边界，计划具备进入技术评审的基本条件。"
		review.Findings = []ReviewFinding{{ID: "F1", Severity: "warning", Statement: "实施结果必须逐项映射到会话中的验收标准和非目标。", Recommendation: "在测试与发布清单中保留验收标准标识。"}}
	case RoleBackendEngineer:
		review.Summary = "检查服务边界、接口、数据、兼容性和后端测试要求。"
		review.Findings = []ReviewFinding{{ID: "F1", Severity: "warning", Statement: "后端变更需要在编码前确认接口、数据和迁移影响。", Recommendation: "技术设计必须包含兼容策略、测试证据和失败回滚路径。"}}
	case RoleFrontendEngineer:
		review.Summary = "检查用户交互、接口契约和前端影响；无前端变更时也应显式记录该边界。"
		review.Findings = []ReviewFinding{{ID: "F1", Severity: "info", Statement: "计划未默认扩大到前端实现范围。", Recommendation: "若接口影响现有客户端，应补充契约兼容和用户体验验证。"}}
	case RoleOperationsEngineer:
		review.Summary = "检查部署、配置、可观测性、容量和回滚要求。"
		review.Findings = []ReviewFinding{{ID: "F1", Severity: "warning", Statement: "发布前必须具备观测指标、告警责任人和可执行回滚方案。", Recommendation: "把部署验证和回滚演练证据纳入发布门禁。"}}
	default:
		return RoleReview{}, fmt.Errorf("unsupported review role: %s", role)
	}
	return review, nil
}

func (*LocalProvider) Coordinate(_ context.Context, _ Session, reviews []RoleReview) (Coordination, error) {
	coordination := Coordination{
		Summary:       "五个角色已基于同一规划上下文完成独立只读评审；协调结果只汇总意见，不替代人工决策。",
		Consensus:     []string{"实施必须受验收标准、企业规范、测试证据和人工发布审批约束。"},
		DecisionItems: []DecisionItem{},
	}
	for _, review := range reviews {
		for _, finding := range review.Findings {
			if finding.Severity != "blocking" {
				continue
			}
			coordination.DecisionItems = append(coordination.DecisionItems, DecisionItem{
				ID: fmt.Sprintf("D%d", len(coordination.DecisionItems)+1), Topic: review.Role + " blocking finding",
				Description: finding.Statement, Options: []string{"接受建议并调整计划", "记录风险后继续", "拒绝当前计划"}, SourceRoles: []string{review.Role},
			})
		}
		for _, question := range review.OpenQuestions {
			coordination.DecisionItems = append(coordination.DecisionItems, DecisionItem{
				ID: fmt.Sprintf("D%d", len(coordination.DecisionItems)+1), Topic: review.Role + " open question",
				Description: question, Options: []string{"补充信息后继续", "明确排除该事项", "拒绝当前计划"}, SourceRoles: []string{review.Role},
			})
		}
		if review.Recommendation == "reject" && len(review.Findings) == 0 && len(review.OpenQuestions) == 0 {
			coordination.DecisionItems = append(coordination.DecisionItems, DecisionItem{
				ID: fmt.Sprintf("D%d", len(coordination.DecisionItems)+1), Topic: review.Role + " rejection",
				Description: "该角色不建议按当前计划继续。", Options: []string{"调整计划后重审", "记录风险后继续", "拒绝当前计划"}, SourceRoles: []string{review.Role},
			})
		}
	}
	return coordination, nil
}

func reviewReferences(snapshot ContextSnapshot) ([]string, []string) {
	knowledgeReferences := make([]string, 0, len(snapshot.Knowledge))
	for _, reference := range snapshot.Knowledge {
		knowledgeReferences = append(knowledgeReferences, reference.ID)
	}
	standardReferences := make([]string, 0, len(snapshot.Standards))
	for _, reference := range snapshot.Standards {
		standardReferences = append(standardReferences, reference.ID)
	}
	return knowledgeReferences, standardReferences
}

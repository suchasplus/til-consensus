import { memo, useCallback, useEffect, useRef, useState } from 'react'
import type { CSSProperties, FormEvent, KeyboardEvent, ReactNode } from 'react'
import { usePanelLayout } from './usePanelLayout'
import { CarbonSelect } from './CarbonSelect'
import type {
  ChatMessage,
  DesktopAPI,
  HostHello,
  InputMode,
  MaterialInput,
  PreparationRecord,
  ProviderInfo,
  ProviderConfig,
  Proposal,
  RunSnapshot,
  RunSummary,
  StartRunInput,
  WorkflowMode,
  RolePlan,
} from '../../../packages/protocol/src/index'

type IconName = 'plus' | 'arrow' | 'chevron' | 'layers' | 'chat' | 'check' | 'stop' | 'play' | 'close' | 'panel' | 'sidebar' | 'alert' | 'refresh' | 'settings' | 'download' | 'trash' | 'search'
type DetailTab = 'proposal' | 'issues' | 'votes'

function WorkspaceMark({ large = false }: { large?: boolean }) {
  return <div className={`brand-symbol${large ? ' large' : ''}`} aria-hidden="true"><svg width="28" height="28" viewBox="0 0 32 32" fill="none"><path d="M10 8h12M8 10v12M24 10v12M10 24h12" stroke="currentColor" strokeWidth="1.5" /><path d="m10 10 12 12M22 10 10 22" stroke="currentColor" strokeWidth="1.5" /><path d="M4 4h8v8H4zM20 4h8v8h-8zM4 20h8v8H4zM20 20h8v8h-8z" fill="currentColor" /></svg></div>
}

function ConsensusGraphic() {
  return <div className="welcome-art" aria-hidden="true"><svg className="consensus-graphic" width="160" height="104" viewBox="0 0 160 104" fill="none"><path className="graphic-grid" d="M0 16h160M0 40h160M0 64h160M0 88h160M20 0v104M44 0v104M68 0v104M92 0v104M116 0v104M140 0v104" /><path className="graphic-connection" d="M32 28h48v24h48M32 76h48V52" /><path className="graphic-node" d="M20 16h24v24H20zM20 64h24v24H20zM116 40h24v24h-24z" /><path className="graphic-primary" d="M66 38h28v28H66z" /><path className="graphic-check" d="m74 52 4 4 8-8" /></svg></div>
}

const modes: { value: InputMode; label: string; description: string; short: string }[] = [
  { value: 'auto', label: '自动选择', description: '先检查话题与材料，再选择适合的讨论方式', short: '自动' },
  { value: 'free_debate', label: '自由辩论', description: '独立评审，交叉讨论，再综合修订', short: '辩论' },
  { value: 'adjudication', label: '证据裁决', description: '提出主张，质疑验证，保留适用条件', short: '裁决' },
  { value: 'delphi', label: '德尔菲', description: '匿名评价候选，观察评分与意见收敛', short: '德尔菲' },
]
const statusLabels: Record<string, string> = {
  queued: '排队中', running: '讨论中', completed: '已完成', stopped: '已停止', interrupted: '已中断', failed: '运行失败',
}
const phaseLabels: Record<string, string> = {
  preparing: '准备讨论', proposal: '起草方案', proposing: '起草方案', p0: '起草方案',
  frame: '整理问题', ingest: '读取材料', initial: '独立评审', debate: '交叉讨论',
  synthesis: '综合修订', amend: '修正评审', final_vote: '最终投票', report: '生成报告', observe: '结果归档',
  propose: '提出主张', challenge: '质疑主张', verify: '证据验证', revise: '修订方案', adjudicate: '裁决主张',
  questionnaire: '匿名问卷', anonymous_summary: '匿名汇总', revision: '修正评分',
  engine_running: '协议执行中', finalizing: '汇总结论', completed: '讨论完成', stopped: '已停止',
  interrupted: '等待恢复', failed: '运行失败', queued: '等待开始', created: '讨论已创建',
  finished: '讨论完成', stopping: '正在停止', action: '整理后续行动',
  delphi_questionnaire: '匿名独立问卷', delphi_summary: '匿名汇总', delphi_revision: '匿名复评',
  delphi_facilitator_summary: '匿名汇总', initial_proposal: '独立评审', debate_round: '交叉讨论',
  semantic_dedup: '整理重复主张', semantic_verify: '语义验证', arbitrate: '裁决主张',
}
const outcomeLabels: Record<string, string> = {
  supported: '方案获支持', supported_with_caveats: '有条件支持', rejected: '不推荐', no_consensus: '尚未达成共识',
  quorum_not_met: '有效票不足', insufficient_evidence: '证据不足', needs_human: '需要人工决定',
  converged: '意见已收敛', accepted: '方案获支持', completed: '流程完成', failed: '判定失败',
}
const agents = [
  { id: 'codex', name: 'Codex', role: '起草 · 综合', initials: 'CX', color: 'green' },
  { id: 'claude', name: 'Claude Code', role: '独立评审', initials: 'CL', color: 'clay' },
  { id: 'grok', name: 'Grok Build', role: '独立评审', initials: 'GR', color: 'purple' },
  { id: 'agy', name: 'agy', role: '独立评审', initials: 'AG', color: 'blue' },
] as const
const examples = [
  { title: '做一个产品决策', text: '为一个面向独立开发者的任务管理工具设计 MVP，比较功能范围、实现成本和可验证的成功标准。', icon: 'layers' as IconName },
  { title: '评审一份技术方案', text: '为桌面多模型讨论工具设计可靠的停止与恢复机制，重点评审进程边界、持久化和重复执行风险。', icon: 'chat' as IconName },
  { title: '比较不同实现路线', text: '比较 SQLite、文件存储和本地 PostgreSQL 作为单用户桌面应用的数据层，给出适用条件与推荐方案。', icon: 'check' as IconName },
]

function Icon({ name, size = 18 }: { name: IconName; size?: number }) {
  const paths: Record<IconName, ReactNode> = {
    plus: <path d="M12 5v14M5 12h14" />,
    arrow: <><path d="M12 19V5M6 11l6-6 6 6" /></>,
    chevron: <path d="m9 5 7 7-7 7" />,
    layers: <><path d="m12 3 9 5-9 5-9-5 9-5Z" /><path d="m3 12 9 5 9-5M3 16l9 5 9-5" /></>,
    chat: <path d="M21 11.5a8.4 8.4 0 0 1-.9 3.8 8.5 8.5 0 0 1-7.6 4.7 8.4 8.4 0 0 1-3.8-.9L3 21l1.9-5.7a8.4 8.4 0 0 1-.9-3.8 8.5 8.5 0 0 1 4.7-7.6 8.4 8.4 0 0 1 3.8-.9h.5a8.5 8.5 0 0 1 8 8v.5Z" />,
    check: <path d="m5 12 4 4L19 6" />,
    stop: <rect x="6" y="6" width="12" height="12" rx="2" />,
    play: <path d="m8 4 12 8-12 8V4Z" />,
    close: <path d="m6 6 12 12M6 18 18 6" />,
    panel: <><rect x="3" y="4" width="18" height="16" rx="3" /><path d="M15 4v16" /></>,
    sidebar: <><rect x="3" y="4" width="18" height="16" rx="3" /><path d="M9 4v16" /></>,
    alert: <><path d="m10.3 3.8-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.7-3.2l-8-14a2 2 0 0 0-3.4 0Z" /><path d="M12 9v4M12 17h.01" /></>,
    refresh: <><path d="M20 7v5h-5M4 17v-5h5" /><path d="M6.1 7a7 7 0 0 1 11.5-2L20 8M4 16l2.4 3A7 7 0 0 0 18 17" /></>,
    settings: <><path d="M9 4h6l1 3 3 1v7l-3 1-1 4H9l-1-4-3-1V8l3-1 1-3Z" /><circle cx="12" cy="12" r="3" /></>,
    download: <><path d="M12 3v12m-5-5 5 5 5-5M4 16v5h16v-5" /></>,
    trash: <><path d="M3 6h18M9 6V3h6v3M5 6l1 15h12l1-15M10 10v7M14 10v7" /></>,
    search: <><circle cx="10.5" cy="10.5" r="6.5" /><path d="m16 16 5 5" /></>,
  }
  return <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">{paths[name]}</svg>
}

function phaseName(phase: string) {
  if (phaseLabels[phase]) return phaseLabels[phase]
  const base = Object.keys(phaseLabels).find((key) => phase.startsWith(`${key}.`) || phase.startsWith(`${key}_round`) || phase.startsWith(`${key}:`))
  return base ? phaseLabels[base] : phase || '准备开始'
}

function providerId(value: string, roles: RolePlan = legacyRolePlan) {
  const id = value.toLowerCase().replace(/-/g, '_')
  const expert = id.match(/^(?:expert|reviewer)_([a-z])$/)
  if (expert) return roles.reviewers[expert[1].charCodeAt(0) - 97] ?? 'system'
  if (id === 'facilitator') return roles.proposer
  if (id.includes('codex')) return 'codex'
  if (id.includes('claude') || id.includes('reviewer_a') || id === 'expert_a') return 'claude'
  if (id.includes('grok') || id.includes('reviewer_b') || id === 'expert_b') return 'grok'
  if (id.includes('agy') || id.includes('reviewer_c') || id === 'expert_c') return 'agy'
  return 'system'
}

const legacyRolePlan: RolePlan = { proposer: 'codex', reviewers: ['claude', 'grok', 'agy'] }
const providerEnabled = (provider: ProviderInfo) => provider.effective?.enabled ?? provider.enabled ?? true

function agentLabel(id: string, anonymous: boolean, roles: RolePlan = legacyRolePlan) {
  const provider = providerId(id, roles)
  if (anonymous) {
    if (provider === roles.proposer) return '提案协调器'
    const index = roles.reviewers.indexOf(provider)
    return index >= 0 ? `专家 ${String.fromCharCode(65 + index)}` : '协调器'
  }
  return agents.find((agent) => agent.id === provider)?.name ?? '协调器'
}

function publicText(text: string, anonymous: boolean, roles: RolePlan = legacyRolePlan) {
  if (!anonymous) return text
  return text.replace(/Claude\s*Code/gi, agentLabel('claude', true, roles)).replace(/\bclaude(?:[-_][\w.-]+)?\b/gi, agentLabel('claude', true, roles))
    .replace(/Grok\s*Build/gi, agentLabel('grok', true, roles)).replace(/\bgrok(?:[-_][\w.-]+)?\b/gi, agentLabel('grok', true, roles))
    .replace(/\b(?:agy|antigravity|gemini(?:[-_][\w.-]+)?)\b/gi, agentLabel('agy', true, roles))
    .replace(/\bgpt[-_][\w.-]+\b/gi, '协调模型').replace(/Codex/gi, agentLabel('codex', true, roles))
}

function dateLabel(value: string) {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '' : new Intl.DateTimeFormat('zh-CN', { month: 'numeric', day: 'numeric', hour: '2-digit', minute: '2-digit' }).format(date)
}

function isActive(run?: RunSummary | null) {
  return run?.status === 'running' || run?.status === 'queued'
}

function errorMessage(error: unknown) {
  const message = error instanceof Error ? error.message : String(error)
  return message.replace(/^Error invoking remote method '[^']+':\s*(?:Error:\s*)?/, '').replace(/^Error:\s*/, '')
}

const preparationProviders = ['agy', 'codex', 'claude', 'grok'] as const
const materialBytes = (value: string) => new TextEncoder().encode(value).byteLength
const sizeLabel = (bytes: number) => bytes < 1024 ? `${bytes} B` : `${(bytes / 1024).toFixed(1)} KiB`
const preparationActive = (record: PreparationRecord) => record.status === 'created' || record.status === 'running'
const inputSignature = (goal: string, materials: MaterialInput[], provider: string) => JSON.stringify([goal.trim(), materials, provider])
const concreteRecommendation = (value?: string): value is WorkflowMode => ['free_debate', 'adjudication', 'delphi'].includes(value ?? '')

function validateMaterials(materials: MaterialInput[]) {
  if (materials.length > 8) return '最多添加 8 份材料，包含关联讨论继承的材料。'
  let total = 0
  for (const material of materials) {
    const bytes = materialBytes(material.content)
    if (!material.content.trim()) return `“${material.title}”没有文本内容。`
    if (bytes > 64 * 1024) return `“${material.title}”超过每份 64 KiB 的上限。`
    total += bytes
  }
  return total > 128 * 1024 ? '材料合计超过 128 KiB，请删减后再添加。' : null
}

function PreparationCard({ record, mock, onRewrite, materialCatalog = [] }: { record: PreparationRecord; mock: boolean; onRewrite?: (value: string) => void; materialCatalog?: MaterialInput[] }) {
  const decision = record.classification
  const analysis = record.materialAnalysis
  const materialTitle = (id: string) => record.materials.find((material) => material.id === id)?.title ?? materialCatalog.find((material) => material.id === id)?.title ?? '附加材料'
  const status = ({ created: '准备检查', running: '正在分析', completed: '检查完成', failed: '检查失败', cancelled: '已取消', interrupted: '已中断' })[record.status]
  const recommendation = decision && (modes.find((item) => item.value === decision.recommendation)?.label ?? (decision.recommendation === 'needs_clarification' ? '需要补充信息' : '无需多方讨论'))
  const number = (value?: number) => typeof value === 'number' && Number.isFinite(value) ? new Intl.NumberFormat('zh-CN').format(value) : '未报告'
  return <section className={`preparation-card preparation-${record.status}`} data-testid="preparation-card" data-preparation-id={record.id} aria-label="话题与材料检查结果">
    <div className="preparation-heading"><span><Icon name="layers" size={16} /><strong>话题与材料检查</strong>{(mock || record.source?.mock) && <span className="inline-mock">Mock</span>}</span><span data-testid="preparation-status" role="status">{preparationActive(record) && <span className="spinner" />}{status}</span></div>
    {preparationActive(record) && <p className="preparation-pending">正在理解话题、整理材料并判断适合的讨论方式。你可以取消，或修改输入后重新检查。</p>}
    {record.error && <p className="preparation-error" role="alert">{record.error}</p>}
    {decision && <>
      <div className="preparation-recommendation"><strong data-testid="preparation-recommendation">{recommendation}</strong><span>信心 {Math.round(decision.confidence * 100)}%</span></div>
      <p>{decision.summary}</p><ul className="preparation-reasons">{decision.why.map((reason, index) => <li key={index}>{reason}</li>)}</ul>
      {decision.recommendation === 'needs_clarification' && <div className="preparation-guidance" data-testid="clarification-questions"><strong>先补充这些信息</strong><ul>{decision.missingInformation.map((question, index) => <li key={index}>{question}</li>)}</ul>{decision.estimatedModeAfterClarification && <p>补充后可能适合：{modes.find((item) => item.value === decision.estimatedModeAfterClarification)?.label}。{decision.estimatedModeReason} 这是预估，不会据此直接开始讨论。</p>}</div>}
      {decision.recommendation === 'not_suitable' && <div className="preparation-guidance" data-testid="not-suitable-notice">这个话题不需要多方讨论，可以直接处理或进一步说明希望比较的分歧。</div>}
      {concreteRecommendation(decision.recommendation) && decision.confidence < 0.6 && <div className="preparation-guidance" data-testid="low-confidence-notice">信心低于 60%，暂不自动开始。请补充上下文，或手动选择讨论模式。</div>}
      {analysis?.gaps.some((gap) => gap.importance === 'required') && <div className="preparation-guidance" data-testid="required-material-gaps">材料仍有需要补充的信息，自动模式暂不开始。请根据下方缺口补充原文或约束后重新检查。</div>}
      {record.status === 'completed' && !record.usedByRunId && Date.now() - Date.parse(record.createdAt) >= 15 * 60_000 && <div className="preparation-guidance" data-testid="expired-preparation-notice">检查已超过 15 分钟。请重新点击“检查话题与材料”，不会自动重试或采用过期结果。</div>}
      {decision.suggestedTask && <div className="preparation-rewrite"><span>建议任务表述</span><p>{decision.suggestedTask}</p>{onRewrite && <button type="button" className="subtle-button" data-testid="adopt-rewrite-button" onClick={() => onRewrite(decision.suggestedTask)}>采用改写</button>}</div>}
    </>}
    {analysis && <div className="material-analysis" data-testid="material-analysis"><h3>材料整理</h3><p>{analysis.summary}</p>{analysis.materials.map((entry) => <details key={entry.materialId} className="material-analysis-item" open><summary>{materialTitle(entry.materialId)}</summary><p>{entry.summary}</p>{entry.keyPoints.map((point, index) => <div className="material-key-point" key={index}><span>原文摘引</span><blockquote>{point.quote}</blockquote><p>{point.meaning}</p></div>)}</details>)}{analysis.gaps.length > 0 && <div className="material-gaps"><h4>信息缺口</h4>{analysis.gaps.map((gap, index) => <p key={index}><strong>{gap.importance === 'required' ? '需要补充' : '有助于判断'}</strong>{gap.question}{gap.materialIds.length > 0 && <small>关联材料：{gap.materialIds.map(materialTitle).join('、')}</small>}</p>)}</div>}{analysis.limitations.length > 0 && <ul>{analysis.limitations.map((item, index) => <li key={index}>{item}</li>)}</ul>}</div>}
    {record.materials.length > 0 && <details className="preparation-materials"><summary>本次检查的原文与指纹 · {record.materials.length} 份</summary>{record.materials.map((material) => <article key={material.id}><strong>{material.title}</strong><span>{sizeLabel(material.byteLength)} · {material.source === 'file' ? '本机文本文件' : '粘贴文本'}</span><code title={material.sha256}>SHA-256 {material.sha256}</code><pre>{material.content}</pre></article>)}</details>}
    {record.source && <details className="preparation-source"><summary>{agentLabel(record.source.providerId, false)} · {record.source.actualModel || '实际模型未报告'} · {(record.source.elapsedMs / 1000).toFixed(1)} 秒</summary><dl><div><dt>请求模型</dt><dd>{record.source.requestedModel || 'CLI 默认'}</dd></div><div><dt>实际报告</dt><dd>{record.source.actualModel || '未报告'} · {record.source.modelSource}</dd></div><div><dt>CLI 版本</dt><dd>{record.source.cliVersion || '未报告'}</dd></div></dl></details>}
    {(record.usage || record.costUsd !== undefined) && <div className="preparation-usage">本次检查 {number(record.usage?.calls)} 次请求 · 输入 {number(record.usage?.inputTokens)} / 输出 {number(record.usage?.outputTokens)} Token · {typeof record.costUsd === 'number' ? `$${record.costUsd.toFixed(4)}` : '费用未报告'}</div>}
    <p className="preparation-footnote">{record.usedByRunId ? '已采用的检查请求计入本次 30 次 / 关联链 90 次上限。' : '这是独立准备记录；采用后计入本次 30 次 / 关联链 90 次上限，15 分钟期限从检查开始计。'} 开始讨论后，材料原文交给提案者与全部参与者；分析建议不代表事实核验。</p>
  </section>
}

const FormattedText = memo(function FormattedText({ text }: { text: string }) {
  return <div className="formatted-text">{text.split('\n').map((line, index) => {
    const heading = line.match(/^#{1,4}\s+(.+)$/)
    if (heading) return <h4 key={index}>{heading[1]}</h4>
    const parts = line.split(/(\*\*[^*]+\*\*|`[^`]+`)/g).map((part, partIndex) =>
      part.startsWith('**') ? <strong key={partIndex}>{part.slice(2, -2)}</strong> : part.startsWith('`') ? <code key={partIndex}>{part.slice(1, -1)}</code> : part,
    )
    if (/^[-*]\s/.test(line)) return <div className="text-list-item" key={index}><span aria-hidden="true">·</span><span>{parts.map((part, partIndex) => partIndex === 0 && typeof part === 'string' ? part.replace(/^[-*]\s/, '') : part)}</span></div>
    return line.trim() ? <p key={index}>{parts}</p> : <div className="text-gap" key={index} />
  })}</div>
})

function ProposalDiff({ previous, current, anonymous, roles }: { previous: Proposal; current: Proposal; anonymous: boolean; roles?: RolePlan }) {
  const before = publicText(previous.content, anonymous, roles).split('\n')
  const after = publicText(current.content, anonymous, roles).split('\n')
  // 使用有序公共行作为锚点；超长文档截断展示，完整版本仍可导出。
  const rows: { kind: 'same' | 'added' | 'removed'; text: string }[] = []
  let a = 0
  let b = 0
  while (a < before.length || b < after.length) {
    if (a < before.length && b < after.length && before[a] === after[b]) {
      rows.push({ kind: 'same', text: before[a++] }); b++; continue
    }
    const nextBefore = b < after.length ? before.indexOf(after[b], a) : -1
    const nextAfter = a < before.length ? after.indexOf(before[a], b) : -1
    if (nextBefore >= 0 && (nextAfter < 0 || nextBefore - a <= nextAfter - b)) {
      while (a < nextBefore) rows.push({ kind: 'removed', text: before[a++] })
    } else if (nextAfter >= 0) {
      while (b < nextAfter) rows.push({ kind: 'added', text: after[b++] })
    } else {
      if (a < before.length) rows.push({ kind: 'removed', text: before[a++] })
      if (b < after.length) rows.push({ kind: 'added', text: after[b++] })
    }
  }
  const added = rows.filter((row) => row.kind === 'added').length
  const removed = rows.filter((row) => row.kind === 'removed').length
  return <details className="proposal-diff"><summary>比较 P{previous.version} → P{current.version}<span>+{added} −{removed} 行</span></summary><div className="diff-lines" aria-label="方案逐行差异">{rows.slice(0, 1500).map((row, index) => <div key={index} className={`diff-line ${row.kind}`}><span aria-label={row.kind === 'added' ? '新增' : row.kind === 'removed' ? '删除' : '保留'}>{row.kind === 'added' ? '+' : row.kind === 'removed' ? '−' : ' '}</span><code>{row.text || ' '}</code></div>)}{rows.length > 1500 && <p>文档较长，界面仅展示前 1,500 行；导出可查看完整内容。</p>}</div></details>
}

function DelphiDistribution({ snapshot }: { snapshot: RunSnapshot }) {
  const candidates = snapshot.claims.filter((claim) => claim.category === 'candidate')
  if (!candidates.length) return <div className="mode-notice">匿名问卷进行中。候选、评分和分歧会随结果更新。</div>
  return <section className="delphi-distribution" aria-label="匿名候选评分分布"><div className="section-label">匿名评分分布<span>1–5 分</span></div>{candidates.map((claim, index) => {
    const votes = snapshot.decision?.votes.filter((vote) => vote.claimId === claim.id && Number.isFinite(vote.support) && vote.support >= 1 && vote.support <= 5) ?? []
    const scores = (snapshot.delphiDistribution?.[claim.id] ?? votes.map((vote) => vote.support)).filter((score) => Number.isFinite(score) && score >= 1 && score <= 5)
    const mean = scores.length ? scores.reduce((total, value) => total + value, 0) / scores.length : claim.supportScore
    const buckets = [1, 2, 3, 4, 5].map((score) => scores.filter((value) => Math.round(value) === score).length)
    const consensus = scores.length >= 2 ? 1 - (Math.max(...scores) - Math.min(...scores)) / 4 : undefined
    return <div className="distribution-card" key={claim.id}><div className="distribution-heading"><strong>候选 {String(index + 1).padStart(2, '0')}</strong><span>{mean === undefined ? '等待评分' : `${mean.toFixed(1)} / 5`}</span></div><p>{publicText(claim.statement, true, snapshot.rolePlan)}</p>{scores.length ? <><div className="distribution-bars" role="img" aria-label={`评分人数：${buckets.map((count, i) => `${i + 1} 分 ${count} 人`).join('，')}`}>{buckets.map((count, i) => <div key={i}><span className="distribution-column"><i style={{ height: `${count ? Math.max(10, count / Math.max(...buckets) * 100) : 0}%` }} /><b>{count || ''}</b></span><small>{i + 1}</small></div>)}</div><div className="distribution-meta"><span>{scores.length} 份有效评分</span><span>{consensus === undefined ? '收敛度待计算' : `收敛度 ${Math.round(consensus * 100)}%`}</span></div></> : <p className="distribution-empty">{mean === undefined ? '尚无有效评分。' : '已获得均分；评分分布尚不可用。'}</p>}{mean !== undefined && mean < 4 && <div className="score-warning">均分未达到推荐阈值 4 分；意见一致不等于推荐。</div>}</div>
  })}</section>
}

function App() {
  const api: DesktopAPI | null = typeof window.tcview !== 'undefined' ? window.tcview : null
  const [hello, setHello] = useState<HostHello | null>(null)
  const [connection, setConnection] = useState<'loading' | 'ready' | 'error'>('loading')
  const layout = usePanelLayout()
  const [runs, setRuns] = useState<RunSummary[]>([])
  const [deletedRuns, setDeletedRuns] = useState<RunSummary[]>([])
  const [historyView, setHistoryView] = useState<'recent' | 'deleted'>('recent')
  const [historyQuery, setHistoryQuery] = useState('')
  const [historyBusy, setHistoryBusy] = useState<string | null>(null)
  const [lastDeletedId, setLastDeletedId] = useState<string | null>(null)
  const historySearch = useRef<HTMLInputElement>(null)
  const [awayFromBottom, setAwayFromBottom] = useState(false)
  const [activeId, setActiveId] = useState<string | null>(null)
  const [snapshot, setSnapshot] = useState<RunSnapshot | null>(null)
  const [loadingRun, setLoadingRun] = useState(false)
  const [mode, setMode] = useState<InputMode>('auto')
  const [draft, setDraft] = useState('')
  const [materials, setMaterials] = useState<MaterialInput[]>([])
  const [materialError, setMaterialError] = useState<string | null>(null)
  const [pasteOpen, setPasteOpen] = useState(false)
  const [pasteTitle, setPasteTitle] = useState('')
  const [pasteContent, setPasteContent] = useState('')
  const [importingMaterials, setImportingMaterials] = useState(false)
  const [analysisProvider, setAnalysisProvider] = useState<string>('agy')
  const [preparation, setPreparation] = useState<PreparationRecord | null>(null)
  const [preparing, setPreparing] = useState(false)
  const [cancellingPreparation, setCancellingPreparation] = useState(false)
  const [pending, setPending] = useState<'start' | 'stop' | 'resume' | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [tab, setTab] = useState<DetailTab>('proposal')
  const [detailsOpen, setDetailsOpen] = useState(false)
  const artifactsVisible = layout.narrow ? detailsOpen : !layout.rightCollapsed
  const [contextExpanded, setContextExpanded] = useState(false)
  const [composerExpanded, setComposerExpanded] = useState(true)
  const [composerDockHeight, setComposerDockHeight] = useState(38)
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [providers, setProviders] = useState<ProviderInfo[]>([])
  const [providersLoading, setProvidersLoading] = useState(false)
  const [checkingProvider, setCheckingProvider] = useState<string | null>(null)
  const [providerBatch, setProviderBatch] = useState<ProviderCheckBatch | null>(null)
  const [settingsBusy, setSettingsBusy] = useState(false)
  const [settingsError, setSettingsError] = useState<string | null>(null)
  const [settingsNotice, setSettingsNotice] = useState<string | null>(null)
  const [cleaningLogs, setCleaningLogs] = useState(false)
  const [selectedVersion, setSelectedVersion] = useState<number | null>(null)
  const [continueDiscussion, setContinueDiscussion] = useState(false)
  const [notice, setNotice] = useState<string | null>(null)
  const [exporting, setExporting] = useState<'markdown' | 'json' | null>(null)
  const [now, setNow] = useState(Date.now())
  const [providerProgress, setProviderProgress] = useState<Record<string, Record<string, { kind: string; status: string }>>>({})
  const activeRef = useRef<string | null>(null)
  const cache = useRef(new Map<string, RunSnapshot>())
  const requestVersions = useRef(new Map<string, number>())
  const listVersion = useRef(0)
  const seenEvents = useRef(new Map<string, number>())
  const interacted = useRef(false)
  const actionLock = useRef(false)
  const providerCheckLock = useRef(false)
  const providerBatchStop = useRef(false)
  const startRequest = useRef<StartRunInput | null>(null)
  const resumeKeys = useRef(new Map<string, string>())
  const branchRequest = useRef<{ runId: string; goal: string; materials: MaterialInput[]; idempotencyKey: string } | null>(null)
  const inputVersion = useRef(0)
  const operationVersion = useRef(0)
  const preparedSignature = useRef<string | null>(null)
  const analysisRequest = useRef<{ id: string; key: string; promise: Promise<PreparationRecord | null> } | null>(null)
  const startOperation = useRef<number | null>(null)
  const textarea = useRef<HTMLTextAreaElement>(null)
  const composerDock = useRef<HTMLDivElement>(null)
  const composerToggle = useRef<HTMLButtonElement>(null)
  const historyPanelToggle = useRef<HTMLButtonElement>(null)
  const artifactsPanelToggle = useRef<HTMLButtonElement>(null)
  const bottom = useRef<HTMLDivElement>(null)
  const chatScroll = useRef<HTMLDivElement>(null)
  const shouldScroll = useRef(true)

  useEffect(() => {
    const node = composerDock.current
    if (!node) return
    const measure = () => setComposerDockHeight(Math.ceil(node.getBoundingClientRect().height))
    const observer = new ResizeObserver(measure)
    observer.observe(node)
    measure()
    return () => observer.disconnect()
  }, [])

  useEffect(() => {
    if (!shouldScroll.current) return
    const frame = requestAnimationFrame(() => bottom.current?.scrollIntoView({ behavior: 'auto', block: 'end' }))
    return () => cancelAnimationFrame(frame)
  }, [contextExpanded, composerExpanded, composerDockHeight])

  const openComposer = () => {
    setComposerExpanded(true)
    requestAnimationFrame(() => textarea.current?.focus({ preventScroll: true }))
  }

  const closeHistoryPanel = () => {
    layout.setCollapsed('left', true)
    requestAnimationFrame(() => {
      const focused = document.activeElement
      if (!focused || focused === document.body || document.getElementById('history-panel')?.contains(focused)) historyPanelToggle.current?.focus({ preventScroll: true })
    })
  }
  const toggleHistoryPanel = () => {
    if (!layout.leftCollapsed) { closeHistoryPanel(); return }
    layout.setCollapsed('left', false)
  }
  const openArtifacts = (nextTab?: DetailTab) => {
    layout.setCollapsed('right', false)
    setDetailsOpen(layout.narrow)
    if (!layout.narrow && nextTab) requestAnimationFrame(() => document.getElementById(`tab-${nextTab}`)?.focus({ preventScroll: true }))
  }
  const closeArtifacts = () => {
    layout.setCollapsed('right', true)
    setDetailsOpen(false)
    requestAnimationFrame(() => {
      const focused = document.activeElement
      if (!focused || focused === document.body || document.getElementById('artifacts-panel')?.contains(focused)) artifactsPanelToggle.current?.focus({ preventScroll: true })
    })
  }

  // 响应式抽屉的临时关闭不改写用户的桌面折叠偏好。
  useEffect(() => { setDetailsOpen(false) }, [layout.narrow])

  const invalidatePreparation = useCallback(() => {
    inputVersion.current++
    operationVersion.current++
    const request = analysisRequest.current
    analysisRequest.current = null
    if (request && api) void api.cancelPreparation(request.id).catch(() => undefined)
    if (startOperation.current !== null) {
      actionLock.current = false
      startOperation.current = null
      setPending(null)
    }
    preparedSignature.current = null
    startRequest.current = null
    branchRequest.current = null
    setPreparation(null)
    setPreparing(false)
    setCancellingPreparation(false)
  }, [api])

  const changeDraft = (value: string) => {
    invalidatePreparation()
    setDraft(value)
  }

  const applySnapshot = useCallback((next: RunSnapshot) => {
    const previous = cache.current.get(next.id)
    if (previous && previous.lastEventId > next.lastEventId) return
    cache.current.set(next.id, next)
    if (activeRef.current === next.id) {
      setSnapshot(next)
      setLoadingRun(false)
    }
    setRuns((current) => (next.deletedAt ? current.filter((run) => run.id !== next.id) : [next, ...current.filter((run) => run.id !== next.id)]).sort((a, b) => b.updatedAt.localeCompare(a.updatedAt)))
    setDeletedRuns((current) => (next.deletedAt ? [next, ...current.filter((run) => run.id !== next.id)] : current.filter((run) => run.id !== next.id)).sort((a, b) => (b.deletedAt ?? b.updatedAt).localeCompare(a.deletedAt ?? a.updatedAt)))
  }, [])

  const refreshList = useCallback(async () => {
    if (!api) return
    const version = ++listVersion.current
    const [next, deleted] = await Promise.all([api.listRuns(), api.listDeletedRuns()])
    if (version === listVersion.current) {
      setRuns(next.filter((run) => !run.deletedAt).sort((a, b) => b.updatedAt.localeCompare(a.updatedAt)))
      setDeletedRuns(deleted.sort((a, b) => (b.deletedAt ?? b.updatedAt).localeCompare(a.deletedAt ?? a.updatedAt)))
    }
    return next
  }, [api])

  const refreshRun = useCallback(async (id: string, quiet = false) => {
    if (!api) return
    const version = (requestVersions.current.get(id) ?? 0) + 1
    requestVersions.current.set(id, version)
    try {
      const cursor = cache.current.get(id)?.lastEventId ?? 0
      await api.getEvents(id, cursor)
      const next = await api.getRun(id)
      if (requestVersions.current.get(id) === version) applySnapshot(next)
    } catch (failure) {
      if (activeRef.current === id && requestVersions.current.get(id) === version) {
        setLoadingRun(false)
        if (!quiet) setError(`读取讨论失败：${errorMessage(failure)}`)
      }
    }
  }, [api, applySnapshot])

  const selectRun = useCallback((id: string) => {
    invalidatePreparation()
    interacted.current = true
    activeRef.current = id
    setActiveId(id)
    setSnapshot(cache.current.get(id) ?? null)
    setLoadingRun(!cache.current.has(id))
    setError(null)
    setTab('proposal')
    setSelectedVersion(null)
    setContinueDiscussion(false)
    setContextExpanded(false)
    setComposerExpanded(false)
    setMode('auto')
    setMaterials([])
    setMaterialError(null)
    setPasteOpen(false)
    setPasteTitle('')
    setPasteContent('')
    setDraft('')
    shouldScroll.current = true
    setAwayFromBottom(false)
    void refreshRun(id)
  }, [refreshRun, invalidatePreparation])

  const initialize = useCallback(async () => {
    if (!api) return
    setConnection('loading')
    setError(null)
    try {
      const [host, history, providerList] = await Promise.all([api.hello(), refreshList(), api.listProviders().catch(() => [])])
      setHello(host)
      setProviders(providerList)
      setConnection('ready')
      if (!interacted.current && history?.length) selectRun(history[0].id)
    } catch (failure) {
      setConnection('error')
      setError(`Go 后台连接失败：${errorMessage(failure)}`)
    }
  }, [api, refreshList, selectRun])

  useEffect(() => { void initialize() }, [initialize])

  useEffect(() => {
    if (!api) return
    let refreshTimer: ReturnType<typeof setTimeout> | undefined
    const unsubscribe = api.subscribe((event) => {
      if (event.type === 'host.disconnected') {
        setConnection('error')
        setError(`Go 后台连接已断开：${typeof event.payload.message === 'string' ? event.payload.message : '请重新连接后台'}`)
        return
      }
      if (event.eventId <= (seenEvents.current.get(event.runId) ?? 0)) return
      seenEvents.current.set(event.runId, event.eventId)
      if (['provider.progress', 'message.delta'].includes(event.type) && typeof event.payload.agentId === 'string') {
        const id = providerId(event.payload.agentId)
        const kind = typeof event.payload.kind === 'string' ? event.payload.kind : ''
        const status = typeof event.payload.status === 'string' ? event.payload.status : ''
        setProviderProgress((current) => current[event.runId]?.[id]?.kind === kind && current[event.runId]?.[id]?.status === status ? current : ({ ...current, [event.runId]: { ...current[event.runId], [id]: { kind, status } } }))
      }
      if (refreshTimer) return
      refreshTimer = setTimeout(() => {
        refreshTimer = undefined
        void refreshList().catch(() => undefined)
        if (activeRef.current) void refreshRun(activeRef.current, true)
      }, 70)
    })
    const reconcile = () => {
      void refreshList().catch(() => undefined)
      if (activeRef.current) void refreshRun(activeRef.current, true)
    }
    const timer = setInterval(reconcile, 5000)
    window.addEventListener('focus', reconcile)
    return () => {
      unsubscribe()
      clearTimeout(refreshTimer)
      clearInterval(timer)
      window.removeEventListener('focus', reconcile)
    }
  }, [api, refreshList, refreshRun])

  useEffect(() => {
    if (shouldScroll.current) bottom.current?.scrollIntoView({ behavior: 'auto', block: 'end' })
  }, [snapshot?.messages.length, snapshot?.lastEventId, snapshot?.status, activeId])

  useEffect(() => {
    if (preparation) chatScroll.current?.querySelector('.draft-preparation')?.scrollIntoView({ behavior: 'auto', block: 'start' })
  }, [preparation?.id, preparation?.status])

  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 15_000)
    return () => clearInterval(timer)
  }, [])

  useEffect(() => {
    if (!notice) return
    const timer = setTimeout(() => setNotice(null), 6000)
    return () => clearTimeout(timer)
  }, [notice])

  useEffect(() => {
    if (!detailsOpen || !layout.narrow) return
    const previousFocus = document.activeElement as HTMLElement | null
    document.getElementById(`tab-${tab}`)?.focus()
    return () => {
      const focused = document.activeElement
      if (!focused || focused === document.body || document.getElementById('artifacts-panel')?.contains(focused)) previousFocus?.focus({ preventScroll: true })
    }
  }, [detailsOpen, layout.narrow])

  useEffect(() => {
    if (!textarea.current || !composerExpanded) return
    textarea.current.style.height = 'auto'
    textarea.current.style.height = `${Math.min(textarea.current.scrollHeight, 150)}px`
  }, [draft, composerExpanded])

  useEffect(() => () => {
    inputVersion.current++
    const request = analysisRequest.current
    if (request && api) void api.cancelPreparation(request.id).catch(() => undefined)
  }, [api])

  const enabledProviders = agents.flatMap((agent) => { const provider = providers.find((entry) => entry.id === agent.id); return provider && providerEnabled(provider) ? [provider] : [] })
  const enabledIds = enabledProviders.map((provider) => provider.id)
  const enabledSignature = enabledIds.join(',')
  const discussionReady = providers.length > 0 && enabledIds.length >= 3
  const analysisOptions = preparationProviders.filter((id) => enabledIds.includes(id))
  useEffect(() => {
    if (!providers.length || enabledIds.includes(analysisProvider)) return
    invalidatePreparation()
    const next = preparationProviders.find((id) => enabledIds.includes(id)) ?? ''
    setAnalysisProvider(next)
    setNotice(next ? `原检查提供方已停用，已切换为 ${agentLabel(next, false)}；请重新检查话题与材料。` : '全部 CLI 已停用。启用至少一家后可检查话题，启用至少三家后可开始讨论。')
  }, [enabledSignature, providers.length, analysisProvider, invalidatePreparation])

  const addMaterials = (incoming: MaterialInput[]) => {
    const inherited = continueDiscussion ? snapshot?.materials ?? [] : []
    const next = [...materials, ...incoming]
    const problem = validateMaterials([...inherited, ...next])
    if (problem) { setMaterialError(problem); return false }
    invalidatePreparation()
    setMaterials(next)
    setMaterialError(null)
    return true
  }

  const importMaterials = async () => {
    if (!api || importingMaterials) return
    if (analysisRequest.current) invalidatePreparation()
    const version = inputVersion.current
    setImportingMaterials(true)
    setMaterialError(null)
    try {
      const selected = await api.importMaterials()
      if (inputVersion.current === version && selected.length) addMaterials(selected)
    } catch (failure) {
      if (inputVersion.current === version) setMaterialError(`读取文本文件失败：${errorMessage(failure)}`)
    } finally { setImportingMaterials(false) }
  }

  const ensurePreparation = async (force = false): Promise<PreparationRecord | null> => {
    if (!api || !draft.trim()) return null
    if (!analysisProvider || !enabledIds.includes(analysisProvider)) { setError('请先在 CLI 设置中启用检查提供方。'); return null }
    const goal = draft.trim()
    const key = inputSignature(goal, materials, analysisProvider)
    const problem = validateMaterials(materials)
    if (problem) { setMaterialError(problem); return null }
    if (!force && preparation?.status === 'completed' && preparedSignature.current === key) {
      if (Date.now() - Date.parse(preparation.createdAt) >= 15 * 60_000) {
        setError('这份检查已超过 15 分钟，请点击“检查话题与材料”重新分析后再开始。')
        return null
      }
      return preparation
    }
    if (analysisRequest.current?.key === key) return analysisRequest.current.promise
    const version = inputVersion.current
    const id = crypto.randomUUID()
    const at = new Date().toISOString()
    setPreparing(true)
    setError(null)
    preparedSignature.current = null
    setPreparation({ id, goal, materials: [], inputHash: '', requestHash: '', status: 'running', createdAt: at, updatedAt: at })
    const current = () => inputVersion.current === version && analysisRequest.current?.id === id
    const promise = (async () => {
      try {
        await Promise.resolve()
        let record = await api.analyzeTopic({ id, goal, materials: [...materials], providerId: analysisProvider })
        const deadline = Date.now() + 150_000
        while (current() && preparationActive(record)) {
          setPreparation(record)
          if (Date.now() >= deadline) throw new Error('检查等待超时，请查看准备记录；不会自动重试。')
          await new Promise((resolve) => setTimeout(resolve, 350))
          if (!current()) return null
          record = await api.getPreparation(id)
        }
        if (!current()) return null
        setPreparation(record)
        if (record.status === 'failed' || record.status === 'interrupted') setComposerExpanded(true)
        if (record.status === 'completed') preparedSignature.current = key
        return record
      } catch (failure) {
        if (!current()) return null
        const stored = await api.getPreparation(id).catch(() => null)
        if (!current()) return null
        setPreparation(stored ?? { id, goal, materials: [], inputHash: '', requestHash: '', status: 'failed', createdAt: at, updatedAt: new Date().toISOString(), error: errorMessage(failure) })
        setComposerExpanded(true)
        setError(`话题与材料检查失败：${errorMessage(failure)}`)
        return null
      } finally {
        if (current()) {
          analysisRequest.current = null
          setPreparing(false)
        }
      }
    })()
    analysisRequest.current = { id, key, promise }
    return promise
  }

  const cancelPreparation = async () => {
    const request = analysisRequest.current
    if (!api || !request || cancellingPreparation) return
    const version = ++inputVersion.current
    operationVersion.current++
    analysisRequest.current = null
    preparedSignature.current = null
    startOperation.current = null
    actionLock.current = false
    setPending(null)
    setCancellingPreparation(true)
    try {
      const record = await api.cancelPreparation(request.id)
      if (inputVersion.current === version) setPreparation(record)
    } catch (failure) {
      if (inputVersion.current === version) setError(`取消状态未能确认：${errorMessage(failure)}`)
    } finally {
      if (inputVersion.current === version) {
        setPreparing(false)
        setCancellingPreparation(false)
      }
    }
  }

  const newDiscussion = () => {
    invalidatePreparation()
    interacted.current = true
    activeRef.current = null
    setActiveId(null)
    setSnapshot(null)
    setLoadingRun(false)
    setError(null)
    setDetailsOpen(false)
    setContextExpanded(false)
    setComposerExpanded(true)
    setMode('auto')
    setDraft('')
    setMaterials([])
    setMaterialError(null)
    setPasteOpen(false)
    setPasteTitle('')
    setPasteContent('')
    setContinueDiscussion(false)
    setSelectedVersion(null)
    setHistoryView('recent')
    setAwayFromBottom(false)
    setTimeout(() => textarea.current?.focus(), 0)
  }

  const deleteDiscussion = async (run: RunSummary) => {
    if (!api || historyBusy) return
    if (isActive(run)) { setError('请先停止讨论，等待 CLI 清理完成后再删除。'); return }
    setHistoryBusy(run.id)
    try {
      const deleted = await api.deleteRun(run.id)
      applySnapshot(deleted)
      setLastDeletedId(run.id)
      if (activeRef.current === run.id) {
        const next = runs.find((entry) => entry.id !== run.id)
        if (next) selectRun(next.id)
        else newDiscussion()
      }
      setNotice('讨论已移到最近删除，可以恢复；关联链预算与审计记录继续保留。')
      await refreshList()
    } catch (failure) { setError(`删除讨论失败：${errorMessage(failure)}`) }
    finally { setHistoryBusy(null) }
  }

  const restoreDiscussion = async (id: string) => {
    if (!api || historyBusy) return
    setHistoryBusy(id)
    try {
      const restored = await api.restoreRun(id)
      applySnapshot(restored)
      setHistoryView('recent')
      setHistoryQuery('')
      setLastDeletedId(null)
      selectRun(id)
      setNotice('讨论已恢复到最近讨论。')
      await refreshList()
    } catch (failure) { setError(`恢复讨论失败：${errorMessage(failure)}`) }
    finally { setHistoryBusy(null) }
  }

  const start = async (event: FormEvent) => {
    event.preventDefault()
    if (!api || !draft.trim() || actionLock.current || checkingProvider || connection !== 'ready' || runs.some(isActive)) return
    if (preparing || cancellingPreparation || importingMaterials) return
    if (!discussionReady) { setError('开始讨论至少需要 3 家已启用的 CLI：1 位提案者和至少 2 位独立评审。'); return }
    const inherited = continueDiscussion ? snapshot?.materials ?? [] : []
    const problem = validateMaterials([...inherited, ...materials])
    if (problem) { setMaterialError(problem); return }
    const operation = ++operationVersion.current
    startOperation.current = operation
    actionLock.current = true
    setPending('start')
    setError(null)
    const goal = draft.trim()
    const submittedMaterials = [...materials]
    const requestedMode = mode
    const parent = continueDiscussion ? snapshot : null
    try {
      let next: RunSnapshot
      if (parent) {
        if (!branchRequest.current || branchRequest.current.runId !== parent.id || branchRequest.current.goal !== goal || JSON.stringify(branchRequest.current.materials) !== JSON.stringify(submittedMaterials)) {
          branchRequest.current = { runId: parent.id, goal, materials: submittedMaterials, idempotencyKey: crypto.randomUUID() }
        }
        next = await api.branchRun(branchRequest.current)
      } else {
        let selectedPreparation = preparation?.status === 'completed' && preparedSignature.current === inputSignature(goal, submittedMaterials, analysisProvider) ? preparation : null
        if (requestedMode === 'auto') {
          selectedPreparation = await ensurePreparation()
          if (operationVersion.current !== operation) return
          if (!selectedPreparation || selectedPreparation.status !== 'completed') return
          const classification = selectedPreparation.classification
          if (!classification || !concreteRecommendation(classification.recommendation) || classification.confidence < 0.6) return
          if (selectedPreparation.materialAnalysis?.gaps.some((gap) => gap.importance === 'required')) return
        }
        if (operationVersion.current !== operation) return
        if (selectedPreparation && Date.now() - Date.parse(selectedPreparation.createdAt) >= 15 * 60_000) {
          setError('这份检查已超过 15 分钟，请点击“检查话题与材料”重新分析后再开始。')
          return
        }
        const preparationId = selectedPreparation?.id
        if (!startRequest.current || startRequest.current.goal !== goal || startRequest.current.mode !== requestedMode || startRequest.current.preparationId !== preparationId || JSON.stringify(startRequest.current.materials) !== JSON.stringify(submittedMaterials)) {
          startRequest.current = { goal, mode: requestedMode, materials: submittedMaterials, preparationId, idempotencyKey: crypto.randomUUID() }
        }
        next = await api.startRun(startRequest.current)
      }
      if (operationVersion.current !== operation) { void refreshList().catch(() => undefined); return }
      activeRef.current = next.id
      setActiveId(next.id)
      setTab('proposal')
      shouldScroll.current = true
      applySnapshot(next)
      setContextExpanded(false)
      setComposerExpanded(false)
      requestAnimationFrame(() => composerToggle.current?.focus({ preventScroll: true }))
      setDraft('')
      setMaterials([])
      setPreparation(null)
      preparedSignature.current = null
      setPasteOpen(false)
      setMaterialError(null)
      setContinueDiscussion(false)
      setSelectedVersion(null)
      startRequest.current = null
      branchRequest.current = null
      void refreshList().catch(() => undefined)
    } catch (failure) {
      if (operationVersion.current === operation) {
        setComposerExpanded(true)
        setError(`发起讨论失败：${errorMessage(failure)}`)
      }
      void refreshList().catch(() => undefined)
    } finally {
      if (startOperation.current === operation) {
        startOperation.current = null
        actionLock.current = false
        setPending(null)
      }
    }
  }

  const controlRun = async (action: 'stop' | 'resume') => {
    if (!api || !snapshot || actionLock.current) return
    const id = snapshot.id
    actionLock.current = true
    setPending(action)
    setError(null)
    try {
      let next: RunSnapshot
      if (action === 'stop') next = await api.stopRun(id)
      else {
        const key = resumeKeys.current.get(id) ?? crypto.randomUUID()
        resumeKeys.current.set(id, key)
        next = await api.resumeRun({ runId: id, idempotencyKey: key })
        resumeKeys.current.delete(id)
        setContextExpanded(false)
        setComposerExpanded(false)
        requestAnimationFrame(() => composerToggle.current?.focus({ preventScroll: true }))
      }
      applySnapshot(next)
      void refreshList().catch(() => undefined)
    } catch (failure) {
      setError(`${action === 'stop' ? '停止' : '恢复'}讨论失败：${errorMessage(failure)}`)
      void refreshRun(id, true)
    } finally {
      actionLock.current = false
      setPending(null)
    }
  }

  const onComposerKey = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key === 'Enter' && !event.shiftKey && !event.nativeEvent.isComposing) {
      event.preventDefault()
      event.currentTarget.form?.requestSubmit()
    }
  }

  const openSettings = async () => {
    if (!api) return
    setSettingsOpen(true)
    setSettingsError(null)
    setSettingsNotice(null)
    setProvidersLoading(true)
    // Preserve in-flight check results and editing snapshots when reopening.
    try { if (!providerCheckLock.current && !providerBatch) setProviders(await api.getProviderSettings()) }
    catch (failure) { setSettingsError(`读取 CLI 配置失败：${errorMessage(failure)}`) }
    finally { setProvidersLoading(false) }
  }

  const performProviderCheck = async (provider: ProviderInfo, config: ProviderConfig) => {
    if (!api) return false
    try {
      const result = await api.checkProvider(provider.id, config)
      setProviders((current) => current.map((entry) => entry.id === result.id ? result : entry))
      return result.requestOK === true
    } catch (failure) {
      const message = `${provider.name} 连接检查失败：${errorMessage(failure)}`
      setSettingsError(message)
      setProviders((current) => current.map((entry) => entry.id === provider.id ? { ...entry, status: 'error', requestOK: false, requestAttempted: undefined, error: message, lastCheckedAt: new Date().toISOString(), checkedConfiguration: 'editing', requestedModel: provider.sources?.model.kind === 'environment' ? provider.effective?.model ?? '' : config.model, requestedReasoning: provider.sources?.reasoning.kind === 'environment' ? provider.effective?.reasoning ?? '' : config.reasoning, reportedModel: undefined, reportedReasoning: undefined, modelSource: 'unreported', latencyMs: undefined } : entry))
      return false
    }
  }

  const checkProvider = async (provider: ProviderInfo, config: ProviderConfig) => {
    if (!api || providerCheckLock.current || settingsBusy || runs.some(isActive)) return
    providerCheckLock.current = true
    setProviderBatch(null)
    setCheckingProvider(provider.id)
    setSettingsError(null)
    try { await performProviderCheck(provider, { ...config }) }
    finally { providerCheckLock.current = false; setCheckingProvider(null) }
  }

  const checkAllProviders = async (configurations: ProviderConfig[]) => {
    if (!api || providerCheckLock.current || settingsBusy || runs.some(isActive) || !configurations.length) return
    // Freeze the entire batch before dispatch; returned results never save it.
    const plan = configurations.map((config) => ({ provider: providers.find((provider) => provider.id === config.id)!, config: { ...config } }))
    if (plan.some((item) => !item.provider)) return
    providerCheckLock.current = true
    providerBatchStop.current = false
    setSettingsError(null)
    setSettingsNotice(null)
    const batch: ProviderCheckBatch = { status: 'running', configurations: plan.map((item) => item.config), completed: 0, passed: 0, failed: 0 }
    setProviderBatch({ ...batch })
    try {
      for (const { provider, config } of plan) {
        if (providerBatchStop.current) break
        setCheckingProvider(provider.id)
        const passed = await performProviderCheck(provider, config)
        batch.completed++
        if (passed) batch.passed++
        else batch.failed++
        setProviderBatch({ ...batch, status: providerBatchStop.current ? 'stopping' : 'running' })
      }
    } finally {
      batch.status = batch.completed < plan.length ? 'stopped' : 'completed'
      setProviderBatch({ ...batch })
      providerCheckLock.current = false
      setCheckingProvider(null)
    }
  }

  const stopProviderBatch = () => {
    providerBatchStop.current = true
    setProviderBatch((batch) => batchIsRunning(batch) ? { ...batch!, status: 'stopping' } : batch)
  }

  const saveProviderSettings = async (configs: ProviderConfig[]) => {
    if (!api || settingsBusy || providerCheckLock.current) return false
    setSettingsBusy(true)
    setSettingsError(null)
    setSettingsNotice(null)
    try {
      setProviders(await api.saveProviderSettings(configs))
      setProviderBatch(null)
      invalidatePreparation()
      setSettingsNotice('配置已保存到应用数据目录。新讨论与新分析使用当前有效配置；运行中及恢复中的讨论继续使用冻结配置。环境变量覆盖的字段仍以环境变量为准。')
      return true
    } catch (failure) { setSettingsError(`保存 CLI 配置失败：${errorMessage(failure)}`); return false }
    finally { setSettingsBusy(false) }
  }

  const resetProviderSettings = async () => {
    if (!api || settingsBusy || providerCheckLock.current) return false
    setSettingsBusy(true)
    setSettingsError(null)
    setSettingsNotice(null)
    try {
      setProviders(await api.resetProviderSettings())
      setProviderBatch(null)
      invalidatePreparation()
      setSettingsNotice('已恢复并保存四家应用默认配置。环境变量覆盖继续有效；运行中及恢复中的讨论不受影响。')
      return true
    } catch (failure) { setSettingsError(`恢复默认失败：${errorMessage(failure)}`); return false }
    finally { setSettingsBusy(false) }
  }

  const reloadProviderModels = async (id: string) => {
    if (!api) return
    const result = await api.reloadProviderModels(id)
    setProviders((current) => current.map((entry) => entry.id === id ? { ...entry, catalog: result } : entry))
  }

  const exportRun = async (format: 'markdown' | 'json') => {
    if (!api || !snapshot || exporting) return
    setExporting(format)
    try {
      const result = await api.exportArtifact(snapshot.id, format)
      if (result.saved) setNotice(`讨论记录已导出${result.path ? `：${result.path}` : '。'}`)
    } catch (failure) { setError(`导出失败：${errorMessage(failure)}`) }
    finally { setExporting(null) }
  }

  const reconnect = async () => {
    if (!api) return
    setConnection('loading')
    try {
      setHello(await api.reconnectHost())
      setConnection('ready')
      setError(null)
      await refreshList()
      if (activeRef.current) await refreshRun(activeRef.current)
    } catch (failure) {
      setConnection('error')
      setError(`重新连接失败：${errorMessage(failure)}`)
    }
  }

  const clearDiagnostics = async () => {
    if (!api || !snapshot || cleaningLogs || isActive(snapshot)) return
    setCleaningLogs(true)
    try {
      const result = await api.clearDiagnostics(snapshot.id)
      setSettingsNotice(`已清理 ${result.removedFiles} 个诊断文件；保留${result.retained.length ? ` ${result.retained.join('、')}` : '方案、票据与运行审计记录'}。`)
    } catch (failure) { setSettingsError(`清理诊断日志失败：${errorMessage(failure)}`) }
    finally { setCleaningLogs(false) }
  }

  useEffect(() => {
    const onKey = (event: globalThis.KeyboardEvent) => {
      if (event.key === 'Escape') {
        setSettingsOpen(false)
        if (layout.narrow && detailsOpen && !settingsOpen) closeArtifacts()
        setPasteOpen(false)
      } else if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k' && !settingsOpen) {
        event.preventDefault()
        layout.setCollapsed('left', false)
        requestAnimationFrame(() => historySearch.current?.focus({ preventScroll: true }))
      } else if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'n') {
        event.preventDefault()
        if (!settingsOpen && !actionLock.current) newDiscussion()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [settingsOpen, detailsOpen, layout.narrow])

  if (!api) return <div className="desktop-required"><WorkspaceMark large /><h1>在桌面应用中打开 TCView</h1><p>聊天界面通过 Electron 安全桥接连接 Go 后台。<br />请使用项目的桌面启动命令打开应用。</p><span className="mock-badge">真实 CLI · 本地协调</span></div>

  const mock = snapshot ? snapshot.providerMode === 'mock' || snapshot.decision?.mock === true : hello?.providerMode === 'mock'
  const anonymous = (snapshot?.mode ?? mode) === 'delphi'
  const rolePlan = snapshot?.rolePlan ?? (snapshot ? legacyRolePlan : { proposer: enabledIds[0] ?? '', reviewers: enabledIds.slice(1) })
  const proposalVersions = [...new Map([...(snapshot?.proposalHistory ?? []), ...(snapshot?.proposal ? [snapshot.proposal] : [])].map((proposal) => [proposal.version, proposal])).values()].sort((a, b) => a.version - b.version)
  const selectedProposal = proposalVersions.find((proposal) => proposal.version === selectedVersion) ?? snapshot?.proposal
  const previousProposal = selectedProposal ? [...proposalVersions].reverse().find((proposal) => proposal.version < selectedProposal.version) : undefined
  const expired = !!snapshot && ['stopped', 'interrupted', 'failed'].includes(snapshot.status) && new Date(snapshot.deadlineAt).getTime() < now
  const selectedMode = modes.find((item) => item.value === (snapshot?.mode ?? mode))!
  const active = isActive(snapshot)
  const runningElsewhere = runs.find((run) => isActive(run) && run.id !== snapshot?.id)
  const hasActiveRun = active || !!runningElsewhere
  const canResume = snapshot && !snapshot.deletedAt && ['stopped', 'interrupted', 'failed'].includes(snapshot.status)
  const openIssues = snapshot?.reviews.filter((issue) => issue.status === 'open').length ?? 0
  const groups = new Map<string, { phase: string; round?: number; attemptId: string; messages: ChatMessage[] }>()
  for (const message of snapshot?.messages ?? []) {
    const key = `${message.attemptId}:${message.phase}:${message.round ?? 0}`
    const group = groups.get(key) ?? { phase: message.phase, round: message.round, attemptId: message.attemptId, messages: [] }
    group.messages.push(message)
    groups.set(key, group)
  }
  const claimIds = [...new Set([...(snapshot?.claims.map((claim) => claim.id) ?? []), ...(snapshot?.decision?.votes.map((vote) => vote.claimId) ?? [])])]
  const claimLabel = (id: string) => {
    const index = claimIds.indexOf(id)
    return index < 0 ? '关联主张' : `${anonymous ? '候选' : '主张'} ${String(index + 1).padStart(2, '0')}`
  }
  const providerOrder = ['codex', 'claude', 'grok', 'agy', 'system']
  for (const group of groups.values()) group.messages.sort((a, b) => providerOrder.indexOf(providerId(a.agentId, rolePlan)) - providerOrder.indexOf(providerId(b.agentId, rolePlan)))

  const participantIds = snapshot ? snapshot.providers?.filter((provider) => provider.enabled !== false).map((provider) => provider.id) ?? ['codex', 'claude', 'grok', 'agy'] : enabledIds
  const history = historyView === 'deleted' ? deletedRuns : runs
  const filteredHistory = history.filter((run) => `${run.title} ${run.goal}`.toLocaleLowerCase().includes(historyQuery.trim().toLocaleLowerCase()))

  return <div className={`app-shell carbon-workspace${/Mac/i.test(navigator.platform) ? ' platform-mac' : ''}${layout.leftCollapsed ? ' sidebar-collapsed' : ''}${!artifactsVisible ? ' artifacts-collapsed' : ''}`} data-theme="carbon" style={layout.style}>
    <aside id="history-panel" className="sidebar" aria-label="讨论历史" hidden={layout.leftCollapsed}>
      <div className="traffic-space" />
      <div className="sidebar-heading"><div className="brand" title="til-consensus 桌面客户端"><WorkspaceMark /><div><strong>TCView</strong><span>共识工作台</span></div><span className="version-tag">{hello?.appVersion ?? '—'}</span></div><button type="button" className="icon-button sidebar-collapse" data-testid="collapse-history-panel" aria-label="收起最近讨论" onClick={closeHistoryPanel}><Icon name="sidebar" size={16} /></button></div>
      <button className="new-discussion" data-testid="new-chat-button" onClick={newDiscussion} disabled={pending === 'start' && !preparing}>新建讨论<Icon name="plus" size={17} /></button>
      <div className="history-heading"><button className={historyView === 'recent' ? 'selected' : ''} data-testid="recent-history-tab" onClick={() => setHistoryView('recent')}>最近讨论 <span>{runs.length}</span></button><button className={historyView === 'deleted' ? 'selected' : ''} data-testid="deleted-history-tab" onClick={() => { setHistoryView('deleted'); void refreshList().catch(() => undefined) }} title="已删除的讨论可恢复"><Icon name="trash" size={12} />最近删除{deletedRuns.length > 0 && <span>{deletedRuns.length}</span>}</button></div>
      <div className="history-search"><Icon name="search" size={13} /><input ref={historySearch} data-testid="history-search" aria-label="搜索讨论" placeholder="搜索讨论 · ⌘K / Ctrl K" value={historyQuery} onChange={(event) => setHistoryQuery(event.target.value)} />{historyQuery && <button className="icon-button" aria-label="清除讨论搜索" onClick={() => { setHistoryQuery(''); historySearch.current?.focus() }}><Icon name="close" size={12} /></button>}</div>
      <nav className="history-list" aria-label={historyView === 'deleted' ? '最近删除的讨论' : '选择讨论'}>
        {filteredHistory.length === 0 ? <p className="history-empty">{historyQuery ? '没有匹配的讨论。' : historyView === 'deleted' ? '最近删除为空。删除的讨论可以在这里恢复。' : <>暂无讨论记录。<br />新建讨论以开始评审。</>}</p> : filteredHistory.map((run) => <div key={run.id} data-testid={`history-row-${run.id}`} className={`history-row ${activeId === run.id && !run.deletedAt ? 'selected' : ''}`}>
          {run.deletedAt ? <div className="history-item"><span className="history-dot" /><span className="history-item-content"><strong title={run.title || run.goal}>{run.title || run.goal}</strong><span>删除于 {dateLabel(run.deletedAt)}</span></span></div> : <button className="history-item" onClick={() => selectRun(run.id)} aria-current={activeId === run.id ? 'page' : undefined}>
            <span className={`history-dot ${run.status}`} /><span className="history-item-content"><strong title={run.title || run.goal}>{run.title || run.goal}</strong><span>{modes.find((item) => item.value === run.mode)?.short}<i>·</i>{statusLabels[run.status]}{run.providerMode === 'mock' ? ' · Mock' : ''}<i>·</i>{dateLabel(run.updatedAt)}</span></span>
          </button>}
          <button className="icon-button history-row-action" data-testid={run.deletedAt ? `restore-run-${run.id}` : `delete-run-${run.id}`} disabled={!!historyBusy} aria-label={run.deletedAt ? `恢复讨论 ${run.title || run.goal}` : `删除讨论 ${run.title || run.goal}`} title={run.deletedAt ? '恢复到最近讨论' : isActive(run) ? '请先停止讨论再删除' : '删除，可在最近删除中恢复'} onClick={() => void (run.deletedAt ? restoreDiscussion(run.id) : deleteDiscussion(run))}><Icon name={run.deletedAt ? 'refresh' : 'trash'} size={13} /></button>
        </div>)}
      </nav>
      <div className="sidebar-footer"><div className={`connection-dot ${connection}`} /><div><strong data-testid="connection-status">{connection === 'ready' ? 'Go 后台已连接' : connection === 'loading' ? '正在连接后台' : '后台连接异常'}</strong><span>本机保存 · {hello?.providerMode === 'mock' ? 'Mock 测试模式' : '真实 CLI'}</span></div>{connection === 'error' && <button className="icon-button" onClick={() => void reconnect()} aria-label="重新连接"><Icon name="refresh" size={16} /></button>}</div>
    </aside>

    <div {...layout.separator('left')} hidden={layout.leftCollapsed} />
    <main className="main-panel" style={{ '--composer-dock-height': `${composerDockHeight}px` } as CSSProperties}>
      <header className="topbar"><div className="topbar-navigation"><button ref={historyPanelToggle} type="button" className="icon-button history-panel-toggle" data-testid="toggle-history-panel" aria-label={layout.leftCollapsed ? '展开最近讨论' : '收起最近讨论'} title={layout.leftCollapsed ? '展开最近讨论' : '收起最近讨论'} aria-expanded={!layout.leftCollapsed} aria-controls="history-panel" onClick={toggleHistoryPanel}><Icon name="sidebar" size={17} /></button>{layout.leftCollapsed && <button type="button" className="icon-button" data-testid="new-chat-compact" aria-label="新建讨论" title="新建讨论 · ⌘N / Ctrl+N" disabled={pending === 'start' && !preparing} onClick={newDiscussion}><Icon name="plus" size={17} /></button>}<div className="breadcrumb"><span>工作空间</span><Icon name="chevron" size={12} /><strong>{snapshot?.title || '新建讨论'}</strong></div></div><div className="topbar-actions"><span className={`mock-badge ${mock ? '' : 'cli-badge'}`}><span />{mock ? 'Mock 历史 / 测试' : '真实 CLI'}</span><button className="icon-button" data-testid="provider-settings-button" onClick={() => void openSettings()} aria-label="CLI 设置与连接检查" title="CLI 设置与连接检查"><Icon name="settings" /></button><button ref={artifactsPanelToggle} type="button" className={`icon-button inspector-toggle ${artifactsVisible ? 'active' : ''}`} data-testid="toggle-artifacts-panel" onClick={() => { if (artifactsVisible) closeArtifacts(); else openArtifacts() }} aria-label={layout.narrow ? artifactsVisible ? '收起方案面板' : '打开方案面板' : artifactsVisible ? '收起讨论产物' : '展开讨论产物'} title={artifactsVisible ? '收起讨论产物' : '展开讨论产物'} aria-expanded={artifactsVisible} aria-controls="artifacts-panel"><Icon name="panel" /></button></div></header>

      <section className="discussion-context" aria-label="讨论控制与信息">
      <div className="session-toolbar"><div className="mode-selector"><Icon name="layers" size={15} /><CarbonSelect data-testid="mode-select" aria-label="讨论模式" value={snapshot?.mode ?? mode} disabled={!!snapshot || (pending !== null && !preparing)} onValueChange={(value) => { if (analysisRequest.current) invalidatePreparation(); setMode(value as InputMode) }} options={modes} /></div><span className="toolbar-divider" /><span className="phase-label">{snapshot ? `${snapshot.requestedMode === 'auto' ? '自动选择 · ' : ''}${phaseName(snapshot.phase)}` : selectedMode.description}</span><div className={`run-state ${snapshot?.status ?? 'idle'}`} aria-live="polite" data-testid="run-status"><span className="state-dot" />{snapshot ? statusLabels[snapshot.status] : '准备就绪'}</div><div className="run-controls">{active && <button className="subtle-button" data-testid="stop-button" disabled={pending !== null} onClick={() => void controlRun('stop')}><Icon name="stop" size={13} />{pending === 'stop' ? '正在停止' : '停止讨论'}</button>}{canResume && <button className="subtle-button resume-button" data-testid="resume-button" disabled={pending !== null || !!runningElsewhere || connection !== 'ready' || expired} onClick={() => void controlRun('resume')}><Icon name="play" size={13} />{pending === 'resume' ? '正在恢复' : expired ? '检查点已过期' : '从检查点继续'}</button>}</div><button type="button" className="context-toggle" data-testid="toggle-discussion-context" aria-controls="discussion-context-details" aria-expanded={contextExpanded} onClick={() => setContextExpanded((value) => !value)}><Icon name="settings" size={13} />{contextExpanded ? '收起讨论信息' : '讨论信息'}<span className="panel-chevron"><Icon name="chevron" size={12} /></span></button></div>
        {!snapshot && !discussionReady && <div className="compact-context-warning" data-testid="participant-gate" role="status">至少启用 3 家 CLI 才能讨论。<button onClick={() => void openSettings()}>选择 CLI</button></div>}
        <div id="discussion-context-details" className="discussion-context-body" data-testid="discussion-context-body" hidden={!contextExpanded}>
      <section className="discussion-heading"><div><div className="eyebrow">MULTI-AGENT CONSENSUS</div><h1>{snapshot ? '讨论记录与决策依据' : '新建共识讨论'}</h1><p>{snapshot ? '围绕同一份方案，保留每一次评审、修订与分歧。' : `提出你的问题，由 1 位提案者和 ${Math.max(0, enabledIds.length - 1)} 位独立评审共同推敲。`}</p></div></section>

      <div className="participants" style={{ gridTemplateColumns: `repeat(${Math.max(1, participantIds.length)}, minmax(0, 1fr))` }} aria-label={anonymous ? '匿名讨论参与者' : '已启用的 CLI 参与者'}>{participantIds.map((id) => {
        const agent = agents.find((entry) => entry.id === id)!
        if (!agent) return null
        const proposer = agent.id === rolePlan.proposer
        const hasResponded = snapshot?.messages.some((message) => providerId(message.agentId, rolePlan) === agent.id)
        const actualModel = [...(snapshot?.messages ?? [])].reverse().find((message) => providerId(message.agentId, rolePlan) === agent.id && message.model)?.model
        const frozenProvider = snapshot?.providers?.find((provider) => provider.id === agent.id)
        const displayedModel = actualModel || frozenProvider?.model || providers.find((provider) => provider.id === agent.id)?.model
        const working = active && ((proposer && /propos|synth|revis|p0|prepar/.test(snapshot?.phase ?? '')) || (!proposer && !/propos|synth|p0|prepar/.test(snapshot?.phase ?? '')))
        return <div key={agent.id} data-testid={`participant-${agent.id}`} className={`participant ${working ? 'working' : ''}`}><div className={`agent-avatar ${agent.color}`}>{anonymous ? proposer ? 'N' : agentLabel(agent.id, true, rolePlan).slice(-1) : agent.initials}</div><div className="participant-info"><strong title={anonymous ? undefined : displayedModel}>{agentLabel(agent.id, anonymous, rolePlan)}<span>{mock ? 'Mock' : anonymous ? '匿名' : displayedModel || '默认模型（未报告）'}</span></strong><small>{proposer ? '起草 · 综合' : anonymous ? '匿名独立评审' : '独立评审'}{frozenProvider?.version ? ` · ${frozenProvider.version}` : ''}</small></div><span className={`participant-dot ${working ? 'working' : hasResponded ? 'done' : ''}`} title={working ? 'CLI 正在处理' : hasResponded ? '已返回响应' : '等待开始'} /></div>
      })}</div>
      <div className={`role-plan ${!snapshot && !discussionReady ? 'role-plan-blocked' : ''}`} data-testid="role-plan">{snapshot ? '本次冻结角色' : '新讨论角色'}：{rolePlan.proposer ? <><strong>{agentLabel(rolePlan.proposer, anonymous, rolePlan)}</strong> 起草与综合 · {rolePlan.reviewers.length} 位独立评审</> : '尚未启用 CLI'}{!snapshot && <button onClick={() => void openSettings()}>选择 CLI</button>}</div>

        </div>
      </section>

      {notice && <div className="notice-banner" role="status"><Icon name="check" size={15} /><span>{notice}</span>{notice.startsWith('讨论已移到最近删除') && lastDeletedId && deletedRuns.some((run) => run.id === lastDeletedId) && <button className="subtle-button" data-testid="undo-delete-run" disabled={!!historyBusy} onClick={() => void restoreDiscussion(lastDeletedId)}>撤销删除</button>}<button className="icon-button" aria-label="关闭通知" onClick={() => setNotice(null)}><Icon name="close" size={14} /></button></div>}
      {snapshot?.deletedAt && <div className="notice-banner" role="status"><span>这条讨论位于最近删除中；恢复后可以继续讨论。</span><button className="subtle-button" disabled={!!historyBusy} onClick={() => void restoreDiscussion(snapshot.id)}>恢复讨论</button></div>}
      {error && <div className="error-banner" role="alert"><Icon name="alert" size={16} /><span>{error}</span><button className="icon-button" aria-label="关闭错误提示" onClick={() => setError(null)}><Icon name="close" size={15} /></button></div>}

      <div className="chat-area" ref={chatScroll} onScroll={() => { const node = chatScroll.current; if (node) { shouldScroll.current = node.scrollHeight - node.scrollTop - node.clientHeight < 100; setAwayFromBottom(!shouldScroll.current) } }}>
        {loadingRun && !snapshot ? <div className="loading-state"><span className="spinner" />正在读取本地讨论记录…</div> : !snapshot ? <div className={`welcome ${preparation ? 'welcome-with-preparation' : ''}`}>
          <ConsensusGraphic />
          <span className="welcome-kicker">MULTI-AGENT WORKSPACE</span><h2>让每项决策，都有充分依据。</h2><p>明确目标，汇集独立评审。<br />在同一工作台追踪方案、分歧与最终结论。</p>
          <div className="example-grid">{examples.map((example) => <button key={example.title} className="example-card" onClick={() => { changeDraft(example.text); openComposer() }}><Icon name={example.icon} size={18} /><strong>{example.title}</strong><span>{example.text}</span><Icon name="chevron" size={14} /></button>)}</div>
          <div className="mock-explainer"><span className="mini-spark">✳</span>{hello?.providerMode === 'mock' ? '当前为显式 Mock 测试模式。' : '调用已启用的本机 CLI；模型请求由各自服务处理。'}</div>
          <p className="request-estimate">{mode === 'free_debate' ? '典型流程约 18 次 CLI 请求；' : ''}本次最多 30 次，关联链最多 90 次，期限 15 分钟。</p>
        </div> : <div className="message-stream" data-testid="message-list">
          <div className="brief-message"><div className="brief-meta"><span className="you-avatar">你</span><strong>讨论目标</strong><time>{dateLabel(snapshot.createdAt)}</time></div><p>{snapshot.goal}</p>{snapshot.parentRunId && <div className="run-lineage"><Icon name="layers" size={14} /><span>继承方案 {snapshot.sourceProposalVersion !== undefined ? `P${snapshot.sourceProposalVersion}` : '版本'}{snapshot.sourceProposalHash ? ` · ${snapshot.sourceProposalHash.slice(0, 10)}` : ''} · 新一轮独立投票</span><button onClick={() => selectRun(snapshot.parentRunId!)}>查看上次讨论</button></div>}</div>
          {snapshot.preparation && <PreparationCard record={snapshot.preparation} materialCatalog={snapshot.materials} mock={mock} />}
          {!!snapshot.materials?.length && (!snapshot.preparation || snapshot.materials.length !== snapshot.preparation.materials.length) && <details className="frozen-materials"><summary>本次讨论的原文材料 · {snapshot.materials.length} 份</summary>{snapshot.materials.map((material) => <article key={material.id}><strong>{material.title}</strong><small>{sizeLabel(material.byteLength)} · SHA-256 {material.sha256.slice(0, 16)}</small><pre>{material.content}</pre></article>)}</details>}
          {anonymous && <><div className="anonymous-notice"><Icon name="layers" size={15} />德尔菲模式：专家原文不在聊天中展开，候选评分与分歧使用匿名聚合展示。</div><DelphiDistribution snapshot={snapshot} /></>}
          {[...groups.entries()].map(([key, group], groupIndex) => <section className="message-group" key={key}><div className="group-heading"><span className="group-number">{String(groupIndex + 1).padStart(2, '0')}</span><h2>{phaseName(group.phase)}{group.round ? ` · 第 ${group.round} 轮` : ''}</h2><div /><span>{group.attemptId !== snapshot.attemptId ? '历史执行 · ' : ''}{group.messages.length} 条记录</span></div>{group.messages.map((message) => {
            const provider = providerId(message.agentId, rolePlan)
            const agent = agents.find((item) => item.id === provider)
            const system = message.kind === 'system' || provider === 'system'
            const privateExpertResponse = anonymous && message.kind !== 'system' && provider !== rolePlan.proposer
            return <article id={`message-${message.id}`} key={message.id} className={`message-card ${system ? 'system-message' : ''} ${message.kind === 'decision' ? 'decision-message' : ''}`}><div className="message-avatar-wrap"><div className={`agent-avatar ${agent?.color ?? 'neutral'}`}>{system ? <Icon name="layers" size={15} /> : anonymous ? provider === rolePlan.proposer ? 'N' : agentLabel(message.agentId, true, rolePlan).slice(-1) : agent?.initials}</div></div><div className="message-main"><div className="message-meta"><strong>{agentLabel(message.agentId, anonymous, rolePlan)}</strong><span className="message-role">{privateExpertResponse ? '匿名问卷' : system ? '流程记录' : publicText(message.label || (message.kind === 'proposal' ? '方案提案' : message.kind === 'decision' ? '最终结论' : '评审意见'), anonymous, rolePlan)}</span>{mock && <span className="inline-mock">Mock</span>}<time>{new Date(message.at).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })}</time></div><FormattedText text={privateExpertResponse ? message.streaming ? '匿名专家正在填写问卷…' : '匿名问卷已提交，评价已纳入候选聚合结果。' : publicText(message.content, anonymous, rolePlan)} />{message.streaming && <div className="message-streaming" role="status"><span className="typing-dots"><i /><i /><i /></span>正在生成{!anonymous && message.model ? ` · ${message.model}` : ''}</div>}{!message.streaming && !anonymous && message.model && <div className="message-source">模型 {message.model}</div>}</div></article>
          })}</section>)}
          {active && <div className="activity-line" role="status"><span className="typing-dots"><i /><i /><i /></span><span>{phaseName(snapshot.phase)}<span className="muted"> · {mock ? '正在等待 Mock 响应' : 'CLI 正在生成与校验响应'}</span></span></div>}
          {active && providerProgress[snapshot.id] && <div className="provider-progress" aria-label="CLI 任务阶段">{Object.entries(providerProgress[snapshot.id]).map(([agent, progress]) => <span key={agent}><i className={/failed|cancel/.test(progress.status) ? 'failed' : ''} />{agentLabel(agent, anonymous, rolePlan)} · {/failed/.test(progress.status) ? '调用失败' : /cancel/.test(progress.status) ? '正在取消' : progress.kind === 'started' ? '已连接模型' : /tool/.test(progress.kind) ? '工具阶段' : progress.kind === 'terminal' ? '检查响应' : progress.kind === 'usage' ? '汇总用量' : '正在生成'}</span>)}</div>}
          {snapshot.error && <div className="run-error"><Icon name="alert" size={16} /><div><strong>本次运行需要处理</strong><p>{snapshot.error}</p></div></div>}
          {snapshot.status === 'completed' && <div className="completion-card" data-outcome={snapshot.decision?.outcome ?? 'completed'}><span className="completion-icon"><Icon name={['supported', 'accepted'].includes(snapshot.decision?.outcome ?? '') ? 'check' : 'layers'} size={18} /></span><div><strong>{outcomeLabels[snapshot.decision?.outcome ?? ''] ?? '讨论已完成'}</strong><p>{publicText(snapshot.decision?.summary ?? '方案与评审结果已保存在本机，可在右侧查看。', anonymous, rolePlan)}</p><span>{mock ? 'Mock 结果 · 不代表真实模型评审或事实验证' : '结论保留有效投票、方案版本与未解决意见'}</span></div><button className="subtle-button" onClick={() => { setTab('votes'); openArtifacts('votes') }}>查看决策<Icon name="chevron" size={13} /></button></div>}
          {canResume && <div className="checkpoint-note"><Icon name="play" size={14} />{expired ? '本次运行的截止时间已过，可补充要求创建关联讨论。' : snapshot.checkpoint ? '完整检查点已保存，可以从冻结配置恢复本次讨论。' : '讨论已停止，可尝试从后台保存的状态继续。'}</div>}
          <div ref={bottom} />
        </div>}
        {preparation && <div className="draft-preparation"><PreparationCard record={preparation} mock={hello?.providerMode === 'mock'} onRewrite={preparation.status === 'completed' && !hasActiveRun ? (value) => { changeDraft(value); setNotice('已采用改写。原检查已失效，后续分析将使用新表述。'); openComposer() } : undefined} /></div>}
      </div>

      {awayFromBottom && snapshot && <button className="jump-to-latest" data-testid="jump-to-latest" onClick={() => { shouldScroll.current = true; setAwayFromBottom(false); bottom.current?.scrollIntoView({ behavior: 'smooth', block: 'end' }) }}>回到最新记录 ↓</button>}
      <div ref={composerDock} className="composer-dock" data-testid="composer-dock">
        <div className="composer-dock-header">
          <div className="composer-dock-summary" data-testid="composer-dock-summary"><Icon name="chat" size={14} /><span>{preparing ? '正在检查话题与材料' : draft.trim() || materials.length || pasteContent.trim() ? `草稿未发送${materials.length ? ` · ${materials.length} 份材料` : ''}${pasteContent.trim() ? ' · 含待添加原文' : ''}` : hasActiveRun ? '讨论运行中 · 输入可按需展开' : snapshot ? '需要继续时展开输入' : '讨论输入'}</span></div>
          <div className="composer-dock-actions">
        {snapshot && !hasActiveRun && <div className="continuation-controls" aria-label="后续讨论方式"><button className={!continueDiscussion ? 'selected' : ''} onClick={() => { invalidatePreparation(); setContinueDiscussion(false); setMode('auto'); openComposer() }} aria-pressed={!continueDiscussion}>新问题</button><button data-testid="branch-button" className={continueDiscussion ? 'selected' : ''} disabled={!!snapshot.deletedAt || !snapshot.proposal || (pending !== null && !preparing)} onClick={() => { invalidatePreparation(); setContinueDiscussion(true); setMode(snapshot.mode); openComposer() }} aria-pressed={continueDiscussion}>补充要求，继续讨论</button></div>}
            {preparing && <button type="button" className="subtle-button" data-testid="cancel-preparation-button" disabled={cancellingPreparation} onClick={() => void cancelPreparation()}><Icon name="stop" size={12} />{cancellingPreparation ? '正在取消…' : '取消分析'}</button>}
            <button ref={composerToggle} type="button" className="composer-toggle" data-testid="toggle-composer" aria-controls="composer-dock-content" aria-expanded={composerExpanded} onClick={() => { if (composerExpanded) setComposerExpanded(false); else openComposer() }}>{composerExpanded ? '收起输入' : '展开输入'}<span className="panel-chevron"><Icon name="chevron" size={12} /></span></button>
          </div>
        </div>
        <div id="composer-dock-content" className="composer-dock-body" data-testid="composer-dock-body" hidden={!composerExpanded}>
      <div className="composer-wrap">
        {snapshot && <div className={`composer-participant-plan ${!discussionReady ? 'blocked' : ''}`} data-testid="composer-participant-plan">新讨论使用当前设置：{enabledIds[0] ? `${agentLabel(enabledIds[0], false)} 起草与综合 · ${Math.max(0, enabledIds.length - 1)} 位评审` : '全部停用'}{!discussionReady && <strong>至少启用 3 家 CLI 才能开始。</strong>}<button onClick={() => void openSettings()}>选择 CLI</button></div>}
        <div className="material-controls"><div><button type="button" className="subtle-button" data-testid="paste-material-button" disabled={hasActiveRun || (pending !== null && !preparing)} onClick={() => { if (analysisRequest.current) invalidatePreparation(); setPasteOpen(!pasteOpen) }}><Icon name="plus" size={13} />粘贴材料</button><button type="button" className="subtle-button" data-testid="import-materials-button" disabled={hasActiveRun || importingMaterials || (pending !== null && !preparing)} onClick={() => void importMaterials()}><Icon name="download" size={13} />{importingMaterials ? '正在读取' : '选择文本文件'}</button></div><span>{continueDiscussion && snapshot?.materials?.length ? `继承 ${snapshot.materials.length} 份 · ` : ''}{materials.length} 份 · {sizeLabel(materials.reduce((total, material) => total + materialBytes(material.content), 0))} / 128 KiB</span></div>
        {pasteOpen && <div className="material-paste" data-testid="material-paste-panel"><input data-testid="material-title-input" aria-label="材料标题" placeholder="材料标题（可选）" value={pasteTitle} onChange={(event) => { invalidatePreparation(); setPasteTitle(event.target.value) }} maxLength={140} /><textarea data-testid="material-content-input" aria-label="粘贴材料原文" placeholder="在此粘贴原文，不会自动读取文本中的文件路径。每份最多 64 KiB。" value={pasteContent} onChange={(event) => { invalidatePreparation(); setPasteContent(event.target.value); setMaterialError(null) }} rows={3} /><div><span>{sizeLabel(materialBytes(pasteContent))} / 64 KiB · 最多 8 份</span><button type="button" className="subtle-button" data-testid="add-pasted-material-button" disabled={!pasteContent.trim()} onClick={() => { if (addMaterials([{ id: crypto.randomUUID(), title: pasteTitle.trim() || `粘贴材料 ${materials.length + 1}`, content: pasteContent, source: 'pasted' }])) { setPasteOpen(false); setPasteTitle(''); setPasteContent('') } }}>添加材料</button></div></div>}
        {materials.length > 0 && <div className="material-list" data-testid="material-list">{materials.map((material) => {
          const normalized = preparation?.materials.find((entry) => entry.id === material.id)
          const sha = normalized?.sha256 || material.rawSha256
          return <div className="material-draft" key={material.id}><details><summary><strong>{material.title}</strong><span>{material.source === 'file' ? '文本文件' : '粘贴'} · {sizeLabel(materialBytes(material.content))}</span></summary><pre>{material.content}</pre><code title={sha}>{sha ? `SHA-256 ${sha}` : '检查后显示后端校验的内容指纹'}</code></details><button type="button" className="icon-button" data-testid={`remove-material-${material.id}`} aria-label={`移除材料 ${material.title}`} disabled={hasActiveRun || (pending !== null && !preparing)} onClick={() => { invalidatePreparation(); setMaterials(materials.filter((entry) => entry.id !== material.id)); setMaterialError(null) }}><Icon name="close" size={14} /></button></div>
        })}</div>}
        {materialError && <div className="material-error" role="alert" data-testid="material-error">{materialError}</div>}
        <form className={`composer ${hasActiveRun ? 'composer-busy' : ''}`} onSubmit={(event) => void start(event)}>
          <textarea data-testid="goal-input" ref={textarea} aria-label="讨论目标" placeholder={active ? '讨论进行中，完成或停止后可继续' : continueDiscussion ? '补充约束、材料或希望调整的取舍…' : snapshot ? '提出新问题，或选择继续讨论补充要求…' : '描述你想解决的问题、目标与约束…'} value={draft} onChange={(event) => changeDraft(event.target.value)} onKeyDown={onComposerKey} disabled={pending === 'start' && !preparing} rows={2} maxLength={6000} />
          <div className="composer-bottom"><span><span className="composer-mode-dot" />{modes.find((item) => item.value === (continueDiscussion ? snapshot?.mode : mode))?.label}<i>·</i>{mode === 'auto' && !continueDiscussion ? '先检查，再自动开始' : anonymous ? '匿名评审' : `1 位提案者 + ${Math.max(0, enabledIds.length - 1)} 位评审`}</span><button className="send-button" data-testid="send-button" type="submit" aria-label={continueDiscussion ? '提交补充要求继续讨论' : '发起讨论'} disabled={!draft.trim() || pending !== null || preparing || cancellingPreparation || importingMaterials || hasActiveRun || !!checkingProvider || connection !== 'ready' || !discussionReady}>{pending === 'start' ? <span className="spinner" /> : <Icon name="arrow" size={18} />}</button></div>
        </form>
        {!continueDiscussion && <div className="preparation-controls"><label>检查提供方<CarbonSelect aria-label="话题检查提供方" data-testid="preparation-provider-select" value={analysisProvider} disabled={hasActiveRun || !analysisOptions.length || (pending !== null && !preparing)} onValueChange={(value) => { invalidatePreparation(); setAnalysisProvider(value) }} options={analysisOptions.length ? analysisOptions.map((id) => ({ value: id, label: id === 'agy' ? 'agy · Gemini（默认）' : agentLabel(id, false) })) : [{ value: '', label: '请先启用 CLI' }]} /></label>{!preparing && <button type="button" className="subtle-button" data-testid="analyze-topic-button" disabled={!draft.trim() || pending !== null || hasActiveRun || !!checkingProvider || connection !== 'ready' || !analysisOptions.length} onClick={() => void ensurePreparation(true)}>检查话题与材料</button>}</div>}
        <div className="composer-caption">{checkingProvider ? <span>{agentLabel(checkingProvider, anonymous, rolePlan)} 正在执行真实连接检查…</span> : runningElsewhere ? <><span>另一场讨论正在运行。</span><button onClick={() => selectRun(runningElsewhere.id)}>查看讨论</button></> : active ? <span>运行中暂不接受新消息，可先停止讨论。</span> : <span>{mode === 'auto' && !continueDiscussion ? '发送后先检查话题；信息充分时直接开始讨论。' : 'Enter 发送，Shift + Enter 换行'}{snapshot && continueDiscussion ? ' · 创建关联讨论' : ''}</span>}<span>{mock ? 'Mock 测试记录' : '真实 CLI · 本地归档'}</span></div>
      </div>
        </div>
      </div>
    </main>

    <div {...layout.separator('right')} hidden={layout.narrow || layout.rightCollapsed} />
    {layout.narrow && detailsOpen && <button className="inspector-backdrop" onClick={closeArtifacts} aria-label="关闭方案面板" />}
    <aside id="artifacts-panel" className={`inspector ${layout.narrow && detailsOpen ? 'open' : ''}`} aria-label="讨论详情" hidden={!artifactsVisible}><div className="inspector-heading"><div><span className="eyebrow">DISCUSSION ARTIFACTS</span><h2>讨论产物</h2></div><span className="live-label"><span />实时同步</span><button type="button" className="icon-button inspector-close" data-testid="collapse-artifacts-panel" onClick={closeArtifacts} aria-label={layout.narrow ? '关闭方案面板' : '收起讨论产物'}><Icon name="close" size={16} /></button></div><div className="detail-tabs" role="tablist" aria-label="讨论产物分类" onKeyDown={(event) => {
        const tabs: DetailTab[] = ['proposal', 'issues', 'votes']
        const index = tabs.indexOf(tab)
        const next = event.key === 'ArrowRight' ? tabs[(index + 1) % tabs.length] : event.key === 'ArrowLeft' ? tabs[(index + tabs.length - 1) % tabs.length] : event.key === 'Home' ? tabs[0] : event.key === 'End' ? tabs[tabs.length - 1] : undefined
        if (next) { event.preventDefault(); setTab(next); document.getElementById(`tab-${next}`)?.focus() }
      }}>{([{ id: 'proposal', label: '方案' }, { id: 'issues', label: '争议' }, { id: 'votes', label: '投票' }] as const).map((item) => <button id={`tab-${item.id}`} key={item.id} role="tab" tabIndex={tab === item.id ? 0 : -1} aria-selected={tab === item.id} aria-controls={`panel-${item.id}`} className={tab === item.id ? 'selected' : ''} onClick={() => setTab(item.id)}>{item.label}{item.id === 'issues' && openIssues > 0 && <span>{openIssues}</span>}{item.id === 'votes' && !!snapshot?.decision?.votes.length && <span>{snapshot.decision.votes.length}</span>}</button>)}</div>
      <div className="detail-body" id={`panel-${tab}`} role="tabpanel" aria-labelledby={`tab-${tab}`} tabIndex={0}>
        {tab === 'proposal' && (snapshot && selectedProposal ? <>
          <div className="artifact-label"><label htmlFor="proposal-version">方案版本</label><CarbonSelect id="proposal-version" aria-label="方案版本" data-testid="proposal-version-select" className="proposal-version-select" value={String(selectedProposal.version)} onValueChange={(value) => setSelectedVersion(Number(value))} options={proposalVersions.map((proposal) => ({ value: String(proposal.version), label: `P${proposal.version}${proposal.hash === snapshot.proposal?.hash ? ' · 当前' : ''}` }))} /></div>
          <h3 className="proposal-title">{publicText(selectedProposal.title, anonymous, rolePlan)}</h3>
          <div className="proposal-document"><FormattedText text={publicText(selectedProposal.content, anonymous, rolePlan)} /></div>
          <div className="artifact-fingerprint"><Icon name="layers" size={13} /><span title={selectedProposal.hash}>内容指纹 {selectedProposal.hash.slice(0, 12)}</span></div>
          {previousProposal && <ProposalDiff previous={previousProposal} current={selectedProposal} anonymous={anonymous} roles={rolePlan} />}
          {selectedProposal.hash !== snapshot.proposal?.hash && <div className="mode-notice">正在查看历史方案。下方主张和投票属于本次运行的最终版本。</div>}
          {anonymous && <DelphiDistribution snapshot={snapshot} />}
          <ClaimCards snapshot={snapshot} anonymous={anonymous} />
          {snapshot.mode === 'adjudication' && <EvidencePanel snapshot={snapshot} anonymous={anonymous} />}
        </> : <ArtifactEmpty icon="layers" title="方案会在这里逐步成形" text="初稿、综合修订与关键主张，将随讨论同步到这里。" />)}
        {tab === 'issues' && (snapshot?.reviews.length ? <>
          <div className="issue-summary"><strong>{openIssues}</strong><span>项待解决</span><span>{snapshot.reviews.length - openIssues} 项已解决</span></div>
          {snapshot.reviews.map((issue) => <article className={`issue-card ${issue.status}`} key={issue.id}><div className="issue-meta"><span className={`severity ${issue.severity}`}>{issue.severity === 'blocking' ? '阻塞' : issue.severity === 'warning' ? '提醒' : '建议'}</span><span>{issue.status === 'resolved' ? '已解决' : '待回应'}</span></div><h3>{publicText(issue.statement, anonymous, rolePlan)}</h3><div className="issue-source"><span>{agentLabel(issue.raisedBy, anonymous, rolePlan)}{mock ? ' · Mock' : ''}</span>{issue.claimId && <span>{claimLabel(issue.claimId)}</span>}</div>{issue.resolutionRefs.length > 0 && <div className="resolution-refs"><Icon name="check" size={13} /><div><span>关闭依据</span><ReferenceLinks refs={issue.resolutionRefs} snapshot={snapshot} anonymous={anonymous} /></div></div>}</article>)}
          {snapshot.mode === 'adjudication' && <EvidencePanel snapshot={snapshot} anonymous={anonymous} />}
        </> : snapshot?.mode === 'adjudication' ? <EvidencePanel snapshot={snapshot} anonymous={anonymous} /> : <ArtifactEmpty icon="chat" title="让分歧留下记录" text="评审提出的问题会列在这里，保留严重程度与解决依据。" />)}
        {tab === 'votes' && (snapshot?.decision ? <DecisionPanel snapshot={snapshot} anonymous={anonymous} mock={mock} /> : <ArtifactEmpty icon="check" title="结论需要共同确认" text="讨论完成后，在这里查看每项主张的支持情况与最终判定。" />)}
      </div>
      {snapshot && <div className="artifact-actions"><button data-testid="export-markdown-button" className="subtle-button" disabled={!!exporting} onClick={() => void exportRun('markdown')}><Icon name="download" size={13} />{exporting === 'markdown' ? '正在导出' : 'Markdown'}</button><button data-testid="export-json-button" className="subtle-button" disabled={!!exporting} onClick={() => void exportRun('json')}><Icon name="download" size={13} />{exporting === 'json' ? '正在导出' : 'JSON'}</button></div>}
      {snapshot && <UsageView snapshot={snapshot} />}
      <div className="inspector-footer"><Icon name="layers" size={14} /><span title={hello ? `Go 协调流程 · til-consensus 引擎 ${hello.engineVersion}` : 'til-consensus · 等待 Go 后台'}>til-consensus · {hello ? hello.engineVersion.slice(0, 12) : '等待后台'}</span></div>
    </aside>
    {settingsOpen && <ProviderSettings providers={providers} loading={providersLoading} checking={checkingProvider} batch={providerBatch} busy={settingsBusy} error={settingsError} notice={settingsNotice} hasActiveRun={!!runs.find(isActive)} run={snapshot} cleaningLogs={cleaningLogs} onClose={() => setSettingsOpen(false)} onCheck={checkProvider} onCheckAll={checkAllProviders} onStopBatch={stopProviderBatch} onSave={saveProviderSettings} onReset={resetProviderSettings} onReload={reloadProviderModels} onClearLogs={() => void clearDiagnostics()} />}
  </div>
}

function ReferenceLinks({ refs, snapshot, anonymous }: { refs: string[]; snapshot: RunSnapshot; anonymous: boolean }) {
  return <div className="reference-links">{refs.map((reference, index) => {
    const message = snapshot.messages.find((item) => item.id === reference || item.invocationId === reference)
    const evidence = snapshot.evidence?.find((item) => item.id === reference)
    const label = anonymous ? `引用 ${index + 1}` : evidence ? `证据 ${snapshot.evidence!.indexOf(evidence) + 1}` : message ? `${phaseName(message.phase)} · ${agentLabel(message.agentId, false)}` : reference.length > 48 ? `${reference.slice(0, 45)}…` : reference
    return message ? <button key={`${reference}:${index}`} onClick={() => document.getElementById(`message-${message.id}`)?.scrollIntoView({ behavior: 'smooth', block: 'center' })}>{label}</button> : <span key={`${reference}:${index}`} title={anonymous ? undefined : reference}>{label}</span>
  })}</div>
}

const adjudicationLabels: Record<string, string> = { keep: '保留', keep_with_caveat: '有条件保留', unresolved: '未决', reject: '驳回', supported: '支持', accepted: '采纳' }

function ClaimCards({ snapshot, anonymous }: { snapshot: RunSnapshot; anonymous: boolean }) {
  if (!snapshot.claims.length) return null
  return <section className="claim-section"><div className="section-label">{snapshot.mode === 'adjudication' ? '主张与裁决' : '关键主张'}<span>{snapshot.claims.length}</span></div>{snapshot.claims.map((claim, index) => <div className="claim-card" key={claim.id}>
    <div><span className="claim-index">{String(index + 1).padStart(2, '0')}</span><span className="claim-category">{({ candidate: '候选', domain: '方案', process: '过程', synthesis: '综合' })[claim.category] ?? publicText(claim.category, anonymous, snapshot.rolePlan)}</span>{claim.adjudication ? <span className="claim-result">{adjudicationLabels[claim.adjudication] ?? publicText(claim.adjudication, anonymous, snapshot.rolePlan)}</span> : typeof claim.accepted === 'boolean' && <span className={`claim-result ${claim.accepted ? 'accepted' : ''}`}>{claim.accepted ? '采纳' : '未采纳'}</span>}</div>
    <p>{publicText(claim.statement, anonymous, snapshot.rolePlan)}</p>
    {claim.supportScore !== undefined && <div className="score-bar"><span style={{ width: `${Math.min(100, Math.max(0, claim.supportScore / (claim.category === 'candidate' ? 5 : 1) * 100))}%` }} /><small>{claim.category === 'candidate' ? `评分 ${claim.supportScore.toFixed(1)}/5` : `支持度 ${Math.round(claim.supportScore * 100)}%`}</small></div>}
    {!!claim.evidenceRefs?.length && <div className="claim-evidence"><span>关联依据</span><ReferenceLinks refs={claim.evidenceRefs} snapshot={snapshot} anonymous={anonymous} /></div>}
    {snapshot.mode === 'adjudication' && !claim.evidenceRefs?.length && <p className="evidence-missing">未提供可关联证据，不能视为事实已验证。</p>}
  </div>)}</section>
}

function EvidencePanel({ snapshot, anonymous }: { snapshot: RunSnapshot; anonymous: boolean }) {
  return <section className="evidence-panel"><div className="section-label">证据与验证<span>{snapshot.evidence?.length ?? 0} 项</span></div>{snapshot.evidence?.length ? snapshot.evidence.map((evidence, index) => <article className="evidence-card" key={evidence.id}>
    <div><span>证据 {String(index + 1).padStart(2, '0')}</span><strong>{({ verified: '已验证', unverified: '未验证', model_opinion: '模型判断', supported: '模型支持', rejected: '不成立', pending: '待验证' })[evidence.verificationStatus] ?? publicText(evidence.verificationStatus, anonymous, snapshot.rolePlan)}</strong></div>
    <p>{publicText(evidence.summary, anonymous, snapshot.rolePlan)}</p><div className="evidence-source">来源：{publicText(evidence.source, anonymous, snapshot.rolePlan)} · {publicText(evidence.kind, anonymous, snapshot.rolePlan)}</div>
    {evidence.artifactRef && !anonymous && <code className="evidence-reference">{evidence.artifactRef}</code>}
  </article>) : <div className="mode-notice">尚无独立证据记录。评审意见不自动成为事实验证结果。</div>}</section>
}

function VoteMatrix({ snapshot }: { snapshot: RunSnapshot }) {
  const reviewers = (snapshot.rolePlan ?? legacyRolePlan).reviewers.map((id) => agents.find((agent) => agent.id === id)).filter((agent): agent is typeof agents[number] => !!agent)
  return <div className="vote-matrix"><table><caption>主张投票矩阵</caption><thead><tr><th scope="col">主张</th>{reviewers.map((agent) => <th key={agent.id} scope="col">{agent.id === 'claude' ? 'Claude' : agent.name.replace(' Build', '')}</th>)}</tr></thead><tbody>{snapshot.claims.map((claim, index) => <tr key={claim.id}><th scope="row" title={claim.statement}>{String(index + 1).padStart(2, '0')}</th>{reviewers.map((agent) => {
    const vote = snapshot.decision?.votes.find((vote) => vote.claimId === claim.id && providerId(vote.agentId) === agent.id)
    const positive = vote && /support|agree|accept|keep|yes/.test(vote.vote)
    return <td key={agent.id} className={vote ? positive ? 'matrix-support' : 'matrix-other' : 'matrix-missing'}>{vote ? ({ support: '支持', agree: '支持', accept: '支持', keep: '保留', keep_with_caveat: '条件', reject: '反对', oppose: '反对', abstain: '弃权', unresolved: '未决', yes: '支持', no: '反对' })[vote.vote] ?? vote.vote : '缺席'}</td>
  })}</tr>)}</tbody></table></div>
}

function DecisionPanel({ snapshot, anonymous, mock }: { snapshot: RunSnapshot; anonymous: boolean; mock: boolean }) {
  const decision = snapshot.decision!
  const boundVersion = [...(snapshot.proposalHistory ?? []), ...(snapshot.proposal ? [snapshot.proposal] : [])].find((proposal) => proposal.hash === decision.proposalHash)
  const mismatch = decision.proposalHash && snapshot.proposal && decision.proposalHash !== snapshot.proposal.hash
  return <>
    <div className={`decision-summary ${decision.outcome === 'supported' ? 'supported' : ''}`} data-outcome={decision.outcome}><div className="artifact-label"><span>桌面判定</span>{mock && <span className="inline-mock">Mock</span>}</div><h3>{outcomeLabels[decision.outcome] ?? decision.outcome}</h3><p>{publicText(decision.summary, anonymous, snapshot.rolePlan)}</p><div>内核结果 <code>{decision.coreOutcome}</code></div></div>
    {snapshot.mode === 'free_debate' && <div className={`vote-binding ${mismatch ? 'binding-mismatch' : ''}`}><strong>{mismatch ? '投票版本与当前方案不一致' : boundVersion ? `投票绑定 P${boundVersion.version}` : '投票内容绑定'}</strong>{decision.proposalHash ? <><span>方案指纹 {decision.proposalHash.slice(0, 16)}</span>{decision.manifestHash && <span>清单指纹 {decision.manifestHash.slice(0, 16)}</span>}</> : <span>历史记录未提供完整的版本绑定信息。</span>}</div>}
    {!!snapshot.ballots?.length && <section className="ballot-validation"><div className="section-label">评审票有效性<span>{snapshot.ballots.filter((ballot) => ballot.valid).length} / {snapshot.ballots.length} 有效</span></div>{snapshot.ballots.map((ballot, index) => <div className={`validation-row ${ballot.valid ? 'valid' : 'invalid'}`} key={`${ballot.invocationId}:${index}`}><div><span>{agentLabel(ballot.agentId, anonymous, snapshot.rolePlan)}</span><strong>{ballot.valid ? '有效' : '无效'}</strong></div><p>{ballot.reason ? publicText(ballot.reason, anonymous, snapshot.rolePlan) : ballot.valid ? '内容与投票清单已绑定' : '未通过票据校验'}</p>{!anonymous && <small>方案 {ballot.proposalHash.slice(0, 12)} · 清单 {ballot.manifestHash.slice(0, 12)}</small>}</div>)}</section>}
    {anonymous && <DelphiDistribution snapshot={snapshot} />}
    {snapshot.mode === 'free_debate' && <VoteMatrix snapshot={snapshot} />}
    {snapshot.mode === 'adjudication' && <ClaimCards snapshot={snapshot} anonymous={anonymous} />}
    {snapshot.mode === 'free_debate' && <><div className="section-label votes-label">逐项投票<span>{decision.votes.length} 票</span></div>{snapshot.claims.map((claim, index) => {
      const votes = decision.votes.filter((vote) => vote.claimId === claim.id)
      return <div className="ballot-card" key={claim.id}><span className="ballot-claim-id">{anonymous ? '候选' : '主张'} {String(index + 1).padStart(2, '0')}</span><p>{publicText(claim.statement, anonymous, snapshot.rolePlan)}</p>{votes.length ? votes.map((vote, voteIndex) => <div className="ballot-row" key={`${vote.agentId}:${vote.claimId}:${voteIndex}`}><span>{agentLabel(vote.agentId, anonymous, snapshot.rolePlan)}{mock && <small>Mock</small>}</span><span className={`vote-value ${/support|agree|accept|keep|yes/.test(vote.vote) ? 'positive' : ''}`}>{({ support: '支持', agree: '支持', accept: '支持', keep: '保留', keep_with_caveat: '有条件保留', reject: '反对', oppose: '反对', abstain: '弃权', unresolved: '未决', yes: '支持', no: '反对' })[vote.vote] ?? publicText(vote.vote, anonymous, snapshot.rolePlan)}<b>{claim.category === 'candidate' || vote.support > 1 ? `${vote.support}/5` : `${Math.round(vote.support * 100)}%`}</b></span></div>) : <span className="muted small">暂无该主张的有效票</span>}</div>
    })}
    {decision.votes.filter((vote) => !snapshot.claims.some((claim) => claim.id === vote.claimId)).map((vote, index) => <div className="ballot-row unlinked-vote" key={`${vote.agentId}:${index}`}><span>{agentLabel(vote.agentId, anonymous, snapshot.rolePlan)}{mock ? ' · Mock' : ''}<small>关联主张</small></span><span>{publicText(vote.vote, anonymous, snapshot.rolePlan)} · {vote.support}</span></div>)}</>}
    {!!decision.minorityOpinions?.length && <section className="decision-notes"><div className="section-label">保留意见</div>{decision.minorityOpinions.map((opinion, index) => <p key={index}>{publicText(opinion, anonymous, snapshot.rolePlan)}</p>)}</section>}
    {!!decision.assumptions?.length && <section className="decision-notes"><div className="section-label">适用条件与假设</div>{decision.assumptions.map((assumption, index) => <p key={index}>{publicText(assumption, anonymous, snapshot.rolePlan)}</p>)}</section>}
    {snapshot.mode === 'adjudication' && <EvidencePanel snapshot={snapshot} anonymous={anonymous} />}
    <div className="decision-footnote"><Icon name="alert" size={14} />意见收敛与方案正确分别判断。{mock ? '当前为模拟投票，不作为事实验证。' : '模型评审不能代替实际证据验证。'}</div>
  </>
}

function UsageView({ snapshot }: { snapshot: RunSnapshot }) {
  const usage = snapshot.usage
  const format = (value?: number) => typeof value !== 'number' || !Number.isFinite(value) ? '未报告' : new Intl.NumberFormat('zh-CN').format(value)
  const cost = usage?.costUsd
  return <details className="usage-view"><summary>CLI 请求 {format(usage?.calls)}/30<span>{typeof cost !== 'number' || !Number.isFinite(cost) ? '费用未知' : `${usage?.estimated ? '约 ' : '已报告 '}$${cost.toFixed(4)}`}</span></summary><dl>{snapshot.chainLimit !== undefined && <div><dt>关联链请求</dt><dd>{format(snapshot.chainCalls)} / {format(snapshot.chainLimit)}</dd></div>}<div><dt>输入 Token</dt><dd>{format(usage?.inputTokens)}</dd></div><div><dt>输出 Token</dt><dd>{format(usage?.outputTokens)}</dd></div><div><dt>缓存 Token</dt><dd>{format(usage?.cachedTokens)}</dd></div><div><dt>数据来源</dt><dd>{usage?.source || 'CLI 尚未提供'}</dd></div></dl><p>请求计数包括重试和恢复，不等于服务端 API 次数。{usage?.estimated ? '费用为估算值。' : '仅展示 CLI 已报告的用量与费用。'}未报告部分不会按零计算。</p></details>
}

const cliDefaultLabel = (value?: string) => value || '使用 CLI 默认（不传覆盖）'
const editableProvider = (provider: ProviderInfo): ProviderConfig => {
  const configuration = provider.saved ?? provider.defaults
  return { id: provider.id, enabled: configuration?.enabled ?? true, model: configuration?.model ?? '', reasoning: configuration?.reasoning ?? '' }
}
const configurationKey = (configuration: ProviderConfig) => JSON.stringify([configuration.id, configuration.enabled ?? true, configuration.model, configuration.reasoning])
const providerSettingsKey = (providers: ProviderInfo[]) => JSON.stringify(providers.map((provider) => configurationKey(editableProvider(provider))))

type ProviderCheckBatch = {
  status: 'running' | 'stopping' | 'completed' | 'stopped'
  configurations: ProviderConfig[]
  completed: number
  passed: number
  failed: number
}
const batchIsRunning = (batch: ProviderCheckBatch | null) => batch?.status === 'running' || batch?.status === 'stopping'

function ProviderSettings({ providers, loading, checking, batch, busy, error, notice, hasActiveRun, run, cleaningLogs, onClose, onCheck, onCheckAll, onStopBatch, onSave, onReset, onReload, onClearLogs }: {
  providers: ProviderInfo[]; loading: boolean; checking: string | null; batch: ProviderCheckBatch | null; busy: boolean; error: string | null; notice: string | null; hasActiveRun: boolean; run: RunSnapshot | null; cleaningLogs: boolean
  onClose: () => void; onCheck: (provider: ProviderInfo, configuration: ProviderConfig) => Promise<void>
  onCheckAll: (configurations: ProviderConfig[]) => Promise<void>; onStopBatch: () => void
  onSave: (configurations: ProviderConfig[]) => Promise<boolean>; onReset: () => Promise<boolean>; onReload: (id: string) => Promise<void>; onClearLogs: () => void
}) {
  const dialog = useRef<HTMLDivElement>(null)
  const [drafts, setDrafts] = useState<Record<string, ProviderConfig>>({})
  const [manualModels, setManualModels] = useState<Record<string, boolean>>({})
  const [clearedReasoning, setClearedReasoning] = useState<Record<string, boolean>>({})
  const [catalogLoading, setCatalogLoading] = useState<Record<string, boolean>>({})
  const [localError, setLocalError] = useState<string | null>(null)
  const savedKey = useRef('')
  const catalogRequests = useRef(new Set<string>())
  const requestedCatalog = useRef(new Set<string>())
  const settingsKey = providerSettingsKey(providers)
  const batchRunning = batchIsRunning(batch)
  const checksLocked = busy || !!checking || batchRunning
  useEffect(() => {
    if (savedKey.current === settingsKey) return
    savedKey.current = settingsKey
    setDrafts(Object.fromEntries(providers.map((provider) => [provider.id, batchRunning ? { ...(batch!.configurations.find((config) => config.id === provider.id) ?? editableProvider(provider)) } : editableProvider(provider)])))
    setManualModels({})
    setClearedReasoning({})
  }, [providers, settingsKey])
  useEffect(() => {
    const previousFocus = document.activeElement as HTMLElement | null
    dialog.current?.querySelector<HTMLButtonElement>('button')?.focus()
    return () => previousFocus?.focus()
  }, [])
  const reload = async (id: string) => {
    if (catalogRequests.current.has(id)) return
    catalogRequests.current.add(id)
    setCatalogLoading((current) => ({ ...current, [id]: true }))
    setLocalError(null)
    try { await onReload(id) }
    catch (failure) { setLocalError(`${agentLabel(id, false)} 模型目录读取失败：${errorMessage(failure)}`) }
    finally { catalogRequests.current.delete(id); setCatalogLoading((current) => ({ ...current, [id]: false })) }
  }
  useEffect(() => {
    if (loading || checking || batchRunning) return
    for (const provider of providers) {
      if (providerEnabled(provider) && provider.available && (!provider.catalog || provider.catalog.source === 'cli-directory-stale') && !requestedCatalog.current.has(provider.id)) {
        requestedCatalog.current.add(provider.id)
        void reload(provider.id)
      }
    }
  }, [loading, providers, checking, batchRunning])
  const dirty = providers.some((provider) => configurationKey(drafts[provider.id] ?? editableProvider(provider)) !== configurationKey(editableProvider(provider)))
  const update = (id: string, change: Partial<ProviderConfig>) => {
    setLocalError(null)
    setDrafts((current) => ({ ...current, [id]: { ...(current[id] ?? editableProvider(providers.find((provider) => provider.id === id)!)), ...change } }))
  }
  const validate = () => {
    const emptyManual = providers.find((provider) => manualModels[provider.id] && !(drafts[provider.id]?.model ?? '').trim())
    if (emptyManual) { setLocalError(`${emptyManual.name} 的手工模型 ID 不能为空；如需默认模型，请选择“使用 CLI 默认”。`); return false }
    return true
  }
  const save = async () => {
    if (!validate()) return
    setLocalError(null)
    await onSave(providers.map((provider) => drafts[provider.id] ?? editableProvider(provider)))
  }
  return <div className="settings-overlay"><button className="settings-backdrop" onClick={onClose} aria-label="关闭 CLI 设置背景" /><div ref={dialog} className="settings-dialog" role="dialog" aria-modal="true" aria-labelledby="provider-settings-heading" onKeyDown={(event) => {
    if (event.key !== 'Tab') return
    const items = [...(dialog.current?.querySelectorAll<HTMLElement>('button:not(:disabled), input:not(:disabled), select:not(:disabled), [tabindex="0"]') ?? [])]
    const first = items[0]; const last = items.at(-1)
    if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last?.focus() }
    else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus() }
  }}><div className="settings-heading"><div><span className="eyebrow">LOCAL CLI CONNECTIONS</span><h2 id="provider-settings-heading">CLI 与连接检查</h2></div><button className="icon-button" aria-label="关闭设置" onClick={onClose}><Icon name="close" /></button></div>
    <div className="settings-toolbar"><div><strong data-testid="provider-settings-save-state">{dirty ? '有未保存修改' : '已保存配置'}</strong><span>{dirty ? batchRunning ? '本批次使用未保存的编辑快照；关闭后继续，重开保留进度和快照。' : '关闭设置会丢弃未保存修改；真实检查使用编辑中的配置。' : '保存后影响新讨论，恢复讨论沿用冻结配置。'}</span></div><div><button className="subtle-button check-all-providers" data-testid="check-all-providers" disabled={loading || checksLocked || hasActiveRun || !providers.length} aria-busy={batchRunning} onClick={() => { if (!validate()) return; setLocalError(null); void onCheckAll(providers.map((provider) => drafts[provider.id] ?? editableProvider(provider))) }}>{batchRunning ? <span className="spinner" /> : <Icon name="refresh" size={13} />}{batchRunning ? `正在检查 ${Math.min((batch?.completed ?? 0) + 1, batch?.configurations.length ?? 0)}/${batch?.configurations.length}` : '一键真实检查（使用额度）'}</button><button className="subtle-button" data-testid="reset-provider-settings" disabled={loading || checksLocked} onClick={() => { setLocalError(null); void onReset().then((ok) => { if (ok) { setDrafts(Object.fromEntries(providers.map((provider) => [provider.id, { id: provider.id, enabled: provider.defaults?.enabled ?? true, model: provider.defaults?.model ?? '', reasoning: provider.defaults?.reasoning ?? '' }]))); setManualModels({}) } }) }}>恢复并保存默认</button><button className="settings-save-button" data-testid="save-provider-settings" disabled={loading || checksLocked || !dirty} onClick={() => void save()}>{busy ? <span className="spinner" /> : <Icon name="check" size={14} />}{busy ? '正在保存' : '保存配置'}</button></div></div>
    <div className="settings-content"><p className="settings-intro">停用的 CLI 不参加新讨论，至少启用 3 家（1 位提案者与 2 位独立评审）；运行中及恢复中的参与者保持冻结。配置优先级：环境变量覆盖 ＞ 已保存应用设置 ＞ 应用默认。模型目录仅读取 CLI 能力；“真实检查”会调用一次模型并使用额度，检查编辑中的配置并叠加环境变量覆盖。设置只保存在应用数据目录。</p>
      <p className="provider-batch-scope">一键检查全部 {providers.length || 4} 家（含停用项），逐家使用点击时的编辑配置与环境覆盖，每家最多一次；不会自动保存、重试或替换模型。会使用服务额度。</p>
      {batch && <div className={`provider-batch-summary${batch.failed ? ' has-failures' : ''}`} data-testid="provider-batch-status" role="status" aria-live="polite"><div><strong>{batch.status === 'stopping' ? '正在停止后续检查，等待当前调用结束' : batchRunning ? `正在检查 ${Math.min(batch.completed + 1, batch.configurations.length)}/${batch.configurations.length} · ${providers.find((provider) => provider.id === checking)?.name ?? '准备检查'}` : batch.status === 'stopped' ? '已停止后续检查' : '全部检查完成'}</strong><span>已完成 {batch.completed}/{batch.configurations.length} · 通过 {batch.passed} · 失败 {batch.failed}{batch.status === 'stopped' ? ` · 跳过 ${batch.configurations.length - batch.completed}` : ''}</span><p>{batchRunning ? '本批次配置已固定。关闭页面后继续检查，重新打开可查看进度。' : '各家请求参数、实际报告与具体错误见下方卡片；结果不代表配置已保存。'}</p></div>{batchRunning && <button className="subtle-button" data-testid="stop-provider-batch" disabled={batch.status === 'stopping'} onClick={onStopBatch}><Icon name="stop" size={12} />{batch.status === 'stopping' ? '等待当前检查结束' : '停止后续检查'}</button>}</div>}
      {(error || localError) && <div className="settings-error" role="alert">{error || localError}</div>}
      {notice && <div className="settings-notice" role="status">{notice}</div>}
      {loading ? <div className="loading-state"><span className="spinner" />正在读取 CLI 状态…</div> : providers.length ? <div className="provider-settings-list">{providers.map((provider) => {
        const draft = drafts[provider.id] ?? editableProvider(provider)
        const catalog = provider.catalog
        const options = (catalog?.models ?? []).flatMap((option) => provider.id === 'claude' && option.resolvedModel && option.resolvedModel !== option.id ? [option, { ...option, id: option.resolvedModel, label: `${option.label || option.id} · 完整模型 ID` }] : [option]).filter((option, index, all) => all.findIndex((entry) => entry.id === option.id) === index)
        const selectedModel = options.find((option) => option.id === draft.model)
        const manual = !!manualModels[provider.id] || (!!draft.model && !selectedModel)
        const enabledSource = provider.sources?.enabled
        const modelSource = provider.sources?.model
        const reasoningSource = provider.sources?.reasoning
        const effective = provider.effective ?? { id: provider.id, model: provider.model ?? '', reasoning: provider.reasoning ?? '' }
        const editingEffective = { ...draft, model: modelSource?.kind === 'environment' ? effective.model ?? '' : draft.model, reasoning: reasoningSource?.kind === 'environment' ? effective.reasoning ?? '' : draft.reasoning }
        const checkedMatches = provider.lastCheckedAt && (provider.requestedModel ?? '') === editingEffective.model && (provider.requestedReasoning ?? '') === editingEffective.reasoning
        const requestPassed = provider.requestOK === true || provider.status === 'ready'
        const requestNotAttempted = 'requestAttempted' in provider && provider.requestAttempted === false
        const frozen = run?.providers?.find((entry) => entry.id === provider.id)
        const changed = configurationKey(draft) !== configurationKey(editableProvider(provider))
        const locked = checksLocked
        const effortValues = selectedModel?.reasoningMode === 'argument' ? selectedModel.reasoningValues : []
        const encodedVariants = selectedModel?.reasoningMode === 'encoded' && selectedModel.family ? options.filter((option) => option.family === selectedModel.family && option.reasoningMode === 'encoded' && option.effort) : []
        const sourceLabel = (source?: ProviderInfo['sources']['model']) => source?.kind === 'environment' ? `环境变量 ${source.env || ''}` : source?.kind === 'saved' ? '已保存设置' : '应用默认'
        return <article className={`provider-settings-card ${provider.status === 'error' ? 'provider-error' : ''}`} key={provider.id} data-testid={`provider-card-${provider.id}`}>
          <div className="provider-settings-name"><strong>{provider.name}</strong><label className="provider-enable-control"><input type="checkbox" data-testid={`provider-enabled-${provider.id}`} checked={draft.enabled !== false} disabled={checksLocked} onChange={(event) => update(provider.id, { enabled: event.target.checked })} />参与新讨论</label><span className={`provider-state ${checking === provider.id ? 'checking' : provider.status ?? (provider.available ? 'available' : 'unavailable')}`}>{checking === provider.id ? '正在真实检查' : requestPassed ? checkedMatches ? '当前编辑配置检查通过' : '历史配置检查通过' : provider.status === 'error' ? provider.lastCheckedAt && !requestNotAttempted ? '真实请求失败' : '配置或探测未通过' : provider.available ? '已安装 · 未真实检查' : '未找到 CLI'}</span></div>
          <p className="provider-enabled-summary" data-testid={`provider-enabled-effective-${provider.id}`}>已保存：{(provider.saved ?? provider.defaults)?.enabled !== false ? '启用' : '停用'} · 当前有效：{providerEnabled(provider) ? '启用' : '停用'} · {sourceLabel(enabledSource)}{enabledSource?.kind === 'environment' && '（保存不会更改环境覆盖）'}{!providerEnabled(provider) && '。不参加新讨论；仍可手动检查以修复连接。'}</p>
          <div className="provider-probe-stages" aria-label={`${provider.name} 检查阶段`}><span className={provider.executableFound ?? !!provider.binaryPath ? 'passed' : ''}><Icon name="check" size={11} />可执行文件 {provider.executableFound ?? !!provider.binaryPath ? '已找到' : '未找到'}</span><span className={provider.versionOK ?? !!provider.version ? 'passed' : ''}><Icon name="check" size={11} />版本 {provider.versionOK ?? !!provider.version ? '通过' : '未通过'}</span><span className={requestPassed ? 'passed' : ''}><Icon name="check" size={11} />模型请求 {checking === provider.id ? '检查中' : provider.lastCheckedAt ? requestNotAttempted ? '未发起' : requestPassed ? '通过' : '未通过' : '未检查'}</span></div>
          <div className="provider-editor">
            <label htmlFor={`provider-model-${provider.id}`}>模型 selector<CarbonSelect id={`provider-model-${provider.id}`} aria-label={`${provider.name} 模型 selector`} data-testid={`provider-model-${provider.id}`} value={manual ? '__manual__' : draft.model} disabled={locked} onValueChange={(value) => {
              if (value === '__manual__') { setManualModels((current) => ({ ...current, [provider.id]: true })); setClearedReasoning((current) => ({ ...current, [provider.id]: !!draft.reasoning })); update(provider.id, { model: '', reasoning: '' }); return }
              setManualModels((current) => ({ ...current, [provider.id]: false }))
              const next = options.find((option) => option.id === value)
              const reasoning = next?.reasoningMode === 'argument' && next.reasoningValues.includes(draft.reasoning) ? draft.reasoning : ''
              setClearedReasoning((current) => ({ ...current, [provider.id]: !!draft.reasoning && !reasoning }))
              update(provider.id, { model: value, reasoning })
            }} options={[{ value: '', label: '使用 CLI 默认（不传覆盖）' }, ...options.map((option) => ({ value: option.id, label: `${option.label && option.label !== option.id ? `${option.label} · ${option.id}` : option.id}${option.verified ? ' · CLI 目录返回' : ''}` })), { value: '__manual__', label: '手工填写模型 ID…' }]} /></label>
            {manual && <label className="manual-model-field">手工模型 ID<input data-testid={`provider-model-manual-${provider.id}`} aria-label={`${provider.name} 手工模型 ID`} value={draft.model} placeholder="输入 CLI 支持的模型 selector" maxLength={200} disabled={locked} onChange={(event) => { setManualModels((current) => ({ ...current, [provider.id]: true })); update(provider.id, { model: event.target.value, reasoning: '' }) }} /><span className="unverified-model" data-testid={`provider-model-verification-${provider.id}`}>{requestPassed && checkedMatches && editingEffective.model === draft.model ? '该 selector 的真实请求已通过，实际模型见下方报告。' : '手工 ID 尚未验证；保存不代表模型可用。'}</span></label>}
            {encodedVariants.length ? <label>模型内置推理档位<CarbonSelect data-testid={`provider-reasoning-${provider.id}`} aria-label={`${provider.name} 模型内置推理档位`} value={draft.model} disabled={locked} onValueChange={(value) => update(provider.id, { model: value, reasoning: '' })} options={encodedVariants.map((option) => ({ value: option.id, label: `${option.effort} · ${option.id}` }))} /><span>档位已编码在真实模型 ID 中，不再额外传入 effort。</span></label> : <label>推理配置<CarbonSelect data-testid={`provider-reasoning-${provider.id}`} aria-label={`${provider.name} 推理配置`} value={draft.reasoning} disabled={locked || !effortValues.length} onValueChange={(value) => update(provider.id, { reasoning: value })} options={[{ value: '', label: '使用 CLI 默认（不传覆盖）' }, ...effortValues.map((value) => ({ value, label: value })), ...(draft.reasoning && !effortValues.includes(draft.reasoning) ? [{ value: draft.reasoning, label: `${draft.reasoning} · 当前目录未确认` }] : [])]} /><span>{selectedModel?.reasoningMode === 'unsupported' ? '此模型不支持单独配置推理强度。' : selectedModel?.reasoningMode === 'encoded' ? '档位包含在模型 ID 中，不额外传 effort。' : effortValues.length ? '可选值来自该 CLI 与模型的能力描述。' : '当前模型的可选档位尚未确认，仅使用 CLI 默认。'}</span>{draft.reasoning && !effortValues.includes(draft.reasoning) && <button type="button" className="subtle-button clear-unconfirmed-reasoning" disabled={locked} onClick={() => update(provider.id, { reasoning: '' })}>清除未确认的推理覆盖</button>}</label>}
          </div>
          {clearedReasoning[provider.id] && <p className="provider-catalog-error" role="status">切换模型后已清除不适用的推理覆盖，当前使用 CLI 默认。检查与保存将使用上方显示的新配置。</p>}
          <div className="provider-catalog"><span>{catalogLoading[provider.id] ? '正在读取 CLI 模型目录…' : catalog ? `${catalog.models.length} 个目录条目 · ${catalog.source}${catalog.loadedAt ? ` · ${dateLabel(catalog.loadedAt)}` : ''}` : '模型目录尚未加载'}{selectedModel?.verified && !manual ? ' · 目录返回不代表真实请求已通过' : ''}</span><button className="subtle-button" data-testid={`reload-provider-models-${provider.id}`} disabled={!!catalogLoading[provider.id] || locked || !provider.available} onClick={() => void reload(provider.id)}><Icon name="refresh" size={12} />重新加载模型目录</button></div>
          {catalog?.error && <p className="provider-catalog-error" role="status">{catalog.error} 可填写手工模型 ID，保持未验证状态。</p>}
          <dl className="provider-config-comparison" data-testid={`provider-config-values-${provider.id}`}><div><dt>已保存配置</dt><dd>{provider.saved ? <><code>{cliDefaultLabel(provider.saved.model)}</code><span>推理：{cliDefaultLabel(provider.saved.reasoning)}</span></> : <span>尚无保存项，采用应用默认。</span>}</dd></div><div><dt>当前有效模型</dt><dd><code>{cliDefaultLabel(effective.model)}</code><span data-testid={`provider-model-source-${provider.id}`}>{sourceLabel(modelSource)}</span></dd></div><div><dt>当前有效推理</dt><dd><code>{cliDefaultLabel(effective.reasoning)}</code><span data-testid={`provider-reasoning-source-${provider.id}`}>{sourceLabel(reasoningSource)}</span></dd></div><div><dt>本次检查将用</dt><dd data-testid={`provider-editing-effective-${provider.id}`}><code>{cliDefaultLabel(editingEffective.model)}</code><span>推理：{cliDefaultLabel(editingEffective.reasoning)}{changed ? ' · 含未保存修改' : ''}</span></dd></div></dl>
          {(enabledSource?.kind === 'environment' || modelSource?.kind === 'environment' || reasoningSource?.kind === 'environment') && <p className="provider-environment-note" data-testid={`provider-env-notice-${provider.id}`}><Icon name="alert" size={13} />{[enabledSource?.kind === 'environment' ? `启用状态由 ${enabledSource.env || '环境变量'} 覆盖` : '', modelSource?.kind === 'environment' ? `模型由 ${modelSource.env || '环境变量'} 覆盖` : '', reasoningSource?.kind === 'environment' ? `推理由 ${reasoningSource.env || '环境变量'} 覆盖` : ''].filter(Boolean).join('；')}。保存应用设置不会替换当前覆盖值，真实检查也以覆盖值为准。</p>}
          {frozen && <details className="provider-runtime-details" data-testid={`provider-frozen-${provider.id}`}><summary>本次讨论的冻结配置 · 恢复时继续沿用</summary><dl><div><dt>请求模型</dt><dd>{cliDefaultLabel(frozen.model)}</dd></div><div><dt>请求推理</dt><dd>{cliDefaultLabel(frozen.reasoning)}</dd></div><div><dt>冻结 CLI 版本</dt><dd>{frozen.version || '旧记录未提供'}</dd></div></dl></details>}
          <details className="provider-runtime-details"><summary>CLI 版本、权限与程序路径</summary><dl><div><dt>CLI / 版本</dt><dd>{provider.cliType} · {provider.version || '未报告'}</dd></div><div><dt>权限范围</dt><dd>{provider.permissionScope || '未报告'}</dd></div>{provider.binaryPath && <div><dt>程序路径</dt><dd><code>{provider.binaryPath}</code></dd></div>}</dl></details>
          {provider.lastCheckedAt && <div className="provider-check-result" data-testid={`provider-check-result-${provider.id}`}><div><strong>{requestNotAttempted ? '未发起模型请求' : requestPassed ? '真实请求通过' : '真实请求失败'}</strong><span>{dateLabel(provider.lastCheckedAt)} · {typeof provider.latencyMs === 'number' ? `${(provider.latencyMs / 1000).toFixed(2)} 秒` : '耗时未报告'}</span></div><p>{requestNotAttempted ? '配置校验或版本探测未通过，本次没有派发模型请求。' : provider.checkedConfiguration === 'editing' ? '检查编辑中的配置，未保存项不会因检查而自动保存。' : '检查当时的有效配置。'}{!checkedMatches ? ' 以下是上次请求的结果，与当前编辑配置不同。' : ''}</p><dl><div><dt>请求模型</dt><dd data-testid={`provider-requested-model-${provider.id}`}>{cliDefaultLabel(provider.requestedModel)}</dd></div><div><dt>实际报告模型</dt><dd data-testid={`provider-reported-model-${provider.id}`}>{provider.reportedModel || '未报告'}<span>{provider.modelSource || '未报告'}</span></dd></div><div><dt>请求推理配置</dt><dd>{cliDefaultLabel(provider.requestedReasoning)}</dd></div><div><dt>实际报告推理</dt><dd data-testid={`provider-reported-reasoning-${provider.id}`}>{provider.reportedReasoning || '未报告'}</dd></div><div><dt>报告费用</dt><dd>{typeof provider.cost === 'number' ? `$${provider.cost.toFixed(4)}` : '未报告'}</dd></div></dl></div>}
          {provider.error && <p className="provider-error-message" role="alert">{provider.error}</p>}<div className="provider-check-footer"><span>{changed ? '检查使用编辑中的配置，不会自动保存。' : '每次点击只发起一个请求；会使用服务额度。'}</span><button className="subtle-button" disabled={!provider.available || !!checking || busy || hasActiveRun || (manual && !draft.model.trim())} data-testid={`check-provider-${provider.id}`} aria-busy={checking === provider.id} onClick={() => { setLocalError(null); void onCheck(provider, draft) }}>{checking === provider.id ? <span className="spinner" /> : <Icon name="refresh" size={13} />}{checking === provider.id ? '检查中' : '真实检查（使用额度）'}</button></div>
        </article>
      })}</div> : <div className="mode-notice">后台没有返回 CLI 状态。请重新连接后台后重试。</div>}
      {hasActiveRun && <p className="settings-footnote">讨论正在运行，可以为新讨论保存配置；结束或停止后可执行连接检查与日志清理。</p>}
      {run && <div className="diagnostics-settings"><div><h3>本次讨论诊断日志</h3><p>清理临时诊断文件，保留方案、投票及运行审计记录。</p></div><button className="subtle-button" disabled={hasActiveRun || cleaningLogs} onClick={onClearLogs}>{cleaningLogs ? '正在清理' : '清理诊断日志'}</button></div>}
    </div><div className="settings-footer">模型目录、请求 selector 与 CLI 实际报告分别记录；未报告的实际模型和推理强度保持“未报告”。保存不会改写全局 CLI 配置、登录状态或凭据。</div>
  </div></div>
}

function ArtifactEmpty({ icon, title, text }: { icon: IconName; title: string; text: string }) {
  return <div className="artifact-empty"><div><Icon name={icon} size={24} /></div><h3>{title}</h3><p>{text}</p><span>等待讨论产物</span></div>
}

export default App

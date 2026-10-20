export const PROTOCOL_VERSION = '1.4' as const
export type ProviderMode = 'cli' | 'mock'

export type WorkflowMode = 'free_debate' | 'adjudication' | 'delphi'
export type InputMode = WorkflowMode | 'auto'
export type MaterialSource = 'pasted' | 'file'
export interface MaterialInput { id: string; title: string; content: string; source: MaterialSource; rawSha256?: string }
export interface Material extends MaterialInput { rawSha256: string; sha256: string; byteLength: number; normalization: string }
export interface Classification {
  recommendation: WorkflowMode | 'needs_clarification' | 'not_suitable'
  confidence: number
  summary: string
  why: string[]
  missingInformation: string[]
  estimatedModeAfterClarification: WorkflowMode | ''
  estimatedModeReason: string
  suggestedTask: string
}
export interface MaterialAnalysis {
  summary: string
  materials: { materialId: string; summary: string; keyPoints: { quote: string; meaning: string }[] }[]
  gaps: { question: string; importance: 'required' | 'helpful'; materialIds: string[] }[]
  limitations: string[]
}
export interface PreparationInput { id: string; goal: string; materials: MaterialInput[]; providerId?: string }
export interface PreparationRecord {
  id: string
  inputHash: string
  requestHash: string
  goal: string
  materials: Material[]
  status: 'created' | 'running' | 'completed' | 'failed' | 'cancelled' | 'interrupted'
  createdAt: string
  updatedAt: string
  classification?: Classification
  materialAnalysis?: MaterialAnalysis
  source?: { providerId: string; cliType: string; cliVersion: string; command?: string; requestedModel: string; actualModel?: string; reasoning?: string; modelSource: string; elapsedMs: number; mock?: boolean }
  usage?: RunUsage
  costUsd?: number | null
  error?: string
  usedByRunId?: string
}
export type RunStatus = 'queued' | 'running' | 'completed' | 'stopped' | 'interrupted' | 'failed'

export interface HostHello {
  protocolVersion: string
  appVersion: string
  engineVersion: string
  capabilities: string[]
  providerMode: ProviderMode
}

export interface ProviderConfig {
  id: string
  enabled?: boolean
  command?: string
  model: string
  reasoning: string
}
export interface ProviderConfigSource { kind: 'default' | 'saved' | 'environment'; env?: string }
export interface ProviderConfigSources { enabled: ProviderConfigSource; command: ProviderConfigSource; model: ProviderConfigSource; reasoning: ProviderConfigSource }
export interface ProviderModelOption {
  id: string
  resolvedModel?: string
  label?: string
  family?: string
  effort?: string
  reasoningMode: 'argument' | 'encoded' | 'unsupported' | 'unknown'
  reasoningValues: string[]
  verified: boolean
}
export interface ProviderModelCatalog {
  id: string
  cliVersion?: string
  models: ProviderModelOption[]
  source: string
  loadedAt: string
  error?: string
  manualAllowed: boolean
}
export interface ProviderInfo {
  id: string
  enabled?: boolean
  name: string
  cliType: string
  binaryPath?: string
  version?: string
  available: boolean
  model?: string
  reasoning?: string
  permissionScope?: string
  status?: 'available' | 'unavailable' | 'checking' | 'ready' | 'error'
  error?: string
  lastCheckedAt?: string
  defaults?: ProviderConfig
  saved?: ProviderConfig
  effective?: ProviderConfig
  sources: ProviderConfigSources
  catalog?: ProviderModelCatalog
  executableFound?: boolean
  versionOK?: boolean
  requestOK?: boolean
  requestAttempted?: boolean
  checkedConfiguration?: 'editing' | 'effective'
  requestedModel?: string
  reportedModel?: string
  requestedReasoning?: string
  reportedReasoning?: string
  modelSource?: string
  reasoningSource?: string
  latencyMs?: number
  usage?: Record<string, unknown>
  cost?: number | null
}

export interface ProviderSnapshot {
  id: string
  enabled?: boolean
  command: string
  model?: string
  reasoning?: string
  version?: string
  permission?: string
  available: boolean
  sources?: Partial<ProviderConfigSources>
}

export interface RunUsage {
  calls?: number
  inputTokens?: number
  outputTokens?: number
  cachedTokens?: number
  costUsd?: number
  source?: string
  estimated?: boolean
}

export interface ChatMessage {
  id: string
  agentId: string
  role: string
  label: string
  content: string
  at: string
  phase: string
  attemptId: string
  kind: 'proposal' | 'review' | 'system' | 'decision'
  invocationId?: string
  streaming?: boolean
  round?: number
  model?: string
}

export interface Proposal {
  id: string
  title: string
  content: string
  hash: string
  version: number
  createdAt?: string
  invocationId?: string
  kind?: string
}

export interface ReviewIssue {
  id: string
  claimId?: string
  raisedBy: string
  severity: 'blocking' | 'warning' | 'info'
  status: 'open' | 'resolved'
  statement: string
  resolutionRefs: string[]
}

export interface ClaimView {
  id: string
  statement: string
  category: string
  supportScore?: number
  accepted?: boolean
  evidenceRefs?: string[]
  adjudication?: string
}

export interface Decision {
  outcome: string
  coreOutcome: string
  summary: string
  mock: boolean
  votes: { agentId: string; claimId: string; vote: string; support: number }[]
  proposalHash?: string
  manifestHash?: string
  minorityOpinions?: string[]
  assumptions?: string[]
}

export interface RunSummary {
  deletedAt?: string
  providerMode?: ProviderMode
  id: string
  title: string
  goal: string
  mode: WorkflowMode
  status: RunStatus
  phase: string
  createdAt: string
  updatedAt: string
}

export interface RolePlan { proposer: string; reviewers: string[] }

export interface RunSnapshot extends RunSummary {
  rolePlan?: RolePlan
  requestedMode?: InputMode
  materials?: Material[]
  preparation?: PreparationRecord
  delphiDistribution?: Record<string, number[]>
  providerMode?: ProviderMode
  providers?: ProviderSnapshot[]
  proposalHistory?: Proposal[]
  parentRunId?: string
  lineageRootId?: string
  sourceProposalVersion?: number
  sourceProposalHash?: string
  usage?: RunUsage
  chainCalls?: number
  chainLimit?: number
  ballots?: { agentId: string; invocationId: string; proposalHash: string; manifestHash: string; valid: boolean; reason?: string }[]
  evidence?: { id: string; kind: string; summary: string; source: string; artifactRef?: string; verificationStatus: string }[]
  hostPhase: string
  attemptId: string
  sessionId?: string
  checkpoint?: string
  deadlineAt: string
  proposal?: Proposal
  decision?: Decision
  error?: string
  messages: ChatMessage[]
  reviews: ReviewIssue[]
  claims: ClaimView[]
  lastEventId: number
}

export interface DesktopEvent {
  eventId: number
  runId: string
  attemptId: string
  type: string
  at: string
  payload: Record<string, unknown>
}

export interface StartRunInput {
  goal: string
  mode: InputMode
  idempotencyKey: string
  materials?: MaterialInput[]
  preparationId?: string
}

export interface ResumeRunInput {
  runId: string
  idempotencyKey: string
}

export interface BranchRunInput {
  runId: string
  goal: string
  idempotencyKey: string
  materials?: MaterialInput[]
}

export type ExportFormat = 'markdown' | 'json'
export interface ExportResult { saved: boolean; path?: string }

export interface DesktopAPI {
  hello(): Promise<HostHello>
  listRuns(): Promise<RunSummary[]>
  listDeletedRuns(): Promise<RunSummary[]>
  deleteRun(runId: string): Promise<RunSnapshot>
  restoreRun(runId: string): Promise<RunSnapshot>
  startRun(input: StartRunInput): Promise<RunSnapshot>
  getRun(runId: string): Promise<RunSnapshot>
  stopRun(runId: string): Promise<RunSnapshot>
  resumeRun(input: ResumeRunInput): Promise<RunSnapshot>
  getEvents(runId: string, afterEventId: number): Promise<DesktopEvent[]>
  listProviders(): Promise<ProviderInfo[]>
  checkProvider(providerId: string, config?: ProviderConfig): Promise<ProviderInfo>
  getProviderSettings(): Promise<ProviderInfo[]>
  saveProviderSettings(configs: ProviderConfig[]): Promise<ProviderInfo[]>
  resetProviderSettings(): Promise<ProviderInfo[]>
  reloadProviderModels(providerId: string): Promise<ProviderModelCatalog>
  branchRun(input: BranchRunInput): Promise<RunSnapshot>
  exportArtifact(runId: string, format: ExportFormat): Promise<ExportResult>
  reconnectHost(): Promise<HostHello>
  clearDiagnostics(runId: string): Promise<{ removedFiles: number; retained: string[] }>
  importMaterials(): Promise<MaterialInput[]>
  analyzeTopic(input: PreparationInput): Promise<PreparationRecord>
  getPreparation(id: string): Promise<PreparationRecord>
  cancelPreparation(id: string): Promise<PreparationRecord>
  subscribe(listener: (event: DesktopEvent) => void): () => void
}

declare global {
  interface Window {
    tcview: DesktopAPI
  }
}

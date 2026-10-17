import { apiClient } from '../client'

export type BeyondLimitsPlatform = 'openai' | 'deepseek' | 'gemini'

export interface BeyondLimitsConfig {
  platform?: string
  bridge: string
  tamper_rules: string[]
  rule_count: number
  default_bridge: string
  default_tamper_rules: string[]
}

export interface UpdateBeyondLimitsConfig {
  bridge?: string
  tamper_rules?: string[]
}

export interface BeyondLimitsToggleGroupResponse {
  id: number
  [key: string]: unknown
}

export interface BeyondLimitsStats {
  total: number
  injected: number
  tampered: number
  success_rate: number
  by_category: Record<string, number>
}

export interface BeyondLimitsInteraction {
  id: number
  time: string
  platform: string
  model: string
  category: string
  user_prompt: string
  reply_preview: string
  injected: boolean
  tampered: boolean
  duration_ms: number
}

export async function getConfig(platform: BeyondLimitsPlatform): Promise<BeyondLimitsConfig> {
  const { data } = await apiClient.get<BeyondLimitsConfig>(`/admin/beyond-limits/${platform}/config`)
  return data
}

export async function updateConfig(
  platform: BeyondLimitsPlatform,
  payload: UpdateBeyondLimitsConfig
): Promise<BeyondLimitsConfig> {
  const { data } = await apiClient.put<BeyondLimitsConfig>(
    `/admin/beyond-limits/${platform}/config`,
    payload
  )
  return data
}

export async function toggleGroup(
  platform: BeyondLimitsPlatform,
  id: number,
  enabled: boolean
): Promise<BeyondLimitsToggleGroupResponse> {
  const { data } = await apiClient.put<BeyondLimitsToggleGroupResponse>(
    `/admin/beyond-limits/${platform}/groups/${id}/toggle`,
    { enabled }
  )
  return data
}

export async function getStats(platform: BeyondLimitsPlatform): Promise<BeyondLimitsStats> {
  const { data } = await apiClient.get<BeyondLimitsStats>(`/admin/beyond-limits/${platform}/stats`)
  return data
}

export async function getLog(platform: BeyondLimitsPlatform): Promise<BeyondLimitsInteraction[]> {
  const { data } = await apiClient.get<BeyondLimitsInteraction[]>(
    `/admin/beyond-limits/${platform}/log`
  )
  return data
}

export const beyondLimitsAPI = {
  getConfig,
  updateConfig,
  toggleGroup,
  getStats,
  getLog
}

export default beyondLimitsAPI

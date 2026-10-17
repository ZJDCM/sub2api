<template>
  <AppLayout>
    <div class="space-y-6">
      <div v-if="loading" class="flex items-center justify-center py-16">
        <div class="h-8 w-8 animate-spin rounded-full border-b-2 border-primary-600"></div>
      </div>

      <template v-else>
        <!-- Page Header -->
        <div class="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
          <div class="flex items-center gap-3">
            <div class="flex h-10 w-10 items-center justify-center rounded-xl bg-gray-100 text-gray-900 dark:bg-dark-800 dark:text-white">
              <PlatformIcon :platform="platform" size="lg" />
            </div>
            <div>
              <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">
                {{ t(titleKey) }}
              </h1>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ t(descKey) }}
              </p>
            </div>
          </div>
          <div class="flex items-center gap-2">
            <button
              type="button"
              class="btn btn-secondary inline-flex items-center gap-2"
              @click="loadAll"
            >
              <Icon name="refresh" size="sm" />
              {{ t('common.refresh') }}
            </button>
            <button
              type="button"
              class="btn btn-primary inline-flex items-center gap-2"
              :disabled="saving"
              @click="saveConfig"
            >
              <Icon v-if="saving" name="refresh" size="sm" class="animate-spin" />
              {{ saving ? t('common.saving') : t('common.save') }}
            </button>
          </div>
        </div>

        <!-- Runtime stats（仅 openai 启用篡改，deepseek/gemini 只注入不显示） -->
        <div v-if="showRules" class="grid grid-cols-2 gap-3 md:grid-cols-4">
          <div class="rounded-lg border border-gray-100 bg-white px-4 py-3 shadow-sm dark:border-dark-700 dark:bg-dark-800">
            <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.beyondLimits.stats.total') }}</p>
            <p class="mt-1 text-xl font-semibold text-gray-900 dark:text-white">{{ stats.total }}</p>
          </div>
          <div class="rounded-lg border border-gray-100 bg-white px-4 py-3 shadow-sm dark:border-dark-700 dark:bg-dark-800">
            <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.beyondLimits.stats.injected') }}</p>
            <p class="mt-1 text-xl font-semibold text-gray-900 dark:text-white">{{ stats.injected }}</p>
          </div>
          <div class="rounded-lg border border-gray-100 bg-white px-4 py-3 shadow-sm dark:border-dark-700 dark:bg-dark-800">
            <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.beyondLimits.stats.tampered') }}</p>
            <p class="mt-1 text-xl font-semibold text-amber-600 dark:text-amber-400">{{ stats.tampered }}</p>
          </div>
          <div class="rounded-lg border border-gray-100 bg-white px-4 py-3 shadow-sm dark:border-dark-700 dark:bg-dark-800">
            <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.beyondLimits.stats.successRate') }}</p>
            <p class="mt-1 text-xl font-semibold text-gray-900 dark:text-white">{{ successRateLabel }}</p>
          </div>
        </div>

        <div v-if="showRules && categoryEntries.length > 0" class="rounded-lg border border-gray-100 bg-white px-4 py-3 shadow-sm dark:border-dark-700 dark:bg-dark-800">
          <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.beyondLimits.stats.byCategory') }}</p>
          <div class="mt-2 flex flex-wrap gap-2">
            <span
              v-for="[cat, count] in categoryEntries"
              :key="cat"
              class="inline-flex items-center gap-1.5 rounded-full bg-gray-100 px-2.5 py-1 text-xs font-medium text-gray-600 dark:bg-dark-700 dark:text-gray-300"
            >
              {{ categoryLabel(cat) }}
              <span class="font-semibold">{{ count }}</span>
            </span>
          </div>
        </div>

        <!-- Group toggle -->
        <div class="card">
          <div class="flex flex-col gap-4 border-b border-gray-100 px-6 py-4 dark:border-dark-700 lg:flex-row lg:items-center lg:justify-between">
            <div>
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.beyondLimits.groups.title') }}</h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.beyondLimits.groups.description') }}</p>
            </div>
          </div>
          <div class="p-6">
            <p v-if="groups.length === 0" class="text-sm text-gray-500 dark:text-gray-400">
              {{ t('admin.beyondLimits.groups.empty') }}
            </p>
            <div v-else class="grid grid-cols-1 gap-2 md:grid-cols-2 xl:grid-cols-3">
              <div
                v-for="g in groups"
                :key="g.id"
                class="flex items-center justify-between gap-3 rounded-lg border border-gray-200 bg-white px-4 py-3 dark:border-dark-700 dark:bg-dark-800"
              >
                <div class="min-w-0">
                  <p class="truncate text-sm font-medium text-gray-900 dark:text-white">{{ g.name }}</p>
                  <p class="mt-0.5 truncate text-xs text-gray-500 dark:text-gray-400">
                    {{ g.platform }} · {{ g.id }}
                  </p>
                </div>
                <Toggle
                  :model-value="!!g[enabledField]"
                  @update:model-value="(v: boolean) => toggleGroup(g, v)"
                />
              </div>
            </div>
          </div>
        </div>

        <!-- Bridge editor（deepseek/gemini 仅注入提示词，不显示拒绝检测规则） -->
        <div :class="showRules ? 'grid grid-cols-1 gap-6 xl:grid-cols-2' : 'grid grid-cols-1 gap-6'">
          <div class="card">
            <div class="flex flex-col gap-4 border-b border-gray-100 px-6 py-4 dark:border-dark-700 lg:flex-row lg:items-center lg:justify-between">
              <div>
                <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.beyondLimits.bridge.title') }}</h2>
                <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.beyondLimits.bridge.description') }}</p>
              </div>
              <button type="button" class="btn btn-secondary inline-flex items-center gap-2" @click="resetBridge">
                {{ t('admin.beyondLimits.bridge.resetDefault') }}
              </button>
            </div>
            <div class="p-6">
              <textarea
                v-model="configForm.bridge"
                rows="22"
                class="w-full rounded-lg border border-gray-200 bg-white px-3 py-2 font-mono text-xs text-gray-900 focus:border-primary-500 focus:outline-none focus:ring-1 focus:ring-primary-500 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-100"
                :placeholder="t('admin.beyondLimits.bridge.placeholder')"
              ></textarea>
            </div>
          </div>

          <div v-if="showRules" class="card">
            <div class="flex flex-col gap-4 border-b border-gray-100 px-6 py-4 dark:border-dark-700 lg:flex-row lg:items-center lg:justify-between">
              <div>
                <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.beyondLimits.rules.title') }}</h2>
                <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                  {{ t('admin.beyondLimits.rules.description') }} · {{ t('admin.beyondLimits.rules.count', { count: rulesCount }) }}
                </p>
              </div>
              <button type="button" class="btn btn-secondary inline-flex items-center gap-2" @click="resetRules">
                {{ t('admin.beyondLimits.rules.resetDefault') }}
              </button>
            </div>
            <div class="p-6">
              <textarea
                v-model="rulesText"
                rows="22"
                class="w-full rounded-lg border border-gray-200 bg-white px-3 py-2 font-mono text-xs text-gray-900 focus:border-primary-500 focus:outline-none focus:ring-1 focus:ring-primary-500 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-100"
                :placeholder="t('admin.beyondLimits.rules.placeholder')"
              ></textarea>
            </div>
          </div>
        </div>

        <!-- Interaction log（仅 openai 显示） -->
        <div v-if="showRules" class="card">
          <div class="flex items-center justify-between border-b border-gray-100 px-6 py-4 dark:border-dark-700">
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.beyondLimits.log.title') }}</h2>
          </div>
          <div class="p-6">
            <p v-if="log.length === 0" class="text-sm text-gray-500 dark:text-gray-400">
              {{ t('admin.beyondLimits.log.empty') }}
            </p>
            <div v-else class="overflow-x-auto">
              <table class="w-full text-left text-sm">
                <thead class="text-xs uppercase text-gray-500 dark:text-gray-400">
                  <tr>
                    <th class="px-2 py-2">{{ t('admin.beyondLimits.log.time') }}</th>
                    <th class="px-2 py-2">{{ t('admin.beyondLimits.log.model') }}</th>
                    <th class="px-2 py-2">{{ t('admin.beyondLimits.log.category') }}</th>
                    <th class="px-2 py-2">{{ t('admin.beyondLimits.log.injected') }}</th>
                    <th class="px-2 py-2">{{ t('admin.beyondLimits.log.tampered') }}</th>
                    <th class="px-2 py-2">{{ t('admin.beyondLimits.log.durationMs') }}</th>
                    <th class="px-2 py-2">{{ t('admin.beyondLimits.log.prompt') }}</th>
                    <th class="px-2 py-2">{{ t('admin.beyondLimits.log.reply') }}</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
                  <tr v-for="row in log" :key="row.id" class="align-top">
                    <td class="whitespace-nowrap px-2 py-2 text-xs text-gray-500 dark:text-gray-400">{{ formatDateTime(row.time) }}</td>
                    <td class="whitespace-nowrap px-2 py-2 text-xs text-gray-900 dark:text-white">{{ row.model }}</td>
                    <td class="whitespace-nowrap px-2 py-2 text-xs text-gray-600 dark:text-gray-300">{{ categoryLabel(row.category) }}</td>
                    <td class="px-2 py-2 text-xs">{{ row.injected ? t('common.yes') : t('common.no') }}</td>
                    <td class="px-2 py-2 text-xs">{{ row.tampered ? t('common.yes') : t('common.no') }}</td>
                    <td class="whitespace-nowrap px-2 py-2 text-xs text-gray-500 dark:text-gray-400">{{ row.duration_ms }}ms</td>
                    <td class="max-w-[220px] px-2 py-2 text-xs text-gray-600 dark:text-gray-300">
                      <span class="line-clamp-2">{{ row.user_prompt }}</span>
                    </td>
                    <td class="max-w-[260px] px-2 py-2 text-xs text-gray-600 dark:text-gray-300">
                      <span class="line-clamp-2">{{ row.reply_preview }}</span>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>
        </div>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import Toggle from '@/components/common/Toggle.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import { adminAPI } from '@/api/admin'
import type {
  BeyondLimitsInteraction,
  BeyondLimitsPlatform,
  BeyondLimitsStats
} from '@/api/admin/beyondLimits'
import type { AdminGroup } from '@/types'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatDateTime } from '@/utils/format'

interface Props {
  platform: BeyondLimitsPlatform
}

const props = defineProps<Props>()

type EnabledField = 'beyond_limits_enabled' | 'beyond_limits_deepseek_enabled' | 'beyond_limits_gemini_enabled'

const { t } = useI18n()
const appStore = useAppStore()

const titleKey = computed(() => `admin.beyondLimits.${props.platform}.title`)
const descKey = computed(() => `admin.beyondLimits.${props.platform}.description`)

const enabledField = computed<EnabledField>(() => {
  switch (props.platform) {
    case 'deepseek':
      return 'beyond_limits_deepseek_enabled'
    case 'gemini':
      return 'beyond_limits_gemini_enabled'
    default:
      return 'beyond_limits_enabled'
  }
})

// 仅 openai 平台启用拒绝篡改；deepseek/gemini 只注入提示词。
const showRules = computed(() => props.platform === 'openai')

const loading = ref(true)
const saving = ref(false)

const groups = ref<AdminGroup[]>([])
const stats = ref<BeyondLimitsStats>({ total: 0, injected: 0, tampered: 0, success_rate: 0, by_category: {} })
const log = ref<BeyondLimitsInteraction[]>([])

const configForm = ref<{ bridge: string; tamper_rules: string[] }>({ bridge: '', tamper_rules: [] })
const defaults = ref<{ bridge: string; rules: string[] }>({ bridge: '', rules: [] })
const rulesText = ref('')

const rulesCount = computed(() => rulesText.value.split('\n').map((s) => s.trim()).filter(Boolean).length)
const successRateLabel = computed(() => `${(stats.value.success_rate * 100).toFixed(1)}%`)
const categoryEntries = computed<[string, number][]>(() => Object.entries(stats.value.by_category || {}))

function categoryLabel(cat: string): string {
  switch (cat) {
    case 'crack':
      return t('admin.beyondLimits.category.crack')
    case 'reverse':
      return t('admin.beyondLimits.category.reverse')
    case 'pentest':
      return t('admin.beyondLimits.category.pentest')
    default:
      return t('admin.beyondLimits.category.general')
  }
}

function resetBridge() {
  configForm.value.bridge = defaults.value.bridge
}

function resetRules() {
  rulesText.value = defaults.value.rules.join('\n')
}

async function loadAll() {
  loading.value = true
  try {
    const [cfg, gs] = await Promise.all([
      adminAPI.beyondLimits.getConfig(props.platform),
      adminAPI.groups.getAll().catch(() => [] as AdminGroup[])
    ])
    const bridge = cfg.bridge || cfg.default_bridge
    const rules = cfg.tamper_rules && cfg.tamper_rules.length > 0 ? cfg.tamper_rules : cfg.default_tamper_rules
    configForm.value = { bridge, tamper_rules: rules }
    defaults.value = { bridge: cfg.default_bridge, rules: cfg.default_tamper_rules }
    groups.value = gs
    if (showRules.value) {
      rulesText.value = rules.join('\n')
      const [st, lg] = await Promise.all([
        adminAPI.beyondLimits.getStats(props.platform),
        adminAPI.beyondLimits.getLog(props.platform)
      ])
      stats.value = st
      log.value = lg
    }
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('admin.beyondLimits.loadFailed')))
  } finally {
    loading.value = false
  }
}

async function saveConfig() {
  saving.value = true
  try {
    const payload: { bridge: string; tamper_rules?: string[] } = { bridge: configForm.value.bridge }
    if (showRules.value) {
      payload.tamper_rules = rulesText.value.split('\n').map((s) => s.trim()).filter(Boolean)
    }
    await adminAPI.beyondLimits.updateConfig(props.platform, payload)
    appStore.showSuccess(t('admin.beyondLimits.saved'))
    await loadAll()
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('admin.beyondLimits.saveFailed')))
  } finally {
    saving.value = false
  }
}

async function toggleGroup(g: AdminGroup, enabled: boolean) {
  try {
    await adminAPI.beyondLimits.toggleGroup(props.platform, g.id, enabled)
    g[enabledField.value] = enabled
    appStore.showSuccess(t('admin.beyondLimits.groups.toggleSuccess'))
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('admin.beyondLimits.groups.toggleFailed')))
  }
}

onMounted(loadAll)
</script>

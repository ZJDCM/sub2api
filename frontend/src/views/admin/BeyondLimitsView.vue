<template>
  <AppLayout>
    <div class="space-y-6">
      <!-- Page Header -->
      <div class="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
        <div>
          <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">
            {{ t('admin.beyondLimits.title') }}
          </h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
            {{ t('admin.beyondLimits.description') }}
          </p>
        </div>
      </div>

      <!-- Content -->
      <div class="card p-8">
        <div class="flex flex-col items-center justify-center text-center">
          <div class="mb-5 flex h-16 w-16 items-center justify-center rounded-2xl bg-primary-50 text-primary-600 dark:bg-primary-900/20 dark:text-primary-400">
            <Icon name="bolt" size="lg" />
          </div>
          <h3 class="text-base font-semibold text-gray-900 dark:text-white">
            {{ t('admin.beyondLimits.placeholderTitle') }}
          </h3>
          <p class="mt-2 max-w-md text-sm text-gray-500 dark:text-gray-400">
            {{ t('admin.beyondLimits.placeholderDescription') }}
          </p>
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
</script>

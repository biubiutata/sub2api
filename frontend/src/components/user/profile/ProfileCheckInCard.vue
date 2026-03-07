<template>
  <div v-if="status?.enabled" class="card overflow-hidden">
    <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <div class="flex items-center justify-between gap-4">
        <div>
          <h3 class="text-lg font-semibold text-gray-900 dark:text-white">
            {{ t('profile.dailyCheckIn.title') }}
          </h3>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
            {{ t('profile.dailyCheckIn.description') }}
          </p>
        </div>
        <span
          :class="[
            'badge whitespace-nowrap',
            status.checked_in_today ? 'badge-success' : 'badge-primary'
          ]"
        >
          {{ status.checked_in_today ? t('profile.dailyCheckIn.checkedIn') : t('profile.dailyCheckIn.available') }}
        </span>
      </div>
    </div>

    <div class="space-y-4 px-6 py-5">
      <div v-if="status.checked_in_today" class="rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-700 dark:border-emerald-800/70 dark:bg-emerald-900/20 dark:text-emerald-300">
        <p>{{ t('profile.dailyCheckIn.checkedInToday') }}</p>
        <p v-if="status.last_checkin_at" class="mt-1 text-xs text-emerald-600 dark:text-emerald-400">
          {{ t('profile.dailyCheckIn.checkedInAt', { time: formatDateTime(status.last_checkin_at) }) }}
        </p>
      </div>

      <div v-else class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div class="text-sm text-gray-600 dark:text-gray-400">
          {{ t('profile.dailyCheckIn.rewardHint') }}
        </div>
        <button type="button" class="btn btn-primary" :disabled="submitting" @click="handleCheckIn">
          {{ submitting ? t('common.loading') : t('profile.dailyCheckIn.action') }}
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { userAPI, type DailyCheckInStatus } from '@/api/user'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { formatDateTime } from '@/utils/format'

const { t } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()

const status = ref<DailyCheckInStatus | null>(null)
const submitting = ref(false)

const loadStatus = async () => {
  try {
    status.value = await userAPI.getDailyCheckInStatus()
  } catch (error) {
    console.error('Failed to load daily check-in status:', error)
    status.value = null
  }
}

const handleCheckIn = async () => {
  if (!status.value || submitting.value) return

  submitting.value = true
  try {
    const result = await userAPI.dailyCheckIn()
    appStore.showSuccess(
      t('profile.dailyCheckIn.successWithAmount', { amount: `$${result.reward_amount.toFixed(2)}` })
    )
  } catch (error: any) {
    if (error?.status !== 409 && error?.code !== 'DAILY_CHECK_IN_ALREADY_COMPLETED') {
      appStore.showError(error?.message || t('profile.dailyCheckIn.failed'))
      submitting.value = false
      return
    }
  }

  try {
    await Promise.all([loadStatus(), authStore.refreshUser()])
  } finally {
    submitting.value = false
  }
}

onMounted(() => {
  loadStatus()
})
</script>

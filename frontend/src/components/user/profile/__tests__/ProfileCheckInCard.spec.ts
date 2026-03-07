import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ProfileCheckInCard from '../ProfileCheckInCard.vue'

const mocks = vi.hoisted(() => ({
  getDailyCheckInStatus: vi.fn(),
  dailyCheckIn: vi.fn(),
  showSuccess: vi.fn(),
  showError: vi.fn(),
  refreshUser: vi.fn()
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

vi.mock('@/api/user', () => ({
  userAPI: {
    getDailyCheckInStatus: mocks.getDailyCheckInStatus,
    dailyCheckIn: mocks.dailyCheckIn
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showSuccess: mocks.showSuccess,
    showError: mocks.showError
  })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    refreshUser: mocks.refreshUser
  })
}))

describe('ProfileCheckInCard', () => {
  beforeEach(() => {
    mocks.getDailyCheckInStatus.mockReset()
    mocks.dailyCheckIn.mockReset()
    mocks.showSuccess.mockReset()
    mocks.showError.mockReset()
    mocks.refreshUser.mockReset()
  })

  it('功能关闭时不渲染签到卡片', async () => {
    mocks.getDailyCheckInStatus.mockResolvedValue({
      enabled: false,
      checked_in_today: false
    })

    const wrapper = mount(ProfileCheckInCard)
    await flushPromises()

    expect(wrapper.find('.card').exists()).toBe(false)
  })

  it('可签到时显示签到按钮', async () => {
    mocks.getDailyCheckInStatus.mockResolvedValue({
      enabled: true,
      checked_in_today: false
    })

    const wrapper = mount(ProfileCheckInCard)
    await flushPromises()

    expect(wrapper.text()).toContain('profile.dailyCheckIn.action')
  })

  it('签到成功后隐藏按钮并刷新用户信息', async () => {
    mocks.getDailyCheckInStatus
      .mockResolvedValueOnce({
        enabled: true,
        checked_in_today: false
      })
      .mockResolvedValueOnce({
        enabled: true,
        checked_in_today: true,
        last_checkin_at: '2025-01-02T03:04:05Z'
      })
    mocks.dailyCheckIn.mockResolvedValue({
      message: 'ok',
      reward_amount: 1.5,
      new_balance: 14,
      checked_in_at: '2025-01-02T03:04:05Z'
    })
    mocks.refreshUser.mockResolvedValue(undefined)

    const wrapper = mount(ProfileCheckInCard)
    await flushPromises()

    await wrapper.find('button').trigger('click')
    await flushPromises()
    await flushPromises()

    expect(mocks.dailyCheckIn).toHaveBeenCalledTimes(1)
    expect(mocks.refreshUser).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).not.toContain('profile.dailyCheckIn.action')
    expect(wrapper.text()).toContain('profile.dailyCheckIn.checkedInToday')
  })
})

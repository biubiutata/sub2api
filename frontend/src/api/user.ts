/**
 * User API endpoints
 * Handles user profile management and password changes
 */

import { apiClient } from './client'
import type { User, ChangePasswordRequest } from '@/types'

export interface DailyCheckInStatus {
  enabled: boolean
  checked_in_today: boolean
  last_checkin_at?: string | null
}

export interface DailyCheckInResult {
  message: string
  reward_amount: number
  new_balance: number
  checked_in_at: string
}

/**
 * Get current user profile
 * @returns User profile data
 */
export async function getProfile(): Promise<User> {
  const { data } = await apiClient.get<User>('/user/profile')
  return data
}

/**
 * Update current user profile
 * @param profile - Profile data to update
 * @returns Updated user profile data
 */
export async function updateProfile(profile: {
  username?: string
}): Promise<User> {
  const { data } = await apiClient.put<User>('/user', profile)
  return data
}

/**
 * Change current user password
 * @param passwords - Old and new password
 * @returns Success message
 */
export async function changePassword(
  oldPassword: string,
  newPassword: string
): Promise<{ message: string }> {
  const payload: ChangePasswordRequest = {
    old_password: oldPassword,
    new_password: newPassword
  }

  const { data } = await apiClient.put<{ message: string }>('/user/password', payload)
  return data
}

export async function getDailyCheckInStatus(): Promise<DailyCheckInStatus> {
  const { data } = await apiClient.get<DailyCheckInStatus>('/user/check-in/status')
  return data
}

export async function dailyCheckIn(): Promise<DailyCheckInResult> {
  const { data } = await apiClient.post<DailyCheckInResult>('/user/check-in', {})
  return data
}

export const userAPI = {
  getProfile,
  updateProfile,
  changePassword,
  getDailyCheckInStatus,
  dailyCheckIn
}

export default userAPI

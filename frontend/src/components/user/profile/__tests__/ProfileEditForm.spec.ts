import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ProfileEditForm from '../ProfileEditForm.vue'

const translations: Record<string, string> = {
  'profile.editProfile': 'Profile Information',
  'profile.username': 'Username',
  'profile.usernameManagedByAdmin': 'Only administrators can change your username.'
}

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => translations[key] ?? key
  })
}))

describe('ProfileEditForm', () => {
  it('renders the username as read-only information with admin guidance', () => {
    const wrapper = mount(ProfileEditForm, {
      props: {
        initialUsername: 'alice'
      }
    })

    expect(wrapper.text()).toContain('Profile Information')
    expect(wrapper.text()).toContain('alice')
    expect(wrapper.text()).toContain('Only administrators can change your username.')
    expect(wrapper.find('form').exists()).toBe(false)
    expect(wrapper.find('input').exists()).toBe(false)
    expect(wrapper.find('button[type="submit"]').exists()).toBe(false)
  })

  it('falls back to a placeholder when username is empty', () => {
    const wrapper = mount(ProfileEditForm, {
      props: {
        initialUsername: ''
      }
    })

    expect(wrapper.text()).toContain('-')
  })
})

import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'

import MarkdownContent from '../MarkdownContent.vue'

describe('MarkdownContent', () => {
  it('renders common markdown elements as HTML', () => {
    const wrapper = mount(MarkdownContent, {
      props: {
        content: [
          '# 工作日报',
          '',
          '## 完成内容',
          '',
          '- 完成用户模块',
          '- 修复请求记录',
          '',
          '| 指标 | 值 |',
          '| --- | --- |',
          '| 请求 | 12 |'
        ].join('\n')
      }
    })

    expect(wrapper.find('h1').text()).toBe('工作日报')
    expect(wrapper.find('h2').text()).toBe('完成内容')
    expect(wrapper.findAll('li').map((item) => item.text())).toEqual(['完成用户模块', '修复请求记录'])
    expect(wrapper.find('table').exists()).toBe(true)
    expect(wrapper.find('th').text()).toBe('指标')
    expect(wrapper.find('td').text()).toBe('请求')
  })

  it('sanitizes unsafe html from markdown input', () => {
    const wrapper = mount(MarkdownContent, {
      props: {
        content: '<img src=x onerror="alert(1)">\n\n<script>alert(2)</script>'
      }
    })

    expect(wrapper.html()).not.toContain('onerror')
    expect(wrapper.html()).not.toContain('<script')
    expect(wrapper.find('img').exists()).toBe(true)
  })
})

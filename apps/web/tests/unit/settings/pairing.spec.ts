import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import PairingCodePanel from '@/components/settings/PairingCodePanel.vue'
import { mockApi, mountView, settle } from '../helpers'

beforeEach(() => {
  // Fake only the clock and intervals so promise flushing (setTimeout) keeps working.
  vi.useFakeTimers({ toFake: ['Date', 'setInterval', 'clearInterval'] })
  vi.setSystemTime(new Date('2026-10-17T10:00:00Z'))
})
afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

describe('PairingCodePanel', () => {
  it('counts down to the expiry and forgets the code when it expires', async () => {
    mockApi({ 'POST /extension/pairing-codes': () => ({ code: 'ABCD-EFGH', expires_at: new Date(Date.now() + 10 * 60_000).toISOString(), server_url: 'http://rea.example.lan' }) })
    const { wrapper } = await mountView(PairingCodePanel)
    await wrapper.findAll('button').find((b) => b.text().includes('Generate pairing code'))!.trigger('click')
    await settle()
    const values = wrapper.findAll('[data-testid="secret-value"]').map((i) => (i.element as HTMLInputElement).value)
    expect(values).toEqual(['ABCD-EFGH', 'http://rea.example.lan'])
    expect(wrapper.find('[data-testid="countdown"]').text()).toBe('10:00')

    vi.advanceTimersByTime(1000)
    await settle(1)
    expect(wrapper.find('[data-testid="countdown"]').text()).toBe('9:59')

    vi.advanceTimersByTime(60_000)
    await settle(1)
    expect(wrapper.find('[data-testid="countdown"]').text()).toBe('8:59')

    vi.advanceTimersByTime(9 * 60_000)
    await settle(1)
    expect(wrapper.find('[data-testid="countdown"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="expired"]').text()).toContain('expired')
    expect(document.body.innerHTML).not.toContain('ABCD-EFGH')
  })
})

import { beforeEach, describe, expect, it, vi } from 'vitest'
import { sessionApi } from '../api/client'
import { snapshot } from '../test/fixture'
import { useSessionStore } from './session'

describe('session store', () => {
  beforeEach(() => {
    useSessionStore.setState({ snapshot, connection: 'connected', pending: false, notice: null })
  })

  it('surfaces command rejection without replacing the snapshot', async () => {
    vi.spyOn(sessionApi, 'command').mockRejectedValueOnce(new Error('insufficient available cash'))
    const accepted = await useSessionStore.getState().send({ type: 'order.place', side: 'BUY', orderType: 'MARKET', quantity: 99_999 })
    expect(accepted).toBe(false)
    expect(useSessionStore.getState().snapshot).toBe(snapshot)
    expect(useSessionStore.getState().notice?.message).toMatch(/insufficient available cash/)
  })
})


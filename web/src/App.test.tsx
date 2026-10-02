import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import App from './App'
import { sessionApi } from './api/client'
import { useSessionStore } from './store/session'
import { snapshot } from './test/fixture'
import type { Command } from './types'

describe('MarketLab terminal', () => {
  const command = vi.spyOn(sessionApi, 'command')

  beforeEach(() => {
    command.mockReset()
    command.mockImplementation(async () => ({ accepted: true, snapshot }))
    useSessionStore.setState({
      snapshot,
      connection: 'connected',
      pending: false,
      notice: null,
      initialize: () => () => undefined,
    })
  })

  it('starts and single-steps the live simulation', async () => {
    const user = userEvent.setup()
    render(<App />)
    await user.click(screen.getByRole('button', { name: 'Start simulation' }))
    await user.click(screen.getByRole('button', { name: 'Advance one simulation step' }))
    expect(command.mock.calls.map(([value]) => (value as Command).type)).toEqual(['simulation.start', 'simulation.step'])
  })

  it('submits an order and triggers a scenario', async () => {
    const user = userEvent.setup()
    render(<App />)
    await user.clear(screen.getByLabelText('QUANTITY'))
    await user.type(screen.getByLabelText('QUANTITY'), '7')
    await user.click(screen.getByRole('button', { name: 'BUY 7 NOVA' }))
    await user.click(screen.getAllByRole('button', { name: /Negative news/ })[0])
    expect(command.mock.calls[0][0]).toMatchObject({ type: 'order.place', side: 'BUY', quantity: 7 })
    expect(command.mock.calls[1][0]).toEqual({ type: 'scenario.load', scenarioId: 'negative-news' })
  })

  it('enters replay and exposes linked causal events', async () => {
    const user = userEvent.setup()
    render(<App />)
    await user.click(screen.getByRole('button', { name: 'PAUSE & REPLAY RECORDING' }))
    expect(command).toHaveBeenCalledWith({ type: 'replay.enter' })
    const negativeNewsButtons = screen.getAllByRole('button', { name: /Negative news/ })
    await user.click(negativeNewsButtons[negativeNewsButtons.length - 1])
    expect(screen.getByText('#8')).toBeInTheDocument()
    expect(screen.getByText('#3')).toBeInTheDocument()
  })
})

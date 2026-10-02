import { expect, test } from '@playwright/test'

test('runs, trades, cancels, shocks, and replays', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByText('CONNECTED')).toBeVisible()
  await expect(page.locator('.mode')).toHaveText('LIVE')

  await page.getByRole('button', { name: 'Start simulation' }).click()
  await expect(page.getByRole('button', { name: 'Pause simulation' })).toBeVisible()
  await page.getByRole('button', { name: 'Pause simulation' }).click()

  await page.getByLabel('QUANTITY').fill('5')
  await page.getByLabel('LIMIT PRICE').fill('90')
  await page.getByRole('button', { name: 'BUY 5 NOVA' }).click()
  await expect(page.getByText(/#\d+/).first()).toBeVisible()
  await page.getByRole('button', { name: 'CANCEL' }).click()
  await expect(page.getByText('No open manual orders')).toBeVisible()

  await page.getByRole('button', { name: /Negative news/ }).first().click()
  await expect(page.getByText(/Negative news triggered/)).toBeVisible()

  await page.getByRole('button', { name: 'PAUSE & REPLAY RECORDING' }).click()
  await expect(page.locator('.mode')).toHaveText('REPLAY')
  await expect(page.getByRole('button', { name: /READ-ONLY IN REPLAY/ })).toBeDisabled()
  await page.getByRole('button', { name: 'Advance one recorded event' }).click()
})

test('reconnects to the authoritative session snapshot', async ({ page }) => {
  await page.goto('/')
  const before = await page.evaluate(async () => (await fetch('/api/session')).json() as Promise<{ sessionId: string; seq: number }>)
  await page.reload()
  await expect(page.getByText('CONNECTED')).toBeVisible()
  const after = await page.evaluate(async () => (await fetch('/api/session')).json() as Promise<{ sessionId: string; seq: number }>)
  expect(after.sessionId).toBe(before.sessionId)
  expect(after.seq).toBeGreaterThanOrEqual(before.seq)
})


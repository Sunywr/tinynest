import { expect, type APIRequestContext, type Page } from '@playwright/test'

async function answer(request: APIRequestContext, id: string) {
  const response = await request.get('/api/v1/test/captcha/' + encodeURIComponent(id))
  expect(response.ok()).toBeTruthy()
  return (await response.json()).answer as string
}

export async function captchaProof(request: APIRequestContext) {
  const challenge = await request.get('/api/v1/auth/captcha')
  expect(challenge.ok()).toBeTruthy()
  const { id } = await challenge.json()
  return { id, answer: await answer(request, id) }
}

export async function fillCaptcha(page: Page) {
  const img = page.getByRole('img', { name: '六位数字验证码' })
  await expect(img).toBeVisible()
  const id = await img.getAttribute('data-challenge-id')
  expect(id).toBeTruthy()
  await page.getByLabel('图形验证码').fill(await answer(page.request, id!))
}

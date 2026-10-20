import { expect, type Page } from '@playwright/test'

// 经过实际 Carbon 组件交互选择，测试不会绕过 UI 或调用原生 selectOption。
export async function selectDropdown(page: Page, testId: string, value: string) {
  const control = page.getByTestId(testId)
  await control.click()
  const menu = page.locator('.carbon-select-menu')
  await expect(menu).toBeVisible()
  for (const option of await menu.getByRole('option').all()) {
    if (await option.getAttribute('data-value') === value) {
      await option.click()
      await expect(control).toHaveAttribute('data-value', value)
      await expect(menu).not.toBeVisible()
      return
    }
  }
  throw new Error(`下拉框 ${testId} 中没有选项 ${JSON.stringify(value)}`)
}

import { expect, type Page } from '@playwright/test'

// CDP 的尺寸设置完成早于 React resize 更新。等待真实视口和已提交的
// 响应式按钮语义，避免隐藏的分隔线丢失按键或在过渡帧读取几何尺寸。
// 按钮标签同时支持展开和收起状态，不改变持久化的侧栏偏好。
export async function setViewportAndWait(page: Page, viewport = { width: 1440, height: 920 }) {
  await page.setViewportSize(viewport)
  await expect.poll(() => page.evaluate(() => ({ width: innerWidth, height: innerHeight }))).toEqual(viewport)
  await expect(page.getByTestId('toggle-artifacts-panel')).toHaveAttribute(
    'aria-label', viewport.width <= 1030 ? /^(打开|收起)方案面板$/ : /^(展开|收起)讨论产物$/,
  )
}

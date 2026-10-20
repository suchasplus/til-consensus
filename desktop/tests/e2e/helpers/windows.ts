import { expect, type ElectronApplication } from '@playwright/test'

// 等待产品完成 ready-to-show，随后保持原生窗口可见以持续提交合成帧。
// 仅测试实例穿透真实鼠标事件并禁止新的原生聚焦，真实 CDP 鼠标、键盘和截图
// 保持运行；交互过程中不再切换窗口可见性，避免 blur 改变菜单等焦点状态。
export async function prepareTestWindows(app: ElectronApplication, options: { preserveNotificationSettings?: boolean } = {}) {
  await expect.poll(() => app.evaluate(({ BrowserWindow }) => {
    const windows = BrowserWindow.getAllWindows()
    return windows.length > 0 && windows.every(window => window.isVisible())
  }), { message: '等待 Electron 完成初次显示' }).toBe(true)
  await app.evaluate(({ BrowserWindow }) => {
    for (const window of BrowserWindow.getAllWindows()) {
      window.setIgnoreMouseEvents(true)
      window.setFocusable(false)
      // macOS 的 setFocusable(false) 不移除已有焦点；交互前尽力 blur 一次。
      // CI 无其他 key window 时可能保留当前焦点，不能据此声称键盘隔离已确认。
      window.blur()
    }
  })
  await expect.poll(() => app.evaluate(({ BrowserWindow }) => {
    return BrowserWindow.getAllWindows().map(window => ({
      id: window.id, visible: window.isVisible(), focusable: window.isFocusable(), focused: window.isFocused(),
    }))
  }), { message: '确认测试窗口保持可见且已配置为不可聚焦；当前焦点仅作诊断' }).toMatchObject([{ visible: true, focusable: false }])
  if (!options.preserveNotificationSettings) {
    // 常规 Mock 回归不向用户桌面批量投递通知；专用通知测试显式保留设置并替换原生投递。
    const page = await app.firstWindow()
    await page.evaluate(() => window.tcview.saveNotificationSettings({ enabled: false, soundEnabled: false }))
  }
}

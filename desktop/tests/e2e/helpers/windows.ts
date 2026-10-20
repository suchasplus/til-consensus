import { expect, type ElectronApplication } from '@playwright/test'

// 等待产品完成 ready-to-show，随后保持原生窗口可见以持续提交合成帧。
// 仅测试实例穿透真实鼠标事件并禁止原生聚焦，真实 CDP 鼠标、键盘和截图
// 保持运行；交互过程中不再切换窗口可见性，避免 blur 改变菜单等焦点状态。
export async function prepareTestWindows(app: ElectronApplication) {
  await expect.poll(() => app.evaluate(({ BrowserWindow }) => {
    const windows = BrowserWindow.getAllWindows()
    return windows.length > 0 && windows.every(window => window.isVisible())
  }), { message: '等待 Electron 完成初次显示' }).toBe(true)
  await app.evaluate(({ BrowserWindow }) => {
    for (const window of BrowserWindow.getAllWindows()) {
      window.setIgnoreMouseEvents(true)
      window.setFocusable(false)
      // macOS 的 setFocusable(false) 不移除已有焦点；仅在测试交互前失焦一次。
      window.blur()
    }
  })
  await expect.poll(() => app.evaluate(({ BrowserWindow }) => {
    const windows = BrowserWindow.getAllWindows()
    return windows.length > 0 && windows.every(window => window.isVisible() && !window.isFocusable() && !window.isFocused())
  }), { message: '确认测试窗口保持可见且不接收原生焦点' }).toBe(true)
}

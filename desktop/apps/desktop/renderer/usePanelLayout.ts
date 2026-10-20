import { useEffect, useState } from 'react'
import type { CSSProperties, KeyboardEvent, PointerEvent } from 'react'

type Side = 'left' | 'right'
type Widths = Record<Side, number>
type Collapsed = Record<Side, boolean>
const storageKey = 'tcview.layout.v1'
const collapsedStorageKey = 'tcview.side-panels.v1'
const defaults: Widths = { left: 218, right: 310 }
const limits = { left: [174, 360], right: [260, 560] } as const
const clamp = (value: number, min: number, max: number) => Math.max(min, Math.min(max, value))

function readWidths(): Widths {
  try {
    const stored = JSON.parse(localStorage.getItem(storageKey) ?? 'null') as Partial<Widths> | null
    if (!stored) return defaults
    return Object.fromEntries((['left', 'right'] as const).map((side) => [side, typeof stored[side] === 'number' && Number.isFinite(stored[side]) ? clamp(stored[side], limits[side][0], limits[side][1]) : defaults[side]])) as Widths
  } catch { return defaults }
}

function readCollapsed(): Collapsed {
  try {
    const stored = JSON.parse(localStorage.getItem(collapsedStorageKey) ?? 'null') as { leftCollapsed?: unknown; rightCollapsed?: unknown } | null
    return { left: stored?.leftCollapsed === true, right: stored?.rightCollapsed === true }
  } catch { return { left: false, right: false } }
}

export function usePanelLayout() {
  const [preferred, setPreferred] = useState(readWidths)
  const [collapsed, setCollapsedState] = useState(readCollapsed)
  const [windowWidth, setWindowWidth] = useState(window.innerWidth)
  const [drag, setDrag] = useState<{ side: Side; pointerId: number; x: number; width: number } | null>(null)
  const narrow = windowWidth <= 1030
  // 窄窗只收缩显示宽度；保留用户首选值，窗口再次放大时自动恢复。
  const leftVisible = !collapsed.left
  const rightVisible = !narrow && !collapsed.right
  const dividerWidth = Number(leftVisible) + Number(rightVisible)
  const mainMin = narrow ? 400 : 420
  const leftMax = Math.min(limits.left[1], Math.max(limits.left[0], windowWidth - mainMin - (rightVisible ? limits.right[0] : 0) - dividerWidth))
  const left = leftVisible ? clamp(preferred.left, limits.left[0], leftMax) : 0
  const rightMax = Math.min(limits.right[1], Math.max(limits.right[0], windowWidth - left - mainMin - dividerWidth))
  const right = rightVisible ? clamp(preferred.right, limits.right[0], rightMax) : 0
  useEffect(() => {
    const resize = () => setWindowWidth(window.innerWidth)
    window.addEventListener('resize', resize)
    return () => window.removeEventListener('resize', resize)
  }, [])
  useEffect(() => {
    const flush = () => { try { localStorage.setItem(storageKey, JSON.stringify(preferred)) } catch { /* 受限存储不影响拖动。 */ } }
    // 数据只有两个数字，立即落盘可覆盖拖动结束后直接退出的操作。
    flush()
    window.addEventListener('pagehide', flush)
    return () => window.removeEventListener('pagehide', flush)
  }, [preferred])
  useEffect(() => {
    const flush = () => { try { localStorage.setItem(collapsedStorageKey, JSON.stringify({ leftCollapsed: collapsed.left, rightCollapsed: collapsed.right })) } catch { /* 受限存储不影响折叠。 */ } }
    flush()
    window.addEventListener('pagehide', flush)
    return () => window.removeEventListener('pagehide', flush)
  }, [collapsed])
  useEffect(() => {
    document.body.classList.toggle('resizing-panels', !!drag)
    return () => document.body.classList.remove('resizing-panels')
  }, [!!drag])
  const setCollapsed = (side: Side, value: boolean) => {
    setDrag(null)
    setCollapsedState((current) => current[side] === value ? current : { ...current, [side]: value })
  }
  const toggleCollapsed = (side: Side) => {
    setDrag(null)
    setCollapsedState((current) => ({ ...current, [side]: !current[side] }))
  }
  const visible = (side: Side) => side === 'left' ? leftVisible : rightVisible
  const update = (side: Side, value: number) => setPreferred((current) => ({ ...current, [side]: clamp(value, limits[side][0], side === 'left' ? leftMax : rightMax) }))
  const separator = (side: Side) => ({
    role: 'separator', tabIndex: visible(side) ? 0 : -1, 'aria-orientation': 'vertical' as const,
    'aria-label': side === 'left' ? '调整历史栏宽度' : '调整详情栏宽度',
    'aria-valuemin': limits[side][0], 'aria-valuemax': side === 'left' ? leftMax : rightMax,
    'aria-valuenow': Math.round(side === 'left' ? left : right),
    title: '拖动调整宽度；方向键微调，双击或 Home 恢复默认',
    'data-testid': `resize-${side}`,
    className: `panel-separator separator-${side}${drag?.side === side ? ' dragging' : ''}`,
    onDoubleClick: () => { if (visible(side)) setPreferred((current) => ({ ...current, [side]: defaults[side] })) },
    onPointerDown: (event: PointerEvent<HTMLDivElement>) => {
      if (event.button !== 0 || !visible(side)) return
      event.preventDefault()
      event.currentTarget.focus()
      event.currentTarget.setPointerCapture(event.pointerId)
      setDrag({ side, pointerId: event.pointerId, x: event.clientX, width: side === 'left' ? left : right })
    },
    onPointerMove: (event: PointerEvent<HTMLDivElement>) => {
      if (!visible(side) || drag?.side !== side || event.pointerId !== drag.pointerId) return
      update(side, drag.width + (event.clientX - drag.x) * (side === 'left' ? 1 : -1))
    },
    onPointerUp: (event: PointerEvent<HTMLDivElement>) => {
      if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId)
      setDrag(null)
    },
    onLostPointerCapture: () => setDrag(null),
    onKeyDown: (event: KeyboardEvent<HTMLDivElement>) => {
      if (!visible(side)) return
      if (event.key === 'Home') { event.preventDefault(); setPreferred((current) => ({ ...current, [side]: defaults[side] })); return }
      if (!['ArrowLeft', 'ArrowRight'].includes(event.key)) return
      event.preventDefault()
      update(side, (side === 'left' ? left : right) + (event.key === 'ArrowRight' ? 1 : -1) * (side === 'left' ? 1 : -1) * (event.shiftKey ? 32 : 8))
    },
  })
  // 栏间只占一条像素；透明鼠标热区由 CSS 叠放在边界两侧。
  const style: CSSProperties = { gridTemplateColumns: narrow ? `${left}px ${Number(leftVisible)}px minmax(0, 1fr)` : `${left}px ${Number(leftVisible)}px minmax(0, 1fr) ${Number(rightVisible)}px ${right}px` }
  return { style, separator, narrow, leftCollapsed: collapsed.left, rightCollapsed: collapsed.right, setCollapsed, toggleCollapsed }
}

import { useCallback, useEffect, useId, useLayoutEffect, useRef, useState } from 'react'
import type { CSSProperties, KeyboardEvent } from 'react'
import { createPortal } from 'react-dom'

export type CarbonSelectOption = { value: string; label: string; disabled?: boolean }

type CarbonSelectProps = {
  id?: string
  value: string
  options: CarbonSelectOption[]
  onValueChange: (value: string) => void
  disabled?: boolean
  className?: string
  'aria-label': string
  'data-testid'?: string
}

type MenuPosition = {
  left: number
  top?: number
  bottom?: number
  width: number
  maxHeight: number
  placement: 'top' | 'bottom'
}

const openEvent = 'tcview:carbon-select-open'

/** 保留原生选择器的键盘操作，以应用样式绘制按钮和展开菜单。 */
export function CarbonSelect(props: CarbonSelectProps) {
  const { value, options, onValueChange, disabled = false, className } = props
  const instance = useId()
  const triggerId = props.id ?? `carbon-select-${instance}`
  const menuId = `${triggerId}-listbox`
  const trigger = useRef<HTMLButtonElement>(null)
  const menu = useRef<HTMLDivElement>(null)
  const [open, setOpen] = useState(false)
  const [active, setActive] = useState(-1)
  const [position, setPosition] = useState<MenuPosition | null>(null)
  const search = useRef({ text: '', at: 0 })
  const enabled = options.flatMap((option, index) => option.disabled ? [] : [index])
  const unavailable = disabled || enabled.length === 0
  const expanded = open && !unavailable
  const selected = options.find((option) => option.value === value)
  const selectedIndex = options.findIndex((option) => option.value === value && !option.disabled)
  const optionId = (index: number) => `${menuId}-option-${index}`

  const close = useCallback((restoreFocus = false) => {
    setOpen(false)
    search.current = { text: '', at: 0 }
    // 外部点击和焦点移动只关闭菜单，避免把用户拉回旧控件。
    if (restoreFocus && trigger.current?.isConnected && !trigger.current.disabled) {
      trigger.current.focus({ preventScroll: true })
    }
  }, [])

  const openMenu = (initial = selectedIndex >= 0 ? selectedIndex : enabled[0]) => {
    if (unavailable) return
    window.dispatchEvent(new CustomEvent(openEvent, { detail: instance }))
    search.current = { text: '', at: 0 }
    setActive(initial)
    setPosition(null)
    setOpen(true)
  }

  const choose = (index: number) => {
    const option = options[index]
    if (!option || option.disabled || unavailable) return
    close(true)
    if (option.value !== value) onValueChange(option.value)
  }

  useLayoutEffect(() => {
    if (unavailable) close()
  }, [unavailable, close])

  useLayoutEffect(() => {
    if (!expanded) return
    if (active < 0 || !options[active] || options[active].disabled) {
      setActive(selectedIndex >= 0 ? selectedIndex : enabled[0] ?? -1)
    }
  }, [expanded, active, options, selectedIndex])

  useLayoutEffect(() => {
    if (!expanded) return
    const button = trigger.current
    if (!button) return
    let frame = 0

    const reposition = () => {
      frame = 0
      const rect = button.getBoundingClientRect()
      const viewportWidth = document.documentElement.clientWidth
      const viewportHeight = document.documentElement.clientHeight
      const visible = button.isConnected && !button.closest('[hidden], [inert], [aria-hidden="true"]')
        && rect.width > 0 && rect.height > 0 && getComputedStyle(button).visibility !== 'hidden'
        && rect.bottom > 0 && rect.top < viewportHeight && rect.right > 0 && rect.left < viewportWidth
      // 父面板折叠、控件移除或滚出视口时，不留下悬空的 portal。
      if (!visible) { close(); return }
      const gutter = 8
      const gap = 2
      const labelWidth = options.reduce((longest, option) => Math.max(longest,
        [...option.label].reduce((width, character) => width + (character.charCodeAt(0) > 127 ? 14 : 8), 48)), 0)
      const width = Math.max(0, Math.min(viewportWidth - gutter * 2, Math.max(rect.width, Math.min(560, Math.max(160, labelWidth)))))
      const desiredHeight = Math.min(320, options.length * 40 + 2)
      const below = Math.max(0, viewportHeight - rect.bottom - gutter - gap)
      const above = Math.max(0, rect.top - gutter - gap)
      const placement = below >= Math.min(desiredHeight, 160) || below >= above ? 'bottom' : 'top'
      const next: MenuPosition = {
        left: Math.max(gutter, Math.min(rect.left, viewportWidth - width - gutter)),
        width,
        maxHeight: Math.max(0, Math.min(320, placement === 'bottom' ? below : above)),
        placement,
        ...(placement === 'bottom'
          ? { top: Math.min(viewportHeight - gutter, Math.max(gutter, rect.bottom + gap)) }
          : { bottom: Math.min(viewportHeight - gutter, Math.max(gutter, viewportHeight - rect.top + gap)) }),
      }
      setPosition((previous) => previous && previous.left === next.left && previous.top === next.top
        && previous.bottom === next.bottom && previous.width === next.width && previous.maxHeight === next.maxHeight
        && previous.placement === next.placement ? previous : next)
    }
    const schedulePosition = () => { if (!frame) frame = requestAnimationFrame(reposition) }
    const onScroll = (event: Event) => {
      if (event.target instanceof Node && menu.current?.contains(event.target)) return
      schedulePosition()
    }
    reposition()
    window.addEventListener('resize', schedulePosition)
    document.addEventListener('scroll', onScroll, true)
    window.visualViewport?.addEventListener('resize', schedulePosition)
    window.visualViewport?.addEventListener('scroll', schedulePosition)
    const resize = new ResizeObserver(schedulePosition)
    resize.observe(button)
    resize.observe(document.documentElement)
    const visibility = new MutationObserver(schedulePosition)
    for (let ancestor: HTMLElement | null = button; ancestor; ancestor = ancestor.parentElement) {
      visibility.observe(ancestor, { attributes: true, attributeFilter: ['hidden', 'inert', 'aria-hidden', 'style', 'class'] })
    }
    // 兼容非 React 操作移除祖先节点，以及祖先布局内容变化。
    visibility.observe(document.documentElement, { childList: true, subtree: true })
    return () => {
      if (frame) cancelAnimationFrame(frame)
      window.removeEventListener('resize', schedulePosition)
      document.removeEventListener('scroll', onScroll, true)
      window.visualViewport?.removeEventListener('resize', schedulePosition)
      window.visualViewport?.removeEventListener('scroll', schedulePosition)
      resize.disconnect()
      visibility.disconnect()
    }
  }, [expanded, options, close])

  useEffect(() => {
    const onOtherOpen = (event: Event) => {
      if ((event as CustomEvent<string>).detail !== instance) close()
    }
    window.addEventListener(openEvent, onOtherOpen)
    return () => window.removeEventListener(openEvent, onOtherOpen)
  }, [instance, close])

  useEffect(() => {
    if (!expanded) return
    const outside = (event: Event) => {
      if (!(event.target instanceof Node)) return
      if (!trigger.current?.contains(event.target) && !menu.current?.contains(event.target)) close()
    }
    const onWindowBlur = () => close()
    document.addEventListener('pointerdown', outside, true)
    document.addEventListener('focusin', outside, true)
    window.addEventListener('blur', onWindowBlur)
    return () => {
      document.removeEventListener('pointerdown', outside, true)
      document.removeEventListener('focusin', outside, true)
      window.removeEventListener('blur', onWindowBlur)
    }
  }, [expanded, close])

  useLayoutEffect(() => {
    if (expanded && position && active >= 0) {
      document.getElementById(optionId(active))?.scrollIntoView({ block: 'nearest', inline: 'nearest' })
    }
  }, [expanded, active, position, menuId])

  const onKeyDown = (event: KeyboardEvent<HTMLButtonElement>) => {
    if (unavailable || event.nativeEvent.isComposing) return
    if (event.key === 'Escape' && expanded) {
      event.preventDefault()
      event.stopPropagation()
      close(true)
    } else if (event.key === 'Tab') {
      if (expanded) close()
      // 焦点留在按钮上，由浏览器与父对话框处理正常的 Tab 顺序。
    } else if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault()
      event.stopPropagation()
      if (!expanded) openMenu(selectedIndex >= 0 ? selectedIndex : event.key === 'ArrowUp' ? enabled.at(-1) : enabled[0])
      else {
        const current = enabled.indexOf(active)
        const next = (current + (event.key === 'ArrowDown' ? 1 : -1) + enabled.length) % enabled.length
        setActive(enabled[next])
      }
    } else if (event.key === 'Home' || event.key === 'End') {
      event.preventDefault()
      event.stopPropagation()
      const next = event.key === 'Home' ? enabled[0] : enabled.at(-1)!
      if (!expanded) openMenu(next)
      else setActive(next)
    } else if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault()
      event.stopPropagation()
      if (expanded) choose(active)
      else openMenu()
    } else if (event.key.length === 1 && !event.metaKey && !event.ctrlKey && !event.altKey) {
      event.preventDefault()
      event.stopPropagation()
      const now = Date.now()
      const text = `${now - search.current.at > 700 ? '' : search.current.text}${event.key.toLocaleLowerCase()}`
      const prefix = [...text].every((character) => character === text[0]) ? text[0] : text
      const origin = expanded ? active : selectedIndex
      const ordered = [...enabled.filter((index) => index > origin), ...enabled.filter((index) => index <= origin)]
      const next = ordered.find((index) => options[index].label.trim().toLocaleLowerCase().startsWith(prefix))
      if (!expanded) openMenu(next ?? (selectedIndex >= 0 ? selectedIndex : enabled[0]))
      else if (next !== undefined) setActive(next)
      search.current = { text, at: now }
    }
  }

  const menuStyle: CSSProperties | undefined = position ? {
    position: 'fixed', left: position.left, top: position.top, bottom: position.bottom,
    width: position.width, maxHeight: position.maxHeight,
  } : undefined

  return <span className={`carbon-select${className ? ` ${className}` : ''}`} data-open={expanded} data-disabled={unavailable}>
    <button ref={trigger} id={triggerId} data-testid={props['data-testid']} data-value={value}
      className="carbon-select-trigger" type="button" role="combobox" disabled={unavailable}
      aria-label={props['aria-label']} aria-haspopup="listbox" aria-expanded={expanded}
      aria-controls={expanded ? menuId : undefined} aria-activedescendant={expanded && position && active >= 0 ? optionId(active) : undefined}
      title={selected?.label ?? value} onKeyDown={onKeyDown} onClick={() => expanded ? close() : openMenu()}>
      <span className="carbon-select-value">{selected?.label ?? (value || '请选择')}</span>
      <svg className="carbon-select-chevron" width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true"><path d="m4 6 4 4 4-4" stroke="currentColor" strokeWidth="1.25" /></svg>
    </button>
    {expanded && position && createPortal(<div ref={menu} id={menuId} className="carbon-select-menu" role="listbox"
      aria-label={props['aria-label']} aria-orientation="vertical" data-placement={position.placement} style={menuStyle}>
      {options.map((option, index) => <div key={option.value} id={optionId(index)} role="option"
        className={`carbon-select-option${active === index ? ' is-active' : ''}`} data-value={option.value}
        aria-selected={option.value === value} aria-disabled={option.disabled || undefined} title={option.label}
        onPointerMove={() => { if (!option.disabled) setActive(index) }}
        onPointerDown={(event) => event.preventDefault()} onClick={() => choose(index)}>
        <span className="carbon-select-option-label">{option.label}</span>
        {option.value === value && <svg className="carbon-select-check" width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true"><path d="m3 8 3.25 3.25L13 4.5" stroke="currentColor" strokeWidth="1.5" /></svg>}
      </div>)}
    </div>, document.body)}
  </span>
}

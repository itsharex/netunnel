import { invoke } from '@tauri-apps/api/core'

export type LogLevel = 'DEBUG' | 'INFO' | 'WARN' | 'ERROR'

export async function log(level: LogLevel, message: string): Promise<void> {
  try {
    await invoke('log_message', { input: { level, message } })
  } catch (e) {
    console.error('[logger] failed to write log:', e)
  }
}

const originalError = console.error
const originalWarn = console.warn
const originalInfo = console.info
const originalLog = console.log

function extractStack(error: unknown): string {
  if (error instanceof Error && error.stack) {
    return error.stack
  }
  return ''
}

function sendToRust(level: LogLevel, args: unknown[]) {
  const parts: string[] = []
  for (const a of args) {
    if (typeof a === 'string') {
      parts.push(a)
    } else if (a instanceof Error && a.stack) {
      parts.push(`${a.message}\n${a.stack}`)
    } else {
      try { parts.push(JSON.stringify(a)) } catch { parts.push(String(a)) }
    }
  }
  void log(level, parts.join(' '))
}

console.error = (...args: unknown[]) => {
  sendToRust('ERROR', args)
  originalError.apply(console, args)
}

console.warn = (...args: unknown[]) => {
  sendToRust('WARN', args)
  originalWarn.apply(console, args)
}

console.info = (...args: unknown[]) => {
  sendToRust('INFO', args)
  originalInfo.apply(console, args)
}

console.log = (...args: unknown[]) => {
  sendToRust('INFO', args)
  originalLog.apply(console, args)
}

window.addEventListener('error', (event) => {
  const stack = event.error?.stack ? `\n${event.error.stack}` : ''
  void log('ERROR', `window.error: ${event.message} at ${event.filename}:${event.lineno}${stack}`)
})

window.addEventListener('unhandledrejection', (event) => {
  const reason = event.reason
  const stack = reason instanceof Error ? reason.stack : ''
  const msg = stack ? `${reason.message}\n${stack}` : String(reason)
  void log('ERROR', `unhandledrejection: ${msg}`)
})

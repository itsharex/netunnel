const { spawnSync } = require('node:child_process')
const path = require('node:path')

const desktopDir = path.resolve(__dirname, '..')
const agentDir = path.resolve(desktopDir, '..', 'netunnel-agent')
const outputPath = path.join(agentDir, 'agent-run.exe')

function resolveTargetTriple() {
  const explicitTarget = process.env.NETUNNEL_AGENT_TARGET?.trim()
  if (explicitTarget) {
    return explicitTarget
  }

  const platform = process.platform
  const arch = process.arch

  if (platform === 'win32') {
    return arch === 'arm64' ? 'aarch64-pc-windows-msvc' : 'x86_64-pc-windows-msvc'
  }

  if (platform === 'darwin') {
    return arch === 'arm64' ? 'aarch64-apple-darwin' : 'x86_64-apple-darwin'
  }

  if (platform === 'linux') {
    return arch === 'arm64' ? 'aarch64-unknown-linux-gnu' : 'x86_64-unknown-linux-gnu'
  }

  throw new Error(`Unsupported platform for agent build: ${platform}/${arch}`)
}

function mapTripleToGoEnv(targetTriple) {
  if (targetTriple.includes('windows')) {
    return {
      GOOS: 'windows',
      GOARCH: targetTriple.startsWith('aarch64') ? 'arm64' : 'amd64',
    }
  }

  if (targetTriple.includes('apple-darwin')) {
    return {
      GOOS: 'darwin',
      GOARCH: targetTriple.startsWith('aarch64') ? 'arm64' : 'amd64',
    }
  }

  if (targetTriple.includes('linux')) {
    return {
      GOOS: 'linux',
      GOARCH: targetTriple.startsWith('aarch64') ? 'arm64' : 'amd64',
    }
  }

  throw new Error(`Unsupported target triple for agent build: ${targetTriple}`)
}

function main() {
  const targetTriple = resolveTargetTriple()
  const goEnv = mapTripleToGoEnv(targetTriple)

  console.log(`[build-agent] target=${targetTriple}`)
  console.log(`[build-agent] output=${outputPath}`)

  const result = spawnSync('go', ['build', '-o', outputPath, './cmd/agent'], {
    cwd: agentDir,
    stdio: 'inherit',
    env: {
      ...process.env,
      CGO_ENABLED: '0',
      GOOS: goEnv.GOOS,
      GOARCH: goEnv.GOARCH,
    },
  })

  if (result.status !== 0) {
    process.exit(result.status ?? 1)
  }
}

main()

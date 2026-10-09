// Every component a template names must be in scope from <script setup>.
//
// Vue warns at runtime rather than failing the build, so an import that got
// dropped while moving markup around shows up as an icon that silently
// disappears — which is exactly what happened to the account menu's 生词本.
import { readFileSync, readdirSync, statSync } from 'node:fs'
import { join } from 'node:path'

// Supplied by Vue itself; they are never imported.
const BUILTIN = new Set(['Transition', 'TransitionGroup', 'KeepAlive', 'Teleport', 'Suspense', 'Component'])

function walk(dir, out = []) {
  for (const name of readdirSync(dir)) {
    const p = join(dir, name)
    if (statSync(p).isDirectory()) walk(p, out)
    else if (p.endsWith('.vue')) out.push(p)
  }
  return out
}

let bad = 0
for (const file of walk('src')) {
  const s = readFileSync(file, 'utf8')
  const script = /<script setup>([\s\S]*?)<\/script>/.exec(s)?.[1] ?? ''
  // Nested <template v-if> means the body runs to the last closing tag.
  const i = s.indexOf('<template>')
  const j = s.lastIndexOf('</template>')
  if (i < 0 || j < i) continue
  const body = s.slice(i, j).replace(/<!--[\s\S]*?-->/g, '')

  const used = new Set([...body.matchAll(/<([A-Z][A-Za-z0-9]*)[\s/>]/g)].map((m) => m[1]))
  const imported = new Set()
  for (const m of script.matchAll(/import\s*\{([^}]*)\}\s*from/g)) {
    for (const raw of m[1].split(',')) {
      const n = raw.trim().split(/\s+as\s+/).pop().trim()
      if (n && /^[A-Z]/.test(n)) imported.add(n)
    }
  }
  for (const m of script.matchAll(/import\s+([A-Z][A-Za-z0-9]*)\s+from/g)) imported.add(m[1])

  const missing = [...used].filter((x) => !imported.has(x) && !BUILTIN.has(x))
  if (missing.length) {
    bad++
    console.error(`${file}: unresolved component ${missing.join(', ')}`)
  }
}
if (bad) {
  console.error(`\n${bad} file(s) reference a component they do not import.`)
  process.exit(1)
}
console.log('component imports: ok')

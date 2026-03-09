<script setup lang="ts">
import MarkdownIt from 'markdown-it'
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import type {
  GenerateMetaYAMLRequest,
  ParseReport,
  ProxyNode,
  ServiceSelection,
  ServiceTree,
  UpdateStatus
} from './types'
import { exportToDesktop, generateMetaYAML, getUpdateStatus, loadServiceTree, parseLinks, startUpdateCheck } from './utils/wails'

const md = new MarkdownIt({ html: false, linkify: true, breaks: true })

const activeStep = ref(0)
const inputText = ref('')
const parsing = ref(false)
const generating = ref(false)
const updateStatus = ref<UpdateStatus>({
  running: true,
  completed: false,
  progress: 0,
  message: '准备检查更新...',
  steps: []
})

const nodes = ref<ProxyNode[]>([])
const parseErrors = ref<ParseReport['errors']>([])
const selectedNodeIds = ref<string[]>([])

const services = ref<ServiceTree[]>([])
const searchKeyword = ref('')
const openedPanels = ref<string[]>([])

interface ChainRoute {
  entryNodeId: string
  exitNodeId: string
}

const chainEntryNodeIds = ref<string[]>([])
const chainExitNodeId = ref('')
const chainRoutes = ref<ChainRoute[]>([])

const proxyGroupName = ref('Proxy_Group')
const directCIDRText = ref('')

const selectionState = reactive<Record<string, { enabled: boolean; policy: string }>>({})
const groupSelectionState = reactive<Record<string, { enabled: boolean; policy: string }>>({})

const yamlPreview = ref('')
const isUpdateReady = computed(() => updateStatus.value.completed)
const updateFailureCount = computed(() => updateStatus.value.steps.filter((item) => item.status === 'failed').length)

const guideMarkdown = `# Meta-Link Pro 使用指南

> 安全提示：**所有数据仅在本地处理，不上传服务器**。
> 启动流程：工具会先检查规则集与 GEOSITE/GEOIP 依赖版本是否为最新，完成前会锁定解析与导出功能。

## Step 1 导入链接
- 粘贴单链、多链或订阅链接（http/https）。
- 系统会实时解析并显示节点与失败归因（例如 \`[TUIC] Token缺失\`）。

## Step 2 配置分流
- 输入强制直连源 IP/CIDR（每行一个，如 \`192.168.1.100\`），命中后始终直连（优先级最高）。
- 在“平台/类别”树中为每个服务指定策略：\`DIRECT\`、\`Proxy_Group\` 或具体节点。
- 支持平台/分类级一键策略：例如可一键让整个 Google 平台都走同一策略。
- 分流树标题会显示各分类的服务数量，便于快速定位大类。
- 国内流量固定直连（\`GEOSITE/CN + GEOIP/CN\`），分流规则优先于全局兜底。
- 未命中任何分流规则时，流量默认走代理兜底（\`MATCH,Proxy_Group\`）。
- 可选：通过“前置代理(入口，多选) + 落地代理(出口，单选)”配置链式代理（\`dialer-proxy\`）。

## Step 3 预览与导出
- 实时生成并高亮 YAML。
- 一键导出到系统桌面。`

const guideHtml = computed(() => md.render(guideMarkdown))

const policyOptions = computed(() => {
  const options = [
    { label: 'DIRECT', value: 'DIRECT' },
    { label: proxyGroupName.value || 'Proxy_Group', value: proxyGroupName.value || 'Proxy_Group' }
  ]
  nodes.value.forEach((node) => {
    options.push({ label: node.name, value: node.name })
  })
  return options
})

const filteredServices = computed(() => {
  const keyword = searchKeyword.value.trim().toLowerCase()
  if (!keyword) return services.value

  const filterNode = (node: ServiceTree): ServiceTree | null => {
    const selfMatch = node.name.toLowerCase().includes(keyword)
    if (!node.children?.length) {
      return selfMatch ? node : null
    }
    const children = node.children
      .map((child) => filterNode(child))
      .filter((child): child is ServiceTree => Boolean(child))

    if (selfMatch || children.length) {
      return { ...node, children }
    }
    return null
  }

  return services.value
    .map((node) => filterNode(node))
    .filter((node): node is ServiceTree => Boolean(node))
})

const highlightedYaml = computed(() => {
  const escaped = yamlPreview.value
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')

  return escaped
    .replace(/^(\s*)([\w-]+:)/gm, '$1<span class="text-sky-300">$2</span>')
    .replace(/\b(RULE-SET|MATCH|IP-CIDR|SRC-IP-CIDR|DIRECT|Proxy_Group)\b/g, '<span class="text-emerald-300">$1</span>')
})

const chainRouteRows = computed(() => {
  return chainRoutes.value
    .map((route) => ({
      key: `${route.entryNodeId}->${route.exitNodeId}`,
      entryNodeId: route.entryNodeId,
      exitNodeId: route.exitNodeId,
      entryName: nodeNameById(route.entryNodeId),
      exitName: nodeNameById(route.exitNodeId)
    }))
    .filter((item) => item.entryName && item.exitName)
})

const chainTopologyRows = computed(() => {
  const grouped = new Map<string, { exitName: string; entries: string[] }>()
  chainRouteRows.value.forEach((route) => {
    const current = grouped.get(route.exitNodeId) ?? { exitName: route.exitName, entries: [] }
    if (!current.entries.includes(route.entryName)) {
      current.entries.push(route.entryName)
    }
    grouped.set(route.exitNodeId, current)
  })

  return Array.from(grouped.entries())
    .map(([exitNodeId, item]) => ({
      exitNodeId,
      exitName: item.exitName,
      entries: item.entries.sort((a, b) => a.localeCompare(b))
    }))
    .sort((a, b) => a.exitName.localeCompare(b.exitName))
})

let parseTimer: ReturnType<typeof setTimeout> | null = null

watch(
  () => inputText.value,
  () => {
    if (parseTimer) clearTimeout(parseTimer)
    parseTimer = setTimeout(() => {
      void handleParse(true)
    }, 450)
  }
)

watch(
  () => nodes.value,
  (newNodes) => {
    const newIDs = newNodes.map((item) => item.id)
    selectedNodeIds.value = selectedNodeIds.value.filter((id) => newIDs.includes(id))
    if (selectedNodeIds.value.length === 0) {
      selectedNodeIds.value = newIDs
    }
    syncChainRoutesWithNodes(newNodes)
  },
  { deep: true }
)

function collectLeafServices(node: ServiceTree): ServiceTree[] {
  if (!node.children?.length) {
    return node.kind === 'service' ? [node] : []
  }
  return node.children.flatMap((child) => collectLeafServices(child))
}

function initSelectionState(tree: ServiceTree[]) {
  const leaves = tree.flatMap((item) => collectLeafServices(item))
  leaves.forEach((leaf) => {
    ensureLeafState(leaf.id)
  })
}

function ensureLeafState(serviceID: string) {
  if (!selectionState[serviceID]) {
    selectionState[serviceID] = {
      enabled: false,
      policy: proxyGroupName.value
    }
  }
  return selectionState[serviceID]
}

function ensureGroupState(groupID: string) {
  if (!groupSelectionState[groupID]) {
    groupSelectionState[groupID] = {
      enabled: false,
      policy: proxyGroupName.value
    }
  }
  return groupSelectionState[groupID]
}

function applyGroupPolicy(group: ServiceTree) {
  const state = ensureGroupState(group.id)
  const leaves = collectLeafServices(group)
  leaves.forEach((leaf) => {
    const leafState = ensureLeafState(leaf.id)
    leafState.enabled = state.enabled
    leafState.policy = state.policy
  })
  ElMessage.success(`已将策略应用到 ${group.name}（${leaves.length} 个服务）`)
}

function findNodeById(nodeId: string): ProxyNode | undefined {
  return nodes.value.find((item) => item.id === nodeId)
}

function nodeNameById(nodeId: string): string {
  return findNodeById(nodeId)?.name ?? ''
}

function syncChainRoutesWithNodes(currentNodes: ProxyNode[]) {
  const idSet = new Set(currentNodes.map((item) => item.id))
  const nameToId = new Map(currentNodes.map((item) => [item.name, item.id]))

  const seen = new Set<string>()
  chainRoutes.value = chainRoutes.value.filter((route) => {
    if (!idSet.has(route.entryNodeId) || !idSet.has(route.exitNodeId) || route.entryNodeId === route.exitNodeId) {
      return false
    }
    const key = `${route.entryNodeId}->${route.exitNodeId}`
    if (seen.has(key)) {
      return false
    }
    seen.add(key)
    return true
  })

  currentNodes.forEach((exitNode) => {
    if (!exitNode.dialerProxy) return
    const entryNodeId = nameToId.get(exitNode.dialerProxy)
    if (!entryNodeId || entryNodeId === exitNode.id) return
    const key = `${entryNodeId}->${exitNode.id}`
    if (seen.has(key)) return
    chainRoutes.value.push({
      entryNodeId,
      exitNodeId: exitNode.id
    })
    seen.add(key)
  })
}

function applyChainRoutes() {
  if (chainEntryNodeIds.value.length === 0 || !chainExitNodeId.value) {
    ElMessage.warning('请先选择至少一个前置代理和一个落地代理')
    return
  }
  const exitNode = findNodeById(chainExitNodeId.value)
  if (!exitNode) {
    ElMessage.error('节点不存在，请重新选择')
    return
  }

  let added = 0
  for (const entryNodeId of chainEntryNodeIds.value) {
    if (entryNodeId === exitNode.id) {
      continue
    }
    const entryNode = findNodeById(entryNodeId)
    if (!entryNode) {
      continue
    }
    const exists = chainRoutes.value.some((item) => item.entryNodeId === entryNode.id && item.exitNodeId === exitNode.id)
    if (exists) {
      continue
    }
    chainRoutes.value.push({ entryNodeId: entryNode.id, exitNodeId: exitNode.id })
    added += 1
  }

  if (added === 0) {
    ElMessage.info('没有新增链路（可能已存在，或前置与落地相同）')
    return
  }
  ElMessage.success(`已新增 ${added} 条链路，落地节点：${exitNode.name}`)
}

function removeChainRoute(entryNodeId: string, exitNodeId: string) {
  chainRoutes.value = chainRoutes.value.filter((item) => !(item.entryNodeId === entryNodeId && item.exitNodeId === exitNodeId))
}

function clearChainRoutes() {
  chainRoutes.value = []
  chainEntryNodeIds.value = []
  chainExitNodeId.value = ''
}

function groupTitle(group: ServiceTree): string {
  return `${group.name} (${collectLeafServices(group).length})`
}

async function loadServices() {
  try {
    const tree = await loadServiceTree()
    services.value = tree
    openedPanels.value = tree.map((item) => item.id)
    initSelectionState(tree)
  } catch {
    const fallback = await fetch('/services.json').then((res) => res.json()) as ServiceTree[]
    services.value = fallback
    openedPanels.value = fallback.map((item) => item.id)
    initSelectionState(fallback)
  }
}

function normalizeProgress(value: number): number {
  if (Number.isNaN(value)) return 0
  if (value < 0) return 0
  if (value > 100) return 100
  return Math.round(value)
}

async function initializeUpdateCheck() {
  try {
    updateStatus.value = await startUpdateCheck()
  } catch (error) {
    updateStatus.value = {
      running: false,
      completed: true,
      progress: 100,
      message: `版本检查启动失败：${(error as Error).message}`,
      steps: []
    }
    return
  }

  if (updateStatus.value.completed) {
    updateStatus.value.progress = normalizeProgress(updateStatus.value.progress || 100)
    return
  }

  while (true) {
    await new Promise((resolve) => setTimeout(resolve, 500))
    try {
      const latest = await getUpdateStatus()
      latest.progress = normalizeProgress(latest.progress)
      updateStatus.value = latest
      if (latest.completed) {
        return
      }
    } catch {
      // Keep polling until backend report becomes available again.
    }
  }
}

async function handleParse(silent = false) {
  if (!isUpdateReady.value) {
    if (!silent) {
      ElMessage.warning('依赖版本检查尚未完成，请稍候')
    }
    return
  }

  if (!inputText.value.trim()) {
    nodes.value = []
    parseErrors.value = []
    yamlPreview.value = ''
    return
  }

  parsing.value = true
  try {
    const result = await parseLinks(inputText.value)
    const oldMap = new Map(nodes.value.map((item) => [item.id, item]))
    nodes.value = result.nodes.map((item) => ({
      ...item,
      dialerProxy: oldMap.get(item.id)?.dialerProxy ?? ''
    }))
    parseErrors.value = result.errors

    if (!silent) {
      ElMessage.success(`解析完成：${result.nodes.length} 个节点，${result.errors.length} 条诊断`)
    }
  } catch (error) {
    if (!silent) {
      ElMessage.error((error as Error).message)
    }
  } finally {
    parsing.value = false
  }
}

function setStep(step: number) {
  activeStep.value = step
}

function buildSelections(): ServiceSelection[] {
  return Object.entries(selectionState).map(([serviceId, state]) => ({
    serviceId,
    enabled: state.enabled,
    policy: state.policy
  }))
}

function parseCIDRInput(): string[] {
  return directCIDRText.value
    .split(/[,\n\r\t ]+/)
    .map((item) => item.trim())
    .filter(Boolean)
}

function buildNodesWithChainRoutes(baseNodes: ProxyNode[]) {
  const nodeByID = new Map(baseNodes.map((item) => [item.id, item]))
  const outputNodes = baseNodes.map((item) => ({ ...item, dialerProxy: '' }))
  const selected = new Set(selectedNodeIds.value)

  const routesByExit = new Map<string, string[]>()
  chainRoutes.value.forEach((route) => {
    if (route.entryNodeId === route.exitNodeId) {
      return
    }
    const entry = nodeByID.get(route.entryNodeId)
    const exit = nodeByID.get(route.exitNodeId)
    if (!entry || !exit) {
      return
    }
    const current = routesByExit.get(route.exitNodeId) ?? []
    if (!current.includes(route.entryNodeId)) {
      current.push(route.entryNodeId)
    }
    routesByExit.set(route.exitNodeId, current)
  })

  routesByExit.forEach((entryIDs, exitID) => {
    const exitNode = nodeByID.get(exitID)
    if (!exitNode) return

    if (entryIDs.length === 1) {
      const entryNode = nodeByID.get(entryIDs[0])
      const target = outputNodes.find((item) => item.id === exitID)
      if (entryNode && target) {
        target.dialerProxy = entryNode.name
      }
      return
    }

    entryIDs.forEach((entryID) => {
      const entryNode = nodeByID.get(entryID)
      if (!entryNode) return
      const cloneID = `${exitNode.id}__via__${entryNode.id}`
      const cloneName = `${exitNode.name} [via ${entryNode.name}]`
      outputNodes.push({
        ...exitNode,
        id: cloneID,
        name: cloneName,
        dialerProxy: entryNode.name
      })
      if (selected.has(exitNode.id)) {
        selected.add(cloneID)
      }
    })
  })

  return {
    nodes: outputNodes,
    selectedNodeIds: Array.from(selected)
  }
}

async function handleGenerate() {
  if (!isUpdateReady.value) {
    ElMessage.warning('依赖版本检查尚未完成，请稍候')
    return
  }

  generating.value = true
  try {
    const routePrepared = buildNodesWithChainRoutes(nodes.value)
    const payload: GenerateMetaYAMLRequest = {
      nodes: routePrepared.nodes,
      selectedNodeIds: routePrepared.selectedNodeIds,
      directCidrs: parseCIDRInput(),
      selections: buildSelections(),
      mode: 'blacklist',
      proxyGroupName: proxyGroupName.value,
      servicesSnapshot: services.value
    }

    yamlPreview.value = await generateMetaYAML(payload)
    activeStep.value = 2
    ElMessage.success('YAML 已生成')
  } catch (error) {
    ElMessage.error((error as Error).message)
  } finally {
    generating.value = false
  }
}

async function handleExport() {
  if (!isUpdateReady.value) {
    ElMessage.warning('依赖版本检查尚未完成，请稍候')
    return
  }

  if (!yamlPreview.value.trim()) {
    ElMessage.warning('请先生成 YAML')
    return
  }
  try {
    const path = await exportToDesktop(yamlPreview.value)
    ElMessage.success(`已导出到：${path}`)
  } catch (error) {
    ElMessage.error((error as Error).message)
  }
}

onMounted(async () => {
  await initializeUpdateCheck()
  await loadServices()
})
</script>

<template>
  <div class="mx-auto max-w-[1300px] p-4 md:p-8 text-slate-100 fade-in">
    <header class="mb-6 rounded-2xl glass p-5">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 class="font-display text-3xl font-semibold tracking-wide">Meta-Link Pro</h1>
          <p class="text-sm text-slate-300 mt-1">将代理链接转换为 OpenClash Meta (Mihomo) 高级 YAML 配置</p>
        </div>
        <el-tag type="success" effect="dark">本地处理 · 无服务器上传</el-tag>
      </div>
    </header>

    <section class="mb-6 rounded-2xl glass p-5">
      <div class="flex flex-wrap items-center justify-between gap-3 mb-3">
        <h2 class="font-display text-xl">启动依赖版本检查</h2>
        <el-tag :type="isUpdateReady ? 'success' : 'warning'" effect="dark">
          {{ isUpdateReady ? '检查完成' : '检查中' }}
        </el-tag>
      </div>
      <el-progress
        :percentage="normalizeProgress(updateStatus.progress)"
        :status="isUpdateReady ? 'success' : undefined"
        :stroke-width="14"
      />
      <p class="text-xs text-slate-300 mt-2">{{ updateStatus.message }}</p>
      <p v-if="isUpdateReady && updateFailureCount > 0" class="text-xs text-amber-300 mt-1">
        有 {{ updateFailureCount }} 个依赖版本检查失败，工具仍可使用，建议稍后重试网络环境。
      </p>
      <div class="mt-3 max-h-28 overflow-auto pr-1 space-y-1">
        <div
          v-for="(step, idx) in updateStatus.steps"
          :key="`update-step-${idx}-${step.url}`"
          class="text-xs rounded-md bg-slate-900/40 px-2 py-1 border border-slate-700/70"
        >
          <span class="font-medium">{{ step.name }}</span>
          <span class="mx-1">·</span>
          <span>{{ step.status }}</span>
          <span class="mx-1">·</span>
          <span class="text-slate-300">{{ step.detail }}</span>
        </div>
      </div>
    </section>

    <section class="mb-6 rounded-2xl glass p-5">
      <el-steps :active="activeStep" finish-status="success" simple>
        <el-step title="Step 1 导入" />
        <el-step title="Step 2 分流" />
        <el-step title="Step 3 导出" />
      </el-steps>
    </section>

    <section v-show="activeStep === 0" class="grid grid-cols-1 lg:grid-cols-2 gap-4">
      <div class="rounded-2xl glass p-5">
        <h2 class="mb-3 font-display text-xl">使用指南</h2>
        <article class="prose prose-invert max-w-none" v-html="guideHtml" />
      </div>

      <div class="rounded-2xl glass p-5">
        <h2 class="mb-3 font-display text-xl">导入链接</h2>
        <el-input
          v-model="inputText"
          type="textarea"
          :rows="14"
          resize="vertical"
          placeholder="粘贴单链、多链或订阅链接"
        />

        <div class="mt-4 flex flex-wrap items-center gap-3">
          <el-button type="primary" :loading="parsing" :disabled="!isUpdateReady" @click="handleParse()">立即解析</el-button>
          <el-button :disabled="nodes.length === 0 || !isUpdateReady" @click="setStep(1)">下一步</el-button>
          <span class="text-xs text-slate-300">解析结果会实时刷新</span>
        </div>

        <div class="mt-4">
          <h3 class="text-sm font-semibold text-slate-300 mb-2">节点列表</h3>
          <div class="space-y-2 max-h-48 overflow-auto pr-1">
            <div v-for="node in nodes" :key="node.id" class="rounded-lg bg-slate-900/50 px-3 py-2 border border-slate-700/70">
              <div class="flex items-center justify-between gap-2">
                <span class="text-sm">{{ node.name }}</span>
                <el-tag size="small">{{ node.protocol.toUpperCase() }}</el-tag>
              </div>
              <div class="text-xs text-slate-400 mt-1">{{ node.server }}:{{ node.port }}</div>
            </div>
          </div>
        </div>

        <div class="mt-4">
          <h3 class="text-sm font-semibold text-slate-300 mb-2">失败诊断</h3>
          <div class="space-y-2 max-h-36 overflow-auto pr-1">
            <div
              v-for="(item, idx) in parseErrors"
              :key="`${item.protocol}-${item.field}-${idx}`"
              class="rounded-lg bg-rose-900/30 border border-rose-700/60 px-3 py-2 text-xs text-rose-200"
            >
              {{ item.message }}
            </div>
            <div v-if="parseErrors.length === 0" class="text-xs text-slate-400">暂无错误</div>
          </div>
        </div>
      </div>
    </section>

    <section v-show="activeStep === 1" class="space-y-4">
      <div class="rounded-2xl glass p-5">
        <h2 class="mb-3 font-display text-xl">全局代理与强制直连</h2>
        <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div>
            <label class="text-sm text-slate-300">全局兜底策略</label>
            <div class="mt-2 text-xs text-slate-300 rounded-lg bg-slate-900/40 border border-slate-700/70 px-3 py-2">
              固定为 <code>MATCH, {{ proxyGroupName || 'Proxy_Group' }}</code>（未命中分流规则的流量默认走代理）。
            </div>
            <p class="text-xs text-slate-400 mt-2">
              本工具已移除白名单模式，仅保留分流与黑名单直连逻辑：国内流量固定直连，其他流量默认代理，分流规则优先。
            </p>
          </div>
          <div>
            <label class="text-sm text-slate-300">代理组名称</label>
            <el-input v-model="proxyGroupName" class="mt-2" placeholder="Proxy_Group" />
          </div>
        </div>

        <div class="mt-4">
          <label class="text-sm text-slate-300">生效节点</label>
          <el-checkbox-group v-model="selectedNodeIds" class="mt-2 flex flex-wrap gap-3">
            <el-checkbox v-for="node in nodes" :key="`select-${node.id}`" :label="node.id">
              {{ node.name }}
            </el-checkbox>
          </el-checkbox-group>
        </div>

        <div class="mt-4">
          <label class="text-sm text-slate-300">强制直连源 IP 黑名单（最高优先级，SRC-IP/CIDR）</label>
          <el-input
            v-model="directCIDRText"
            type="textarea"
            :rows="4"
            class="mt-2"
            placeholder="示例: 192.168.1.100 / 10.0.0.0/24 / 172.16.0.0/16"
          />
          <p class="text-xs text-slate-400 mt-2">
            含义：这里匹配的是源 IP（SRC-IP-CIDR），命中后无论访问哪个平台/服务都强制 DIRECT。示例：填入
            <code>10.0.0.0/24</code> 后，该网段来源流量全部直连。
          </p>
        </div>
      </div>

      <div class="rounded-2xl glass p-5">
        <h2 class="mb-3 font-display text-xl">链式代理 (Dialer Proxy)</h2>
        <p class="text-xs text-slate-300 mb-3">支持多个前置代理对应一个落地代理。单前置会直接写入该落地节点；多前置会自动生成多个 <code>[via xxx]</code> 链式节点。</p>
        <div class="grid grid-cols-1 md:grid-cols-3 gap-3">
          <el-select
            v-model="chainEntryNodeIds"
            multiple
            collapse-tags
            collapse-tags-tooltip
            placeholder="选择前置代理(入口，可多选)"
          >
            <el-option
              v-for="candidate in nodes"
              :key="`entry-${candidate.id}`"
              :label="candidate.name"
              :value="candidate.id"
            />
          </el-select>
          <el-select v-model="chainExitNodeId" placeholder="选择落地代理(出口)">
            <el-option
              v-for="candidate in nodes.filter((it) => !chainEntryNodeIds.includes(it.id))"
              :key="`exit-${candidate.id}`"
              :label="candidate.name"
              :value="candidate.id"
            />
          </el-select>
          <div class="flex gap-2">
            <el-button type="primary" @click="applyChainRoutes">应用链路</el-button>
            <el-button @click="clearChainRoutes">清空链路</el-button>
          </div>
        </div>

        <div class="mt-4 space-y-2 max-h-48 overflow-auto pr-1">
          <div class="rounded-lg bg-slate-900/50 px-3 py-2 border border-slate-700/70">
            <div class="text-xs text-slate-300 mb-2">链路拓扑预览（入口 → 出口）</div>
            <div v-if="chainTopologyRows.length === 0" class="text-xs text-slate-400">暂无拓扑</div>
            <div v-for="item in chainTopologyRows" :key="`topo-${item.exitNodeId}`" class="text-xs text-slate-200">
              {{ item.entries.join(' , ') }} → {{ item.exitName }}
            </div>
          </div>
          <div
            v-for="route in chainRouteRows"
            :key="`route-${route.key}`"
            class="rounded-lg bg-slate-900/50 px-3 py-2 border border-slate-700/70 flex items-center justify-between gap-3"
          >
            <div class="text-sm text-slate-100">{{ route.entryName }} → {{ route.exitName }}</div>
            <el-button size="small" type="danger" plain @click="removeChainRoute(route.entryNodeId, route.exitNodeId)">删除</el-button>
          </div>
          <div v-if="chainRouteRows.length === 0" class="text-xs text-slate-400">暂无链式代理配置</div>
        </div>
      </div>

      <div class="rounded-2xl glass p-5">
        <div class="flex items-center justify-between gap-3 mb-3">
          <h2 class="font-display text-xl">分流对象树</h2>
          <el-input v-model="searchKeyword" placeholder="搜索服务（如 YouTube / OpenAI）" clearable class="max-w-xs" />
        </div>

        <el-collapse v-model="openedPanels">
          <el-collapse-item
            v-for="group in filteredServices"
            :key="group.id"
            :name="group.id"
            :title="groupTitle(group)"
          >
            <div class="space-y-3">
              <div class="rounded-lg bg-slate-900/40 px-3 py-3 border border-slate-700/70">
                <div class="flex flex-wrap items-center justify-between gap-2">
                  <div class="text-xs text-slate-300">分类级策略（应用到当前分类全部服务）</div>
                  <div class="flex items-center gap-2">
                    <el-switch
                      v-model="ensureGroupState(group.id).enabled"
                      inline-prompt
                      active-text="启用"
                      inactive-text="关闭"
                    />
                    <el-select
                      v-model="ensureGroupState(group.id).policy"
                      class="min-w-[200px]"
                      :disabled="!ensureGroupState(group.id).enabled"
                    >
                      <el-option
                        v-for="option in policyOptions"
                        :key="`${group.id}-group-${option.value}`"
                        :label="option.label"
                        :value="option.value"
                      />
                    </el-select>
                    <el-button type="primary" plain @click="applyGroupPolicy(group)">应用到本分类</el-button>
                  </div>
                </div>
              </div>
              <div
                v-for="leaf in collectLeafServices(group)"
                :key="leaf.id"
                class="rounded-lg bg-slate-900/50 px-3 py-3 border border-slate-700/70"
              >
                <div class="flex flex-wrap items-center justify-between gap-2">
                  <div class="text-sm">{{ leaf.name }}</div>
                  <div class="flex items-center gap-2">
                    <el-switch v-model="ensureLeafState(leaf.id).enabled" inline-prompt active-text="启用" inactive-text="关闭" />
                    <el-select
                      v-model="ensureLeafState(leaf.id).policy"
                      class="min-w-[200px]"
                      :disabled="!ensureLeafState(leaf.id).enabled"
                    >
                      <el-option
                        v-for="option in policyOptions"
                        :key="`${leaf.id}-${option.value}`"
                        :label="option.label"
                        :value="option.value"
                      />
                    </el-select>
                  </div>
                </div>
              </div>
            </div>
          </el-collapse-item>
        </el-collapse>
      </div>

      <div class="flex flex-wrap gap-3">
        <el-button @click="setStep(0)">上一步</el-button>
        <el-button type="primary" :loading="generating" :disabled="!isUpdateReady" @click="handleGenerate">生成 YAML</el-button>
      </div>
    </section>

    <section v-show="activeStep === 2" class="space-y-4">
      <div class="rounded-2xl glass p-5">
        <div class="flex flex-wrap items-center justify-between gap-3 mb-3">
          <h2 class="font-display text-xl">YAML 预览</h2>
          <div class="flex gap-2">
            <el-button @click="setStep(1)">返回调整</el-button>
            <el-button type="primary" :disabled="!isUpdateReady" @click="handleExport">导出到桌面</el-button>
          </div>
        </div>
        <pre class="code-area rounded-xl p-4 overflow-auto text-sm leading-6"><code v-html="highlightedYaml" /></pre>
      </div>
    </section>
  </div>
</template>

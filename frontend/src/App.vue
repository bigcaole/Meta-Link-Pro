<script setup lang="ts">
import MarkdownIt from 'markdown-it'
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import type {
  GenerateMetaYAMLRequest,
  ParseMode,
  ParseReport,
  ProxyNode,
  ServiceSelection,
  ServiceTree
} from './types'
import { exportToDesktop, generateMetaYAML, loadServiceTree, parseLinks } from './utils/wails'

const md = new MarkdownIt({ html: false, linkify: true, breaks: true })

const activeStep = ref(0)
const inputText = ref('')
const parsing = ref(false)
const generating = ref(false)

const nodes = ref<ProxyNode[]>([])
const parseErrors = ref<ParseReport['errors']>([])
const selectedNodeIds = ref<string[]>([])

const services = ref<ServiceTree[]>([])
const searchKeyword = ref('')
const openedPanels = ref<string[]>([])

const mode = ref<ParseMode>('whitelist')
const proxyGroupName = ref('Proxy_Group')
const directCIDRText = ref('')

const selectionState = reactive<Record<string, { enabled: boolean; policy: string }>>({})

const yamlPreview = ref('')

const guideMarkdown = `# Meta-Link Pro 使用指南

> 安全提示：**所有数据仅在本地处理，不上传服务器**。

## Step 1 导入链接
- 粘贴单链、多链或订阅链接（http/https）。
- 系统会实时解析并显示节点与失败归因（例如 \`[TUIC] Token缺失\`）。

## Step 2 配置分流
- 输入强制直连 IP/CIDR（每行一个，如 \`192.168.1.100\`）。
- 在“平台/类别”树中为每个服务指定策略：\`DIRECT\`、\`Proxy_Group\` 或具体节点。
- 选择全局模式：
  - 白名单：只代理已勾选服务，其余直连。
  - 黑名单：默认代理，仅把勾选服务设为直连或自定义策略。
- 可选：设置节点链式前置代理（\`dialer-proxy\`）。

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
    .replace(/\b(RULE-SET|MATCH|IP-CIDR|DIRECT|Proxy_Group)\b/g, '<span class="text-emerald-300">$1</span>')
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
      policy: mode.value === 'blacklist' ? 'DIRECT' : proxyGroupName.value
    }
  }
  return selectionState[serviceID]
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

async function handleParse(silent = false) {
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

async function handleGenerate() {
  generating.value = true
  try {
    const payload: GenerateMetaYAMLRequest = {
      nodes: nodes.value,
      selectedNodeIds: selectedNodeIds.value,
      directCidrs: parseCIDRInput(),
      selections: buildSelections(),
      mode: mode.value,
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
          <el-button type="primary" :loading="parsing" @click="handleParse()">立即解析</el-button>
          <el-button :disabled="nodes.length === 0" @click="setStep(1)">下一步</el-button>
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
        <h2 class="mb-3 font-display text-xl">全局策略与强制直连</h2>
        <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div>
            <label class="text-sm text-slate-300">全局策略开关</label>
            <el-radio-group v-model="mode" class="mt-2">
              <el-radio-button label="whitelist">白名单模式</el-radio-button>
              <el-radio-button label="blacklist">黑名单模式</el-radio-button>
            </el-radio-group>
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
          <label class="text-sm text-slate-300">强制直连黑名单（IP/CIDR）</label>
          <el-input
            v-model="directCIDRText"
            type="textarea"
            :rows="4"
            class="mt-2"
            placeholder="示例: 192.168.1.100 或 10.0.0.0/24"
          />
        </div>
      </div>

      <div class="rounded-2xl glass p-5">
        <h2 class="mb-3 font-display text-xl">链式代理 (Dialer Proxy)</h2>
        <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
          <div v-for="node in nodes" :key="`dial-${node.id}`" class="rounded-lg bg-slate-900/50 px-3 py-2 border border-slate-700/70">
            <div class="text-sm mb-2">{{ node.name }}</div>
            <el-select v-model="node.dialerProxy" clearable placeholder="无前置代理" class="w-full">
              <el-option
                v-for="candidate in nodes.filter((it) => it.id !== node.id)"
                :key="candidate.id"
                :label="candidate.name"
                :value="candidate.name"
              />
            </el-select>
          </div>
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
            :title="group.name"
          >
            <div class="space-y-3">
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
        <el-button type="primary" :loading="generating" @click="handleGenerate">生成 YAML</el-button>
      </div>
    </section>

    <section v-show="activeStep === 2" class="space-y-4">
      <div class="rounded-2xl glass p-5">
        <div class="flex flex-wrap items-center justify-between gap-3 mb-3">
          <h2 class="font-display text-xl">YAML 预览</h2>
          <div class="flex gap-2">
            <el-button @click="setStep(1)">返回调整</el-button>
            <el-button type="primary" @click="handleExport">导出到桌面</el-button>
          </div>
        </div>
        <pre class="code-area rounded-xl p-4 overflow-auto text-sm leading-6"><code v-html="highlightedYaml" /></pre>
      </div>
    </section>
  </div>
</template>

export type ParseMode = 'blacklist'

export type Protocol = 'vless' | 'tuic' | 'hysteria2' | 'ss' | 'trojan' | 'vmess'

export interface ParseIssue {
  protocol: string
  field: string
  message: string
}

export interface ProxyNode {
  id: string
  name: string
  protocol: Protocol
  server: string
  port: number
  uuid?: string
  token?: string
  password?: string
  cipher?: string
  plugin?: string
  pluginOpts?: string
  network?: string
  tls?: boolean
  sni?: string
  alpn?: string
  flow?: string
  security?: string
  publicKey?: string
  shortId?: string
  fingerprint?: string
  serviceName?: string
  host?: string
  path?: string
  congestionControl?: string
  udpRelayMode?: string
  dialerProxy?: string
  rawLink: string
}

export interface ServiceTree {
  id: string
  name: string
  kind: 'platform' | 'category' | 'service'
  provider?: string
  ruleType?: string
  domains?: string[]
  keywords?: string[]
  ipCidrs?: string[]
  defaultOut?: string
  children?: ServiceTree[]
}

export interface ServiceSelection {
  serviceId: string
  policy: string
  enabled: boolean
}

export interface ParseReport {
  nodes: ProxyNode[]
  errors: ParseIssue[]
}

export interface UpdateStep {
  name: string
  url: string
  status: 'running' | 'ok' | 'failed' | string
  detail: string
}

export interface UpdateStatus {
  running: boolean
  completed: boolean
  progress: number
  message: string
  startedAt?: string
  finishedAt?: string
  steps: UpdateStep[]
}

export interface GenerateMetaYAMLRequest {
  nodes: ProxyNode[]
  selectedNodeIds: string[]
  directCidrs: string[]
  selections: ServiceSelection[]
  mode: ParseMode
  proxyGroupName: string
  servicesSnapshot: ServiceTree[]
}

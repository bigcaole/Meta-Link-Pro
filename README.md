# Meta-Link Pro

Meta-Link Pro 是一个基于 **Go + Wails v3 + Vue3 + Tailwind + Element Plus** 的本地桌面工具，用于把代理链接转换为 OpenClash Meta (Mihomo) YAML。

## 当前已实现

- 后端数据建模：`ProxyNode`、`ServiceTree`、`GenerateMetaYAMLRequest`
- 协议解析：VLESS(Reality)、TUIC v5、Hysteria2、SS(含 2022 兼容输入)、Trojan、VMess
- 完整 YAML 解码：支持复杂嵌套、anchors/aliases、`<<` merge key
- 失败诊断：返回协议级错误（例如 `[TUIC] Token缺失`）
- 分流模型：`services.json` 独立维护平台/分类/服务树
- YAML 生成：
  - fake-ip DNS 预设
  - rule-providers 自动注入（含 Loyalsoldier 与 Blackmatrix7）
  - 规则优先级：自定义直连 IP > 特定服务 > 全局规则 > MATCH
  - 白名单/黑名单模式切换
  - `dialer-proxy` 链式代理输出
- 前端三步流程：导入解析 -> 分流配置 -> YAML 预览与导出
- 导出：后端 `ExportToDesktop` 写入系统桌面
- CLI 可执行入口：可直接输入链接/订阅并生成 YAML（用于 Windows `.exe` 测试）

## 目录

- `backend/models`: 数据结构
- `backend/engine`: 解析与 YAML 引擎
- `backend/services/services.json`: 可扩展服务分类
- `frontend/src/App.vue`: 三步 UI 主流程
- `frontend/src/utils/wails.ts`: Wails 调用适配

## 测试

```bash
go test ./...
```

## Windows EXE 自动打包

- 工作流文件：`.github/workflows/windows-exe.yml`
- 触发方式：推送到 `main` 或 `codex/**` 分支，或手动 `workflow_dispatch`
- 下载路径：GitHub Actions 运行详情中的 Artifact `meta-link-pro-windows-amd64`

CLI 示例：

```bash
Meta-Link-Pro.exe -input-file input.txt -mode blacklist -output meta.yaml
```

## 接入 Wails v3

在 Wails v3 启动代码里绑定 `backend.NewApp()`，并将其方法暴露给前端：

- `ParseLinks(input string)`
- `LoadServiceTree()`
- `GenerateMetaYAML(req)`
- `ExportToDesktop(content string)`

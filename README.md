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
- Wails GUI 主入口：`main.go`（已接入真实桌面窗口与服务绑定）
- CLI 可执行入口：`cmd/metalink-cli/main.go`（可选）

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
- 触发方式：
  - 推送到 `main` / `codex/**`：产出 Actions Artifact
  - 推送标签 `v*`：自动创建 GitHub Release 并上传安装包
- 下载路径：
  - Artifact：Actions 运行详情中的 `meta-link-pro-windows-amd64`
  - Release：仓库 `Releases` 页面

Wails GUI 本地构建：

```bash
wails3 task windows:build PRODUCTION=true
```

CLI 示例（可选）：

```bash
go run ./cmd/metalink-cli -input-file input.txt -mode blacklist -output meta.yaml
```

## 接入 Wails v3

Wails 服务绑定已完成，前端可调用以下后端方法：

- `ParseLinks(input string)`
- `LoadServiceTree()`
- `GenerateMetaYAML(req)`
- `ExportToDesktop(content string)`

package engine

import (
	"fmt"
	"net"
	neturl "net/url"
	"regexp"
	"sort"
	"strings"

	"meta-link-pro/backend/models"
	"meta-link-pro/backend/services"
)

type providerMeta struct {
	Type     string
	Behavior string
	Format   string
	URL      string
	Path     string
}

var providerNameCleaner = regexp.MustCompile(`[^a-z0-9._-]+`)

var builtinProviders = map[string]providerMeta{
	"private": {
		Type:     "http",
		Behavior: "classical",
		Format:   "text",
		URL:      "https://raw.githubusercontent.com/Loyalsoldier/clash-rules/release/private.txt",
		Path:     "./ruleset/private.txt",
	},
	"cn": {
		Type:     "http",
		Behavior: "classical",
		Format:   "text",
		URL:      "https://raw.githubusercontent.com/Loyalsoldier/clash-rules/release/cncidr.txt",
		Path:     "./ruleset/cn.txt",
	},
	"geolocation-!cn": {
		Type:     "http",
		Behavior: "classical",
		Format:   "text",
		URL:      "https://raw.githubusercontent.com/Loyalsoldier/clash-rules/release/gfw.txt",
		Path:     "./ruleset/gfw.txt",
	},
	"youtube": {
		Type:     "http",
		Behavior: "classical",
		Format:   "yaml",
		URL:      "https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Clash/YouTube/YouTube.yaml",
		Path:     "./ruleset/youtube.yaml",
	},
	"gmail": {
		Type:     "http",
		Behavior: "classical",
		Format:   "yaml",
		URL:      "https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Clash/Google/Google.yaml",
		Path:     "./ruleset/google.yaml",
	},
	"google-search": {
		Type:     "http",
		Behavior: "classical",
		Format:   "yaml",
		URL:      "https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Clash/Google/Google.yaml",
		Path:     "./ruleset/google.yaml",
	},
	"gemini": {
		Type:     "http",
		Behavior: "classical",
		Format:   "yaml",
		URL:      "https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Clash/Gemini/Gemini.yaml",
		Path:     "./ruleset/gemini.yaml",
	},
	"office": {
		Type:     "http",
		Behavior: "classical",
		Format:   "yaml",
		URL:      "https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Clash/OneDrive/OneDrive.yaml",
		Path:     "./ruleset/office.yaml",
	},
	"azure": {
		Type:     "http",
		Behavior: "classical",
		Format:   "yaml",
		URL:      "https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Clash/Microsoft/Microsoft.yaml",
		Path:     "./ruleset/azure.yaml",
	},
	"icloud": {
		Type:     "http",
		Behavior: "classical",
		Format:   "yaml",
		URL:      "https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Clash/iCloud/iCloud.yaml",
		Path:     "./ruleset/icloud.yaml",
	},
	"appstore": {
		Type:     "http",
		Behavior: "classical",
		Format:   "yaml",
		URL:      "https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Clash/AppStore/AppStore.yaml",
		Path:     "./ruleset/appstore.yaml",
	},
	"applemusic": {
		Type:     "http",
		Behavior: "classical",
		Format:   "yaml",
		URL:      "https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Clash/AppleMusic/AppleMusic.yaml",
		Path:     "./ruleset/applemusic.yaml",
	},
	"openai": {
		Type:     "http",
		Behavior: "classical",
		Format:   "yaml",
		URL:      "https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Clash/OpenAI/OpenAI.yaml",
		Path:     "./ruleset/openai.yaml",
	},
	"claude": {
		Type:     "http",
		Behavior: "classical",
		Format:   "yaml",
		URL:      "https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Clash/Claude/Claude.yaml",
		Path:     "./ruleset/claude.yaml",
	},
	"netflix": {
		Type:     "http",
		Behavior: "classical",
		Format:   "yaml",
		URL:      "https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Clash/Netflix/Netflix.yaml",
		Path:     "./ruleset/netflix.yaml",
	},
	"disney": {
		Type:     "http",
		Behavior: "classical",
		Format:   "yaml",
		URL:      "https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Clash/Disney/Disney.yaml",
		Path:     "./ruleset/disney.yaml",
	},
	"telegram": {
		Type:     "http",
		Behavior: "ipcidr",
		Format:   "yaml",
		URL:      "https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Clash/Telegram/Telegram.yaml",
		Path:     "./ruleset/telegram.yaml",
	},
	"x": {
		Type:     "http",
		Behavior: "classical",
		Format:   "yaml",
		URL:      "https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Clash/Twitter/Twitter.yaml",
		Path:     "./ruleset/x.yaml",
	},
	"steam": {
		Type:     "http",
		Behavior: "classical",
		Format:   "yaml",
		URL:      "https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Clash/Steam/Steam.yaml",
		Path:     "./ruleset/steam.yaml",
	},
	"epic": {
		Type:     "http",
		Behavior: "classical",
		Format:   "yaml",
		URL:      "https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Clash/Epic/Epic.yaml",
		Path:     "./ruleset/epic.yaml",
	},
}

func RuleProviderURLs() []string {
	seen := make(map[string]struct{})
	urls := make([]string, 0, len(builtinProviders))
	for _, meta := range builtinProviders {
		url := strings.TrimSpace(meta.URL)
		if !isValidRuleURL(url) {
			continue
		}
		if _, ok := seen[url]; ok {
			continue
		}
		seen[url] = struct{}{}
		urls = append(urls, url)
	}
	sort.Strings(urls)
	return urls
}

func GenerateMetaYAML(req models.GenerateMetaYAMLRequest) (string, error) {
	proxyGroup := strings.TrimSpace(req.ProxyGroupName)
	if proxyGroup == "" {
		proxyGroup = "Proxy_Group"
	}
	mode := req.Mode
	if mode != models.ModeWhitelist {
		mode = models.ModeBlacklist
	}

	selected := selectNodes(req.Nodes, req.SelectedNodeIDs)
	if len(selected) == 0 {
		return "", fmt.Errorf("请至少选择一个节点")
	}

	serviceMap := map[string]models.ServiceTree{}
	if len(req.Selections) > 0 {
		serviceTree := req.ServicesSnapshot
		if len(serviceTree) == 0 {
			loaded, err := services.LoadServiceTree()
			if err != nil {
				return "", err
			}
			serviceTree = loaded
		}
		serviceMap = services.FlattenServices(serviceTree)
	}

	providerDefs := make(map[string]providerMeta)
	providerURLToName := make(map[string]string)
	providerRules := make([]string, 0)
	serviceRules := make([]string, 0)
	addProvider := func(hint string, meta providerMeta) (string, bool) {
		if !isValidRuleURL(meta.URL) {
			return "", false
		}
		if existing, ok := providerURLToName[meta.URL]; ok {
			return existing, true
		}

		name := sanitizeProviderName(hint)
		if name == "" {
			name = "provider"
		}
		original := name
		idx := 1
		for {
			if _, exists := providerDefs[name]; !exists {
				break
			}
			name = fmt.Sprintf("%s-%d", original, idx)
			idx++
		}

		if strings.TrimSpace(meta.Path) == "" {
			meta.Path = fmt.Sprintf("./ruleset/%s.%s", name, providerFileExt(meta.Format))
		}
		providerDefs[name] = meta
		providerURLToName[meta.URL] = name
		return name, true
	}

	for _, globalProvider := range []string{"private", "cn"} {
		if name, meta, ok := resolveProviderMeta(models.ServiceTree{ID: globalProvider, Provider: globalProvider}); ok {
			addProvider(name, meta)
		}
	}

	for _, s := range req.Selections {
		if !s.Enabled {
			continue
		}
		item, ok := serviceMap[s.ServiceID]
		if !ok {
			continue
		}
		policy := strings.TrimSpace(s.Policy)
		if policy == "" {
			policy = proxyGroup
		}

		providerBound := false
		if providerNameHint, providerMeta, ok := resolveProviderMeta(item); ok {
			if providerName, added := addProvider(providerNameHint, providerMeta); added {
				providerRules = append(providerRules, fmt.Sprintf("RULE-SET,%s,%s", providerName, policy))
				providerBound = true
			}
		}
		if !providerBound {
			serviceRules = append(serviceRules, buildServiceRules(item, policy)...)
		}
	}

	directCIDRRules := make([]string, 0)
	for _, item := range req.DirectCIDRs {
		if cidr := normalizeCIDR(item); cidr != "" {
			directCIDRRules = append(directCIDRRules, fmt.Sprintf("SRC-IP-CIDR,%s,DIRECT", cidr))
		}
	}

	directCIDRRules = uniqueStrings(directCIDRRules)
	serviceRules = uniqueStrings(serviceRules)
	providerRules = uniqueStrings(providerRules)

	builder := &strings.Builder{}
	builder.WriteString("mixed-port: 7890\n")
	builder.WriteString("allow-lan: true\n")
	builder.WriteString("mode: rule\n")
	builder.WriteString("log-level: info\n")
	builder.WriteString("find-process-mode: strict\n")
	builder.WriteString("unified-delay: true\n")
	builder.WriteString("ipv6: false\n")
	builder.WriteString("\n")
	builder.WriteString("dns:\n")
	builder.WriteString("  enable: true\n")
	builder.WriteString("  ipv6: false\n")
	builder.WriteString("  enhanced-mode: fake-ip\n")
	builder.WriteString("  fake-ip-range: 198.18.0.1/16\n")
	builder.WriteString("  default-nameserver:\n")
	builder.WriteString("    - 223.5.5.5\n")
	builder.WriteString("    - 119.29.29.29\n")
	builder.WriteString("  nameserver:\n")
	builder.WriteString("    - https://dns.alidns.com/dns-query\n")
	builder.WriteString("    - https://dns.cloudflare.com/dns-query\n")
	builder.WriteString("  fallback:\n")
	builder.WriteString("    - tls://1.1.1.1:853\n")
	builder.WriteString("    - tls://8.8.8.8:853\n")
	builder.WriteString("\n")

	builder.WriteString("proxies:\n")
	for _, node := range selected {
		writeProxy(builder, node)
	}
	builder.WriteString("\n")

	builder.WriteString("proxy-groups:\n")
	builder.WriteString(fmt.Sprintf("  - name: %s\n", quote(proxyGroup)))
	builder.WriteString("    type: select\n")
	builder.WriteString("    proxies:\n")
	for _, node := range selected {
		builder.WriteString(fmt.Sprintf("      - %s\n", quote(node.Name)))
	}
	builder.WriteString("\n")

	builder.WriteString("rule-providers:\n")
	providerList := mapKeys(providerDefs)
	sort.Strings(providerList)
	for _, name := range providerList {
		meta := providerDefs[name]
		builder.WriteString(fmt.Sprintf("  %s:\n", name))
		builder.WriteString(fmt.Sprintf("    type: %s\n", meta.Type))
		builder.WriteString(fmt.Sprintf("    behavior: %s\n", meta.Behavior))
		builder.WriteString(fmt.Sprintf("    format: %s\n", meta.Format))
		builder.WriteString(fmt.Sprintf("    url: %s\n", quote(meta.URL)))
		builder.WriteString(fmt.Sprintf("    path: %s\n", quote(meta.Path)))
		builder.WriteString("    interval: 86400\n")
	}
	builder.WriteString("\n")

	builder.WriteString("rules:\n")

	for _, rule := range directCIDRRules {
		builder.WriteString(fmt.Sprintf("  - %s\n", rule))
	}
	if req.BlockQUIC {
		builder.WriteString("  - AND,((DEST-PORT,443),(NETWORK,UDP)),REJECT\n")
	}
	for _, rule := range serviceRules {
		builder.WriteString(fmt.Sprintf("  - %s\n", rule))
	}
	for _, rule := range providerRules {
		builder.WriteString(fmt.Sprintf("  - %s\n", rule))
	}

	builder.WriteString("  - RULE-SET,private,DIRECT\n")
	builder.WriteString("  - RULE-SET,cn,DIRECT\n")
	builder.WriteString("  - GEOSITE,CN,DIRECT\n")
	builder.WriteString("  - GEOIP,CN,DIRECT,no-resolve\n")
	matchTarget := proxyGroup
	if mode == models.ModeWhitelist {
		matchTarget = "DIRECT"
	}
	builder.WriteString(fmt.Sprintf("  - MATCH,%s\n", matchTarget))

	return builder.String(), nil
}

func selectNodes(nodes []models.ProxyNode, selectedIDs []string) []models.ProxyNode {
	if len(selectedIDs) == 0 {
		return nodes
	}
	wanted := make(map[string]struct{}, len(selectedIDs))
	for _, id := range selectedIDs {
		wanted[id] = struct{}{}
	}
	out := make([]models.ProxyNode, 0, len(nodes))
	for _, node := range nodes {
		if _, ok := wanted[node.ID]; ok {
			out = append(out, node)
		}
	}
	return out
}

func writeProxy(builder *strings.Builder, node models.ProxyNode) {
	builder.WriteString(fmt.Sprintf("  - name: %s\n", quote(node.Name)))
	builder.WriteString(fmt.Sprintf("    type: %s\n", node.Protocol))
	builder.WriteString(fmt.Sprintf("    server: %s\n", quote(node.Server)))
	builder.WriteString(fmt.Sprintf("    port: %d\n", node.Port))

	switch node.Protocol {
	case models.ProtocolVLESS:
		builder.WriteString(fmt.Sprintf("    uuid: %s\n", quote(node.UUID)))
		if node.Flow != "" {
			builder.WriteString(fmt.Sprintf("    flow: %s\n", quote(node.Flow)))
		}
		if node.Network != "" {
			builder.WriteString(fmt.Sprintf("    network: %s\n", quote(node.Network)))
		}
		builder.WriteString(fmt.Sprintf("    tls: %t\n", node.TLS))
		if node.SNI != "" {
			builder.WriteString(fmt.Sprintf("    servername: %s\n", quote(node.SNI)))
		}
		if node.Fingerprint != "" {
			builder.WriteString(fmt.Sprintf("    client-fingerprint: %s\n", quote(node.Fingerprint)))
		}
		if node.Security == "reality" {
			builder.WriteString("    reality-opts:\n")
			if node.PublicKey != "" {
				builder.WriteString(fmt.Sprintf("      public-key: %s\n", quote(node.PublicKey)))
			}
			if node.ShortID != "" {
				builder.WriteString(fmt.Sprintf("      short-id: %s\n", quote(node.ShortID)))
			}
		}
		if node.Network == "grpc" && node.ServiceName != "" {
			builder.WriteString("    grpc-opts:\n")
			builder.WriteString(fmt.Sprintf("      grpc-service-name: %s\n", quote(node.ServiceName)))
		}
		if node.Network == "ws" {
			builder.WriteString("    ws-opts:\n")
			if node.Path != "" {
				builder.WriteString(fmt.Sprintf("      path: %s\n", quote(node.Path)))
			}
			if node.Host != "" {
				builder.WriteString("      headers:\n")
				builder.WriteString(fmt.Sprintf("        Host: %s\n", quote(node.Host)))
			}
		}
	case models.ProtocolTUIC:
		builder.WriteString(fmt.Sprintf("    uuid: %s\n", quote(node.UUID)))
		builder.WriteString(fmt.Sprintf("    token: %s\n", quote(node.Token)))
		if node.SNI != "" {
			builder.WriteString(fmt.Sprintf("    sni: %s\n", quote(node.SNI)))
		}
		if node.ALPN != "" {
			builder.WriteString("    alpn:\n")
			for _, a := range splitCSV(node.ALPN) {
				builder.WriteString(fmt.Sprintf("      - %s\n", quote(a)))
			}
		}
		if node.CongestionControl != "" {
			builder.WriteString(fmt.Sprintf("    congestion-controller: %s\n", quote(node.CongestionControl)))
		}
		if node.UDPRelayMode != "" {
			builder.WriteString(fmt.Sprintf("    udp-relay-mode: %s\n", quote(node.UDPRelayMode)))
		}
	case models.ProtocolHysteria:
		builder.WriteString(fmt.Sprintf("    password: %s\n", quote(node.Password)))
		if node.SNI != "" {
			builder.WriteString(fmt.Sprintf("    sni: %s\n", quote(node.SNI)))
		}
	case models.ProtocolSS:
		builder.WriteString(fmt.Sprintf("    cipher: %s\n", quote(node.Cipher)))
		builder.WriteString(fmt.Sprintf("    password: %s\n", quote(node.Password)))
		if node.Plugin != "" {
			builder.WriteString(fmt.Sprintf("    plugin: %s\n", quote(node.Plugin)))
		}
		if opts := parsePluginOpts(node.PluginOpts); len(opts) > 0 {
			builder.WriteString("    plugin-opts:\n")
			keys := mapKeys(opts)
			sort.Strings(keys)
			for _, key := range keys {
				builder.WriteString(fmt.Sprintf("      %s: %s\n", key, quote(opts[key])))
			}
		}
	case models.ProtocolTrojan:
		builder.WriteString(fmt.Sprintf("    password: %s\n", quote(node.Password)))
		if node.SNI != "" {
			builder.WriteString(fmt.Sprintf("    sni: %s\n", quote(node.SNI)))
		}
		if node.Network != "" {
			builder.WriteString(fmt.Sprintf("    network: %s\n", quote(node.Network)))
		}
	case models.ProtocolVMess:
		builder.WriteString(fmt.Sprintf("    uuid: %s\n", quote(node.UUID)))
		if node.Network != "" {
			builder.WriteString(fmt.Sprintf("    network: %s\n", quote(node.Network)))
		}
		builder.WriteString(fmt.Sprintf("    tls: %t\n", node.TLS))
		if node.Host != "" {
			builder.WriteString(fmt.Sprintf("    host: %s\n", quote(node.Host)))
		}
		if node.Path != "" {
			builder.WriteString(fmt.Sprintf("    path: %s\n", quote(node.Path)))
		}
	}

	if node.DialerProxy != "" {
		builder.WriteString(fmt.Sprintf("    dialer-proxy: %s\n", quote(node.DialerProxy)))
	}
}

func normalizeCIDR(value string) string {
	item := strings.TrimSpace(value)
	if item == "" {
		return ""
	}
	if strings.Contains(item, "/") {
		if _, _, err := net.ParseCIDR(item); err == nil {
			return item
		}
		return ""
	}
	ip := net.ParseIP(item)
	if ip == nil {
		return ""
	}
	if ip.To4() != nil {
		return item + "/32"
	}
	return item + "/128"
}

func splitCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

func buildServiceRules(item models.ServiceTree, policy string) []string {
	out := make([]string, 0, len(item.Domains)+len(item.Keywords)+len(item.IPCIDRs))
	seen := make(map[string]struct{})

	for _, domain := range item.Domains {
		domain = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(domain, "*."), "."))
		if domain == "" {
			continue
		}
		rule := fmt.Sprintf("DOMAIN-SUFFIX,%s,%s", domain, policy)
		if _, ok := seen[rule]; ok {
			continue
		}
		seen[rule] = struct{}{}
		out = append(out, rule)
	}

	for _, keyword := range item.Keywords {
		keyword = strings.TrimSpace(keyword)
		if keyword == "" {
			continue
		}
		rule := fmt.Sprintf("DOMAIN-KEYWORD,%s,%s", keyword, policy)
		if _, ok := seen[rule]; ok {
			continue
		}
		seen[rule] = struct{}{}
		out = append(out, rule)
	}

	for _, cidr := range item.IPCIDRs {
		if normalized := normalizeCIDR(cidr); normalized != "" {
			rule := fmt.Sprintf("IP-CIDR,%s,%s,no-resolve", normalized, policy)
			if _, ok := seen[rule]; ok {
				continue
			}
			seen[rule] = struct{}{}
			out = append(out, rule)
		}
	}

	return out
}

func parsePluginOpts(value string) map[string]string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	out := make(map[string]string)
	parts := strings.Split(value, ";")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(kv[0])
		val := strings.TrimSpace(kv[1])
		if key == "" || val == "" {
			continue
		}
		out[key] = val
	}
	return out
}

func mapKeys[T any](m map[string]T) []string {
	out := make([]string, 0, len(m))
	for key := range m {
		out = append(out, key)
	}
	return out
}

func resolveProviderMeta(item models.ServiceTree) (string, providerMeta, bool) {
	hint := strings.TrimSpace(item.Provider)
	if hint == "" {
		hint = strings.TrimSpace(item.ID)
	}
	hint = sanitizeProviderName(hint)
	if hint == "" {
		hint = "provider"
	}

	meta := providerMeta{
		Type:     "http",
		Behavior: "classical",
		Format:   "yaml",
	}
	if builtin, ok := builtinProviders[item.Provider]; ok {
		meta = builtin
	}
	if overrideURL := strings.TrimSpace(item.RuleURL); overrideURL != "" {
		meta.URL = overrideURL
	}
	if !isValidRuleURL(meta.URL) {
		return "", providerMeta{}, false
	}

	if strings.TrimSpace(meta.Type) == "" {
		meta.Type = "http"
	}
	if strings.TrimSpace(meta.Behavior) == "" {
		meta.Behavior = "classical"
	}
	if strings.TrimSpace(meta.Format) == "" {
		meta.Format = "yaml"
	}
	if strings.TrimSpace(meta.Path) == "" {
		meta.Path = fmt.Sprintf("./ruleset/%s.%s", hint, providerFileExt(meta.Format))
	}
	return hint, meta, true
}

func isValidRuleURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	parsed, err := neturl.Parse(raw)
	if err != nil {
		return false
	}
	if parsed.Host == "" {
		return false
	}
	scheme := strings.ToLower(parsed.Scheme)
	return scheme == "http" || scheme == "https"
}

func providerFileExt(format string) string {
	if strings.EqualFold(strings.TrimSpace(format), "text") {
		return "txt"
	}
	return "yaml"
}

func sanitizeProviderName(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	value = providerNameCleaner.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-.")
	return value
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return values
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func quote(value string) string {
	value = strings.ReplaceAll(value, "'", "''")
	return fmt.Sprintf("'%s'", value)
}

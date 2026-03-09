package engine

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
	"meta-link-pro/backend/models"
)

var (
	uuidRegex   = regexp.MustCompile(`^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[1-5][a-fA-F0-9]{3}-[89abAB][a-fA-F0-9]{3}-[a-fA-F0-9]{12}$`)
	linkRegex   = regexp.MustCompile(`(?i)(?:vless|tuic|hysteria2|hy2|ss|trojan|vmess|https?)://[^\s]+`)
	ss2022Regex = regexp.MustCompile(`(?i)^2022-[a-z0-9-]+$`)
)

const (
	subscriptionFetchTimeout       = 10 * time.Second
	maxSubscriptionBodyBytes       = 2 * 1024 * 1024
	maxSubscriptionFetchesPerInput = 3
	maxSubscriptionURLLength       = 2048
)

var subscriptionHTTPClient = &http.Client{
	Timeout: subscriptionFetchTimeout,
}

func ParseInput(raw string) models.ParseReport {
	report := models.ParseReport{}
	content := strings.TrimSpace(raw)
	if content == "" {
		return report
	}

	nodes, errs := parseContentBlock(content, true)
	report.Nodes = append(report.Nodes, nodes...)
	report.Errors = append(report.Errors, errs...)

	if len(report.Nodes) == 0 && len(report.Errors) == 0 {
		report.Errors = append(report.Errors, models.ParseIssue{
			Protocol: "INPUT",
			Field:    "content",
			Message:  "未检测到可解析的代理链接或 Clash YAML 节点",
		})
	}
	return report
}

func ParseLink(link string) (models.ProxyNode, *models.ParseIssue) {
	trimmed := strings.TrimSpace(link)
	trimmed = strings.Trim(trimmed, "\"'")

	u, err := url.Parse(trimmed)
	if err != nil || u.Scheme == "" {
		return models.ProxyNode{}, &models.ParseIssue{Protocol: "UNKNOWN", Field: "url", Message: "无法识别的链接格式"}
	}

	switch strings.ToLower(u.Scheme) {
	case "vless":
		return parseVLESS(trimmed, u)
	case "tuic":
		return parseTUIC(trimmed, u)
	case "hysteria2", "hy2":
		return parseHysteria2(trimmed, u)
	case "ss":
		return parseShadowsocks(trimmed, u)
	case "trojan":
		return parseTrojan(trimmed, u)
	case "vmess":
		return parseVMess(trimmed)
	default:
		return models.ProxyNode{}, &models.ParseIssue{Protocol: strings.ToUpper(u.Scheme), Field: "scheme", Message: "暂不支持该协议"}
	}
}

func parseVLESS(raw string, u *url.URL) (models.ProxyNode, *models.ParseIssue) {
	server, port, issue := parseServerPort(u, "VLESS")
	if issue != nil {
		return models.ProxyNode{}, issue
	}

	uuid := strings.TrimSpace(u.User.Username())
	if uuid == "" {
		return models.ProxyNode{}, &models.ParseIssue{Protocol: "VLESS", Field: "uuid", Message: "[VLESS] UUID缺失"}
	}
	if !uuidRegex.MatchString(uuid) {
		return models.ProxyNode{}, &models.ParseIssue{Protocol: "VLESS", Field: "uuid", Message: "[VLESS] UUID格式不合法"}
	}

	q := u.Query()
	security := strings.ToLower(q.Get("security"))
	pbk := coalesce(q.Get("pbk"), q.Get("publicKey"))

	if security == "reality" && pbk == "" {
		return models.ProxyNode{}, &models.ParseIssue{Protocol: "VLESS", Field: "pbk", Message: "[VLESS] Reality 公钥(pbk)缺失"}
	}

	network := strings.ToLower(coalesce(q.Get("type"), "tcp"))
	node := models.ProxyNode{
		ID:          genNodeID(raw),
		Name:        parseName(u, "VLESS Node"),
		Protocol:    models.ProtocolVLESS,
		Server:      server,
		Port:        port,
		UUID:        uuid,
		Network:     network,
		TLS:         security == "tls" || security == "reality",
		SNI:         coalesce(q.Get("sni"), q.Get("serverName"), q.Get("servername")),
		ALPN:        normalizeCSV(q.Get("alpn")),
		Flow:        q.Get("flow"),
		Security:    security,
		PublicKey:   pbk,
		ShortID:     coalesce(q.Get("sid"), q.Get("short-id"), q.Get("shortId")),
		Fingerprint: coalesce(q.Get("fp"), q.Get("fingerprint")),
		ServiceName: coalesce(q.Get("serviceName"), q.Get("service_name")),
		Host:        q.Get("host"),
		Path:        decodePath(q.Get("path")),
		RawLink:     raw,
	}

	return node, nil
}

func parseTUIC(raw string, u *url.URL) (models.ProxyNode, *models.ParseIssue) {
	server, port, issue := parseServerPort(u, "TUIC")
	if issue != nil {
		return models.ProxyNode{}, issue
	}

	q := u.Query()
	user := strings.TrimSpace(u.User.Username())
	password, _ := u.User.Password()

	uuid := coalesce(q.Get("uuid"), user)
	token := coalesce(q.Get("token"), q.Get("password"), password)

	if uuid == "" {
		return models.ProxyNode{}, &models.ParseIssue{Protocol: "TUIC", Field: "uuid", Message: "[TUIC] UUID缺失"}
	}
	if !uuidRegex.MatchString(uuid) {
		return models.ProxyNode{}, &models.ParseIssue{Protocol: "TUIC", Field: "uuid", Message: "[TUIC] UUID格式不合法"}
	}
	if token == "" {
		return models.ProxyNode{}, &models.ParseIssue{Protocol: "TUIC", Field: "token", Message: "[TUIC] Token缺失"}
	}

	congestion := strings.ToLower(coalesce(q.Get("congestion-control"), q.Get("congestion_control"), q.Get("congestion-controller"), "bbr"))
	if congestion != "bbr" && congestion != "cubic" {
		return models.ProxyNode{}, &models.ParseIssue{Protocol: "TUIC", Field: "congestion-control", Message: "[TUIC] congestion-control 仅支持 bbr/cubic"}
	}

	udpRelay := strings.ToLower(coalesce(q.Get("udp-relay-mode"), q.Get("udp_relay_mode"), q.Get("udp-relay"), "native"))
	if udpRelay != "native" && udpRelay != "quic" {
		return models.ProxyNode{}, &models.ParseIssue{Protocol: "TUIC", Field: "udp-relay-mode", Message: "[TUIC] udp-relay-mode 仅支持 native/quic"}
	}

	node := models.ProxyNode{
		ID:                genNodeID(raw),
		Name:              parseName(u, "TUIC Node"),
		Protocol:          models.ProtocolTUIC,
		Server:            server,
		Port:              port,
		UUID:              uuid,
		Token:             token,
		ALPN:              ensureALPNHasH3(normalizeCSV(q.Get("alpn"))),
		CongestionControl: congestion,
		UDPRelayMode:      udpRelay,
		SNI:               coalesce(q.Get("sni"), q.Get("serverName"), q.Get("servername")),
		RawLink:           raw,
	}

	return node, nil
}

func parseHysteria2(raw string, u *url.URL) (models.ProxyNode, *models.ParseIssue) {
	server, port, issue := parseServerPort(u, "HYSTERIA2")
	if issue != nil {
		return models.ProxyNode{}, issue
	}

	q := u.Query()
	password := coalesce(q.Get("password"), q.Get("auth"), q.Get("token"))
	if password == "" {
		password = strings.TrimSpace(u.User.Username())
	}
	if password == "" {
		return models.ProxyNode{}, &models.ParseIssue{Protocol: "HYSTERIA2", Field: "password", Message: "[Hysteria2] 密码缺失"}
	}

	node := models.ProxyNode{
		ID:       genNodeID(raw),
		Name:     parseName(u, "Hysteria2 Node"),
		Protocol: models.ProtocolHysteria,
		Server:   server,
		Port:     port,
		Password: password,
		SNI:      coalesce(q.Get("sni"), q.Get("peer"), q.Get("serverName")),
		ALPN:     normalizeCSV(q.Get("alpn")),
		RawLink:  raw,
	}
	return node, nil
}

func parseTrojan(raw string, u *url.URL) (models.ProxyNode, *models.ParseIssue) {
	server, port, issue := parseServerPort(u, "TROJAN")
	if issue != nil {
		return models.ProxyNode{}, issue
	}
	password := strings.TrimSpace(u.User.Username())
	if password == "" {
		return models.ProxyNode{}, &models.ParseIssue{Protocol: "TROJAN", Field: "password", Message: "[Trojan] 密码缺失"}
	}

	q := u.Query()
	network := strings.ToLower(coalesce(q.Get("type"), "tcp"))
	node := models.ProxyNode{
		ID:          genNodeID(raw),
		Name:        parseName(u, "Trojan Node"),
		Protocol:    models.ProtocolTrojan,
		Server:      server,
		Port:        port,
		Password:    password,
		SNI:         coalesce(q.Get("sni"), q.Get("peer"), q.Get("serverName")),
		ALPN:        normalizeCSV(q.Get("alpn")),
		Network:     network,
		Path:        decodePath(q.Get("path")),
		Host:        q.Get("host"),
		ServiceName: coalesce(q.Get("serviceName"), q.Get("service_name")),
		RawLink:     raw,
	}
	return node, nil
}

func parseShadowsocks(raw string, u *url.URL) (models.ProxyNode, *models.ParseIssue) {
	name := parseName(u, "SS Node")
	plugin := ""
	pluginOpts := ""
	if u != nil {
		pluginRaw := strings.TrimSpace(u.Query().Get("plugin"))
		plugin, pluginOpts = parseSSPlugin(pluginRaw)
	}

	cipher, password, server, port, err := parseSSCredentials(raw, u)
	if err != nil {
		return models.ProxyNode{}, &models.ParseIssue{Protocol: "SS", Field: "credential", Message: fmt.Sprintf("[SS] %v", err)}
	}

	if ss2022Regex.MatchString(strings.ToLower(cipher)) && password == "" {
		return models.ProxyNode{}, &models.ParseIssue{Protocol: "SS", Field: "password", Message: "[SS-2022] 密码缺失"}
	}

	node := models.ProxyNode{
		ID:         genNodeID(raw),
		Name:       name,
		Protocol:   models.ProtocolSS,
		Server:     server,
		Port:       port,
		Cipher:     cipher,
		Password:   password,
		RawLink:    raw,
		Plugin:     plugin,
		PluginOpts: pluginOpts,
	}
	return node, nil
}

func parseVMess(raw string) (models.ProxyNode, *models.ParseIssue) {
	encoded := strings.TrimPrefix(raw, "vmess://")
	decoded, err := tryDecodeBase64(encoded)
	if err != nil {
		return models.ProxyNode{}, &models.ParseIssue{Protocol: "VMESS", Field: "payload", Message: "[VMess] Base64 数据解码失败"}
	}

	payload := map[string]any{}
	if err := json.Unmarshal([]byte(decoded), &payload); err != nil {
		return models.ProxyNode{}, &models.ParseIssue{Protocol: "VMESS", Field: "payload", Message: "[VMess] JSON 数据解析失败"}
	}

	port, portErr := parseIntAny(firstAny(payload, "port", "p"))
	if portErr != nil {
		return models.ProxyNode{}, &models.ParseIssue{Protocol: "VMESS", Field: "port", Message: "[VMess] 端口必须是数字"}
	}

	server := asString(firstAny(payload, "add", "server", "host"))
	uuid := asString(firstAny(payload, "id", "uuid"))
	if server == "" {
		return models.ProxyNode{}, &models.ParseIssue{Protocol: "VMESS", Field: "add", Message: "[VMess] 服务器地址缺失"}
	}
	if uuid == "" {
		return models.ProxyNode{}, &models.ParseIssue{Protocol: "VMESS", Field: "id", Message: "[VMess] UUID缺失"}
	}

	node := models.ProxyNode{
		ID:          genNodeID(raw),
		Name:        coalesce(asString(payload["ps"]), "VMess Node"),
		Protocol:    models.ProtocolVMess,
		Server:      server,
		Port:        port,
		UUID:        uuid,
		Network:     strings.ToLower(coalesce(asString(payload["net"]), asString(payload["type"]), "tcp")),
		TLS:         strings.EqualFold(asString(firstAny(payload, "tls", "security")), "tls"),
		Host:        asString(firstAny(payload, "host", "ws-headers.host")),
		Path:        asString(firstAny(payload, "path", "ws-path")),
		SNI:         asString(firstAny(payload, "sni", "servername")),
		ALPN:        normalizeCSV(asString(payload["alpn"])),
		RawLink:     raw,
		Fingerprint: asString(firstAny(payload, "fp", "fingerprint")),
	}
	return node, nil
}

func parseSubscription(subscriptionURL string) ([]models.ProxyNode, []models.ParseIssue) {
	subscriptionURL = strings.TrimSpace(subscriptionURL)
	if len(subscriptionURL) > maxSubscriptionURLLength {
		return nil, []models.ParseIssue{{Protocol: "SUB", Field: "url", Message: fmt.Sprintf("[订阅] URL 过长(>%d)", maxSubscriptionURLLength)}}
	}

	ctx, cancel := context.WithTimeout(context.Background(), subscriptionFetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, subscriptionURL, nil)
	if err != nil {
		return nil, []models.ParseIssue{{Protocol: "SUB", Field: "url", Message: "[订阅] URL 无效"}}
	}
	req.Header.Set("User-Agent", "Meta-Link-Pro/1.0")

	resp, err := subscriptionHTTPClient.Do(req)
	if err != nil {
		return nil, []models.ParseIssue{{Protocol: "SUB", Field: "network", Message: fmt.Sprintf("[订阅] 拉取失败: %v", err)}}
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, []models.ParseIssue{{Protocol: "SUB", Field: "status", Message: fmt.Sprintf("[订阅] HTTP 状态异常: %d", resp.StatusCode)}}
	}
	if resp.ContentLength > maxSubscriptionBodyBytes {
		return nil, []models.ParseIssue{{Protocol: "SUB", Field: "body", Message: fmt.Sprintf("[订阅] 响应体超过限制(%dMB)", maxSubscriptionBodyBytes/(1024*1024))}}
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSubscriptionBodyBytes+1))
	if err != nil {
		return nil, []models.ParseIssue{{Protocol: "SUB", Field: "body", Message: "[订阅] 响应体读取失败"}}
	}
	if len(body) > maxSubscriptionBodyBytes {
		return nil, []models.ParseIssue{{Protocol: "SUB", Field: "body", Message: fmt.Sprintf("[订阅] 响应体超过限制(%dMB)", maxSubscriptionBodyBytes/(1024*1024))}}
	}

	content := strings.TrimSpace(string(body))
	if decoded, decErr := tryDecodeBase64(content); decErr == nil && likelyStructuredSubscription(decoded) {
		content = decoded
	}

	nodes, errs := parseContentBlock(content, false)
	if len(nodes) == 0 {
		errs = append(errs, models.ParseIssue{Protocol: "SUB", Field: "content", Message: "[订阅] 未解析到有效节点"})
	}
	return nodes, errs
}

func parseContentBlock(content string, allowSubscriptionFetch bool) ([]models.ProxyNode, []models.ParseIssue) {
	nodes := make([]models.ProxyNode, 0)
	errs := make([]models.ParseIssue, 0)
	content = strings.TrimSpace(content)
	if content == "" {
		return nodes, errs
	}

	if likelyClashYAML(content) {
		yamlNodes, yamlErrs := parseClashYAMLProxies(content)
		nodes = append(nodes, yamlNodes...)
		errs = append(errs, yamlErrs...)
		if len(yamlNodes) > 0 {
			return dedupeNodes(nodes), errs
		}
	}

	links := extractCandidateLinks(content)
	if len(links) == 0 {
		if decoded, err := tryDecodeBase64(content); err == nil {
			decoded = strings.TrimSpace(decoded)
			if decoded != "" && decoded != content {
				decodedNodes, decodedErrs := parseContentBlock(decoded, false)
				nodes = append(nodes, decodedNodes...)
				errs = append(errs, decodedErrs...)
			}
		}
		return dedupeNodes(nodes), errs
	}

	hasProxyScheme := containsProxySchemeMarker(content)
	subscriptionFetchCount := 0
	for _, link := range links {
		if allowSubscriptionFetch && shouldFetchSubscription(link, len(links), hasProxyScheme) {
			if subscriptionFetchCount >= maxSubscriptionFetchesPerInput {
				errs = append(errs, models.ParseIssue{
					Protocol: "SUB",
					Field:    "count",
					Message:  fmt.Sprintf("[订阅] 最多处理 %d 个订阅链接，其余已跳过", maxSubscriptionFetchesPerInput),
				})
				continue
			}
			subscriptionFetchCount++
			subNodes, subErrs := parseSubscription(link)
			nodes = append(nodes, subNodes...)
			errs = append(errs, subErrs...)
			continue
		}
		if isHTTPURL(link) {
			continue
		}

		node, issue := ParseLink(link)
		if issue != nil {
			errs = append(errs, *issue)
			continue
		}
		nodes = append(nodes, node)
	}

	return dedupeNodes(nodes), errs
}

func parseClashYAMLProxies(content string) ([]models.ProxyNode, []models.ParseIssue) {
	decoder := yaml.NewDecoder(strings.NewReader(content))
	issues := make([]models.ParseIssue, 0)
	nodes := make([]models.ProxyNode, 0)

	for {
		var doc yaml.Node
		err := decoder.Decode(&doc)
		if err == io.EOF {
			break
		}
		if err != nil {
			issues = append(issues, models.ParseIssue{
				Protocol: "CLASH",
				Field:    "yaml",
				Message:  fmt.Sprintf("[Clash YAML] 解析失败: %v", err),
			})
			break
		}
		if len(doc.Content) == 0 {
			continue
		}

		proxiesNode := findYAMLMapValueNode(doc.Content[0], "proxies")
		if proxiesNode == nil {
			continue
		}
		if proxiesNode.Kind == yaml.AliasNode && proxiesNode.Alias != nil {
			proxiesNode = proxiesNode.Alias
		}
		if proxiesNode.Kind != yaml.SequenceNode {
			issues = append(issues, models.ParseIssue{
				Protocol: "CLASH",
				Field:    "proxies",
				Message:  "[Clash YAML] proxies 字段必须是列表",
			})
			continue
		}

		for _, item := range proxiesNode.Content {
			anyMap, convErr := yamlNodeToMap(item)
			if convErr != nil {
				issues = append(issues, models.ParseIssue{
					Protocol: "CLASH",
					Field:    "proxies",
					Message:  fmt.Sprintf("[Clash YAML] 节点转换失败: %v", convErr),
				})
				continue
			}

			flat := make(map[string]string)
			flattenYAMLMap("", anyMap, flat)
			node, issue := parseClashEntry(flat)
			if issue != nil {
				issues = append(issues, *issue)
				continue
			}
			nodes = append(nodes, node)
		}
	}

	return nodes, issues
}

func parseClashEntry(entry map[string]string) (models.ProxyNode, *models.ParseIssue) {
	proxyType := strings.ToLower(entry["type"])
	if proxyType == "" {
		return models.ProxyNode{}, &models.ParseIssue{Protocol: "CLASH", Field: "type", Message: "[Clash YAML] 节点 type 缺失"}
	}

	name := coalesce(entry["name"], strings.ToUpper(proxyType)+" Node")
	server := coalesce(entry["server"], entry["add"], entry["host"])
	if server == "" {
		return models.ProxyNode{}, &models.ParseIssue{Protocol: strings.ToUpper(proxyType), Field: "server", Message: fmt.Sprintf("[%s] 服务器地址缺失", strings.ToUpper(proxyType))}
	}

	port, err := strconv.Atoi(strings.TrimSpace(entry["port"]))
	if err != nil || port <= 0 {
		return models.ProxyNode{}, &models.ParseIssue{Protocol: strings.ToUpper(proxyType), Field: "port", Message: fmt.Sprintf("[%s] 端口必须是数字", strings.ToUpper(proxyType))}
	}

	raw := fmt.Sprintf("clash://%s@%s:%d#%s", proxyType, server, port, name)
	baseNode := models.ProxyNode{
		ID:          genNodeID(raw),
		Name:        name,
		Server:      server,
		Port:        port,
		RawLink:     raw,
		Network:     strings.ToLower(coalesce(entry["network"], entry["net"], "tcp")),
		SNI:         coalesce(entry["sni"], entry["servername"]),
		ALPN:        normalizeCSV(entry["alpn"]),
		Host:        coalesce(entry["host"], entry["ws-opts.headers.Host"], entry["ws-opts.headers.host"]),
		Path:        coalesce(entry["path"], entry["ws-opts.path"]),
		DialerProxy: entry["dialer-proxy"],
	}

	switch proxyType {
	case "vless":
		uuid := coalesce(entry["uuid"], entry["id"])
		if uuid == "" {
			return models.ProxyNode{}, &models.ParseIssue{Protocol: "VLESS", Field: "uuid", Message: "[VLESS] UUID缺失"}
		}
		baseNode.Protocol = models.ProtocolVLESS
		baseNode.UUID = uuid
		baseNode.TLS = parseBoolLoose(entry["tls"])
		baseNode.Flow = entry["flow"]
		baseNode.Security = entry["security"]
		baseNode.PublicKey = coalesce(entry["reality-opts.public-key"], entry["public-key"])
		baseNode.ShortID = coalesce(entry["reality-opts.short-id"], entry["short-id"])
		baseNode.ServiceName = coalesce(entry["grpc-opts.grpc-service-name"], entry["serviceName"])
		return baseNode, nil
	case "tuic":
		uuid := entry["uuid"]
		token := coalesce(entry["token"], entry["password"])
		if uuid == "" {
			return models.ProxyNode{}, &models.ParseIssue{Protocol: "TUIC", Field: "uuid", Message: "[TUIC] UUID缺失"}
		}
		if token == "" {
			return models.ProxyNode{}, &models.ParseIssue{Protocol: "TUIC", Field: "token", Message: "[TUIC] Token缺失"}
		}
		baseNode.Protocol = models.ProtocolTUIC
		baseNode.UUID = uuid
		baseNode.Token = token
		baseNode.ALPN = ensureALPNHasH3(baseNode.ALPN)
		baseNode.CongestionControl = strings.ToLower(coalesce(entry["congestion-controller"], entry["congestion-control"], "bbr"))
		baseNode.UDPRelayMode = strings.ToLower(coalesce(entry["udp-relay-mode"], "native"))
		return baseNode, nil
	case "hysteria2":
		password := coalesce(entry["password"], entry["auth"], entry["token"])
		if password == "" {
			return models.ProxyNode{}, &models.ParseIssue{Protocol: "HYSTERIA2", Field: "password", Message: "[Hysteria2] 密码缺失"}
		}
		baseNode.Protocol = models.ProtocolHysteria
		baseNode.Password = password
		return baseNode, nil
	case "ss":
		cipher := coalesce(entry["cipher"], entry["method"])
		password := entry["password"]
		if cipher == "" {
			return models.ProxyNode{}, &models.ParseIssue{Protocol: "SS", Field: "cipher", Message: "[SS] cipher缺失"}
		}
		if password == "" {
			return models.ProxyNode{}, &models.ParseIssue{Protocol: "SS", Field: "password", Message: "[SS] password缺失"}
		}
		baseNode.Protocol = models.ProtocolSS
		baseNode.Cipher = cipher
		baseNode.Password = password
		baseNode.Plugin = entry["plugin"]
		return baseNode, nil
	case "trojan":
		password := entry["password"]
		if password == "" {
			return models.ProxyNode{}, &models.ParseIssue{Protocol: "TROJAN", Field: "password", Message: "[Trojan] 密码缺失"}
		}
		baseNode.Protocol = models.ProtocolTrojan
		baseNode.Password = password
		baseNode.ServiceName = entry["grpc-opts.grpc-service-name"]
		return baseNode, nil
	case "vmess":
		uuid := coalesce(entry["uuid"], entry["id"])
		if uuid == "" {
			return models.ProxyNode{}, &models.ParseIssue{Protocol: "VMESS", Field: "uuid", Message: "[VMess] UUID缺失"}
		}
		baseNode.Protocol = models.ProtocolVMess
		baseNode.UUID = uuid
		baseNode.TLS = parseBoolLoose(entry["tls"])
		baseNode.Fingerprint = coalesce(entry["client-fingerprint"], entry["fingerprint"])
		return baseNode, nil
	default:
		return models.ProxyNode{}, &models.ParseIssue{Protocol: strings.ToUpper(proxyType), Field: "type", Message: "[Clash YAML] 暂不支持该节点类型"}
	}
}

func parseSSCredentials(raw string, u *url.URL) (cipher string, password string, server string, port int, err error) {
	if u != nil {
		host := u.Hostname()
		portStr := u.Port()
		if host != "" && portStr != "" {
			portNum, convErr := strconv.Atoi(portStr)
			if convErr != nil {
				return "", "", "", 0, fmt.Errorf("端口必须是数字")
			}

			username := u.User.Username()
			pass, hasPass := u.User.Password()
			if hasPass {
				if username == "" {
					return "", "", "", 0, fmt.Errorf("加密算法缺失")
				}
				return username, pass, host, portNum, nil
			}

			if username != "" {
				decoded, decErr := tryDecodeBase64(username)
				if decErr == nil {
					parts := strings.SplitN(decoded, ":", 2)
					if len(parts) == 2 {
						return parts[0], parts[1], host, portNum, nil
					}
				}

				if strings.Contains(username, ":") {
					parts := strings.SplitN(username, ":", 2)
					return parts[0], parts[1], host, portNum, nil
				}
			}
		}
	}

	base := strings.TrimPrefix(raw, "ss://")
	base = strings.Split(base, "#")[0]
	base = strings.Split(base, "?")[0]
	base = strings.TrimSpace(base)
	base = strings.Trim(base, "/")

	decodedWhole, decErr := tryDecodeBase64(base)
	if decErr == nil && strings.Contains(decodedWhole, "@") {
		base = decodedWhole
	}

	parts := strings.SplitN(base, "@", 2)
	if len(parts) != 2 {
		return "", "", "", 0, fmt.Errorf("用户信息解析失败")
	}

	cred := parts[0]
	serverPart := parts[1]
	if !strings.Contains(cred, ":") {
		if decoded, decodeErr := tryDecodeBase64(cred); decodeErr == nil {
			cred = decoded
		}
	}

	credential := strings.SplitN(cred, ":", 2)
	if len(credential) != 2 {
		return "", "", "", 0, fmt.Errorf("加密信息格式错误")
	}

	host, portText, splitErr := net.SplitHostPort(serverPart)
	if splitErr != nil {
		return "", "", "", 0, fmt.Errorf("服务器地址或端口格式错误")
	}
	portNum, convErr := strconv.Atoi(portText)
	if convErr != nil {
		return "", "", "", 0, fmt.Errorf("端口必须是数字")
	}

	return credential[0], credential[1], host, portNum, nil
}

func parseSSPlugin(raw string) (string, string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ""
	}
	decoded, err := url.QueryUnescape(raw)
	if err != nil {
		decoded = raw
	}
	parts := strings.Split(decoded, ";")
	if len(parts) == 0 {
		return "", ""
	}
	plugin := strings.TrimSpace(parts[0])
	if len(parts) == 1 {
		return plugin, ""
	}
	return plugin, strings.Join(parts[1:], ";")
}

func parseServerPort(u *url.URL, protocol string) (string, int, *models.ParseIssue) {
	host := u.Hostname()
	if host == "" {
		return "", 0, &models.ParseIssue{Protocol: protocol, Field: "server", Message: fmt.Sprintf("[%s] 服务器地址缺失", protocol)}
	}
	portStr := u.Port()
	if portStr == "" {
		return "", 0, &models.ParseIssue{Protocol: protocol, Field: "port", Message: fmt.Sprintf("[%s] 端口缺失", protocol)}
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", 0, &models.ParseIssue{Protocol: protocol, Field: "port", Message: fmt.Sprintf("[%s] 端口必须是数字", protocol)}
	}
	return host, port, nil
}

func parseName(u *url.URL, fallback string) string {
	if u == nil || u.Fragment == "" {
		return fallback
	}
	name, err := url.QueryUnescape(u.Fragment)
	if err != nil || strings.TrimSpace(name) == "" {
		return fallback
	}
	return name
}

func extractCandidateLinks(raw string) []string {
	matches := linkRegex.FindAllString(raw, -1)
	if len(matches) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(matches))
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		clean := cleanExtractedLink(match)
		if clean == "" {
			continue
		}
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
	}
	return out
}

func cleanExtractedLink(link string) string {
	link = strings.TrimSpace(link)
	link = strings.Trim(link, "\"'`")
	link = strings.TrimRight(link, ",;>")
	link = strings.TrimPrefix(link, "(")
	if strings.HasSuffix(link, ")") && !strings.Contains(link, "(") {
		link = strings.TrimSuffix(link, ")")
	}
	return link
}

func isSubscriptionLink(link string) bool {
	lower := strings.ToLower(link)
	if !isHTTPURL(link) {
		return false
	}
	if strings.Contains(lower, ".yaml") || strings.Contains(lower, ".yml") || strings.Contains(lower, "clash") {
		return true
	}
	return strings.Contains(lower, "subscribe") || strings.Contains(lower, "subscription") || strings.Contains(lower, "sub=") || strings.Contains(lower, "token=")
}

func isHTTPURL(link string) bool {
	lower := strings.ToLower(strings.TrimSpace(link))
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

func containsProxySchemeMarker(content string) bool {
	lower := strings.ToLower(content)
	return strings.Contains(lower, "vless://") ||
		strings.Contains(lower, "vmess://") ||
		strings.Contains(lower, "trojan://") ||
		strings.Contains(lower, "ss://") ||
		strings.Contains(lower, "tuic://") ||
		strings.Contains(lower, "hysteria2://") ||
		strings.Contains(lower, "hy2://")
}

func shouldFetchSubscription(link string, totalLinks int, hasProxyScheme bool) bool {
	if !isHTTPURL(link) {
		return false
	}
	if isSubscriptionLink(link) {
		return true
	}
	// Plain-text subscription links often have no "subscribe/token" keywords.
	return totalLinks == 1 && !hasProxyScheme
}

func tryDecodeBase64(data string) (string, error) {
	data = strings.TrimSpace(data)
	if data == "" {
		return "", fmt.Errorf("empty")
	}

	variants := []string{data, strings.ReplaceAll(strings.ReplaceAll(data, "-", "+"), "_", "/")}
	for _, variant := range variants {
		candidate := strings.TrimSpace(variant)
		if candidate == "" {
			continue
		}

		withPadding := candidate
		if mod := len(withPadding) % 4; mod != 0 {
			withPadding += strings.Repeat("=", 4-mod)
		}

		encodings := []*base64.Encoding{
			base64.StdEncoding,
			base64.RawStdEncoding,
			base64.URLEncoding,
			base64.RawURLEncoding,
		}
		for _, enc := range encodings {
			if decoded, err := enc.DecodeString(withPadding); err == nil {
				return string(decoded), nil
			}
			if decoded, err := enc.DecodeString(candidate); err == nil {
				return string(decoded), nil
			}
		}
	}
	return "", fmt.Errorf("decode failed")
}

func likelyClashYAML(content string) bool {
	lower := strings.ToLower(content)
	if strings.Contains(lower, "proxies:") && strings.Contains(lower, "- name:") {
		return true
	}
	return strings.Contains(lower, "proxies:") && strings.Contains(lower, "type:") && strings.Contains(lower, "server:")
}

func likelyStructuredSubscription(content string) bool {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return false
	}
	if strings.Contains(trimmed, "://") {
		return true
	}
	return likelyClashYAML(trimmed)
}

func parseInlineYAMLMap(payload string) map[string]string {
	payload = strings.TrimSpace(payload)
	payload = strings.TrimPrefix(payload, "{")
	payload = strings.TrimSuffix(payload, "}")
	chunks := splitCommaAware(payload)
	result := make(map[string]string, len(chunks))
	for _, chunk := range chunks {
		if key, value, ok := splitYAMLKeyValue(chunk); ok {
			result[key] = value
		}
	}
	return result
}

func splitCommaAware(raw string) []string {
	parts := make([]string, 0)
	buf := &strings.Builder{}
	quote := rune(0)
	for _, r := range raw {
		switch r {
		case '\'', '"':
			if quote == 0 {
				quote = r
			} else if quote == r {
				quote = 0
			}
			buf.WriteRune(r)
		case ',':
			if quote == 0 {
				parts = append(parts, strings.TrimSpace(buf.String()))
				buf.Reset()
				continue
			}
			buf.WriteRune(r)
		default:
			buf.WriteRune(r)
		}
	}
	if tail := strings.TrimSpace(buf.String()); tail != "" {
		parts = append(parts, tail)
	}
	return parts
}

func splitYAMLKeyValue(line string) (string, string, bool) {
	idx := strings.Index(line, ":")
	if idx <= 0 {
		return "", "", false
	}
	key := strings.TrimSpace(line[:idx])
	value := strings.TrimSpace(line[idx+1:])
	key = strings.Trim(key, "\"'")
	value = strings.Trim(value, "\"'")
	if key == "" {
		return "", "", false
	}
	return key, value, true
}

func parseBoolLoose(value string) bool {
	v := strings.ToLower(strings.TrimSpace(value))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func findYAMLMapValueNode(node *yaml.Node, key string) *yaml.Node {
	if node == nil {
		return nil
	}
	if node.Kind == yaml.DocumentNode {
		if len(node.Content) == 0 {
			return nil
		}
		return findYAMLMapValueNode(node.Content[0], key)
	}
	if node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		k := node.Content[i]
		v := node.Content[i+1]
		if strings.EqualFold(k.Value, key) {
			return v
		}
	}
	return nil
}

func yamlNodeToMap(node *yaml.Node) (map[string]any, error) {
	resolved, err := yamlNodeToAny(node, map[*yaml.Node]bool{})
	if err != nil {
		return nil, err
	}
	out, ok := resolved.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("节点不是对象")
	}
	return out, nil
}

func yamlNodeToAny(node *yaml.Node, visiting map[*yaml.Node]bool) (any, error) {
	if node == nil {
		return nil, nil
	}
	if visiting[node] {
		return nil, fmt.Errorf("检测到循环 alias")
	}

	switch node.Kind {
	case yaml.AliasNode:
		if node.Alias == nil {
			return nil, fmt.Errorf("alias 目标为空")
		}
		visiting[node] = true
		defer delete(visiting, node)
		return yamlNodeToAny(node.Alias, visiting)
	case yaml.DocumentNode:
		if len(node.Content) == 0 {
			return nil, nil
		}
		visiting[node] = true
		defer delete(visiting, node)
		return yamlNodeToAny(node.Content[0], visiting)
	case yaml.MappingNode:
		visiting[node] = true
		defer delete(visiting, node)

		out := make(map[string]any)
		for i := 0; i+1 < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valNode := node.Content[i+1]
			key := strings.TrimSpace(keyNode.Value)
			if key == "" {
				continue
			}

			if key == "<<" {
				merged, err := yamlNodeToAny(valNode, visiting)
				if err != nil {
					return nil, err
				}
				mergeMapAny(out, merged)
				continue
			}

			val, err := yamlNodeToAny(valNode, visiting)
			if err != nil {
				return nil, err
			}
			out[key] = val
		}
		return out, nil
	case yaml.SequenceNode:
		visiting[node] = true
		defer delete(visiting, node)

		out := make([]any, 0, len(node.Content))
		for _, child := range node.Content {
			val, err := yamlNodeToAny(child, visiting)
			if err != nil {
				return nil, err
			}
			out = append(out, val)
		}
		return out, nil
	case yaml.ScalarNode:
		return yamlScalar(node), nil
	default:
		return nil, nil
	}
}

func mergeMapAny(dst map[string]any, source any) {
	switch s := source.(type) {
	case map[string]any:
		for key, val := range s {
			dst[key] = val
		}
	case []any:
		for _, item := range s {
			mergeMapAny(dst, item)
		}
	}
}

func yamlScalar(node *yaml.Node) any {
	switch node.Tag {
	case "!!null":
		return nil
	case "!!bool":
		v, err := strconv.ParseBool(strings.ToLower(strings.TrimSpace(node.Value)))
		if err == nil {
			return v
		}
	case "!!int":
		v, err := strconv.ParseInt(strings.TrimSpace(node.Value), 10, 64)
		if err == nil {
			return v
		}
	case "!!float":
		v, err := strconv.ParseFloat(strings.TrimSpace(node.Value), 64)
		if err == nil {
			return v
		}
	}
	return strings.TrimSpace(node.Value)
}

func flattenYAMLMap(prefix string, src map[string]any, out map[string]string) {
	for key, val := range src {
		fullKey := key
		if prefix != "" {
			fullKey = prefix + "." + key
		}
		switch v := val.(type) {
		case map[string]any:
			flattenYAMLMap(fullKey, v, out)
		case []any:
			out[fullKey] = joinAnySlice(v)
		default:
			text := asString(v)
			if text != "" {
				out[fullKey] = text
			}
		}
	}
}

func joinAnySlice(values []any) string {
	if len(values) == 0 {
		return ""
	}
	parts := make([]string, 0, len(values))
	for _, item := range values {
		text := asString(item)
		if text == "" {
			continue
		}
		parts = append(parts, text)
	}
	return strings.Join(parts, ",")
}

func normalizeCSV(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	parts := strings.Split(value, ",")
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(strings.Trim(part, "\"'"))
		if part == "" {
			continue
		}
		cleaned = append(cleaned, part)
	}
	return strings.Join(cleaned, ",")
}

func ensureALPNHasH3(value string) string {
	normalized := normalizeCSV(value)
	if normalized == "" {
		return "h3"
	}

	parts := strings.Split(normalized, ",")
	seen := make(map[string]struct{}, len(parts))
	out := make([]string, 0, len(parts)+1)
	hasH3 := false
	for _, part := range parts {
		part = strings.ToLower(strings.TrimSpace(strings.Trim(part, "\"'")))
		if part == "" {
			continue
		}
		if _, ok := seen[part]; ok {
			continue
		}
		seen[part] = struct{}{}
		if part == "h3" {
			hasH3 = true
		}
		out = append(out, part)
	}
	if !hasH3 {
		out = append(out, "h3")
	}
	return strings.Join(out, ",")
}

func decodePath(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	decoded, err := url.QueryUnescape(path)
	if err != nil {
		return path
	}
	return decoded
}

func asString(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	case float64:
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10)
		}
		return strings.TrimSpace(strconv.FormatFloat(v, 'f', -1, 64))
	case bool:
		if v {
			return "true"
		}
		return "false"
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", v))
	}
}

func firstAny(m map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := m[key]; ok {
			return value
		}
	}
	return nil
}

func parseIntAny(value any) (int, error) {
	text := asString(value)
	if text == "" {
		return 0, fmt.Errorf("empty")
	}
	return strconv.Atoi(text)
}

func leadingSpaces(line string) int {
	count := 0
	for _, r := range line {
		if r != ' ' {
			break
		}
		count++
	}
	return count
}

func dedupeNodes(nodes []models.ProxyNode) []models.ProxyNode {
	if len(nodes) == 0 {
		return nodes
	}
	seen := make(map[string]struct{}, len(nodes))
	out := make([]models.ProxyNode, 0, len(nodes))
	for _, node := range nodes {
		key := node.ID
		if key == "" {
			key = fmt.Sprintf("%s|%s|%d|%s", node.Protocol, node.Server, node.Port, node.Name)
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, node)
	}
	return out
}

func genNodeID(raw string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(raw))
	return fmt.Sprintf("node-%x", h.Sum64())
}

func coalesce(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

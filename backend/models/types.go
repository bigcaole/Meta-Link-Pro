package models

type ParseMode string

const (
	ModeWhitelist ParseMode = "whitelist"
	ModeBlacklist ParseMode = "blacklist"
)

type Protocol string

const (
	ProtocolVLESS    Protocol = "vless"
	ProtocolTUIC     Protocol = "tuic"
	ProtocolHysteria Protocol = "hysteria2"
	ProtocolSS       Protocol = "ss"
	ProtocolTrojan   Protocol = "trojan"
	ProtocolVMess    Protocol = "vmess"
)

type ParseIssue struct {
	Protocol string `json:"protocol"`
	Field    string `json:"field"`
	Message  string `json:"message"`
}

type ProxyNode struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Protocol Protocol `json:"protocol"`

	Server string `json:"server"`
	Port   int    `json:"port"`

	UUID  string `json:"uuid,omitempty"`
	Token string `json:"token,omitempty"`

	Password   string `json:"password,omitempty"`
	Cipher     string `json:"cipher,omitempty"`
	Plugin     string `json:"plugin,omitempty"`
	PluginOpts string `json:"pluginOpts,omitempty"`

	Network  string `json:"network,omitempty"`
	TLS      bool   `json:"tls,omitempty"`
	SNI      string `json:"sni,omitempty"`
	ALPN     string `json:"alpn,omitempty"`
	Flow     string `json:"flow,omitempty"`
	Security string `json:"security,omitempty"`

	PublicKey   string `json:"publicKey,omitempty"`
	ShortID     string `json:"shortId,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	ServiceName string `json:"serviceName,omitempty"`
	Host        string `json:"host,omitempty"`
	Path        string `json:"path,omitempty"`

	CongestionControl string `json:"congestionControl,omitempty"`
	UDPRelayMode      string `json:"udpRelayMode,omitempty"`

	DialerProxy string `json:"dialerProxy,omitempty"`
	RawLink     string `json:"rawLink"`

	Issues []ParseIssue `json:"issues,omitempty"`
}

type ServiceTree struct {
	ID         string        `json:"id"`
	Name       string        `json:"name"`
	Kind       string        `json:"kind"`
	Provider   string        `json:"provider,omitempty"`
	RuleURL    string        `json:"ruleUrl,omitempty"`
	RuleType   string        `json:"ruleType,omitempty"`
	Domains    []string      `json:"domains,omitempty"`
	Keywords   []string      `json:"keywords,omitempty"`
	IPCIDRs    []string      `json:"ipCidrs,omitempty"`
	DefaultOut string        `json:"defaultOut,omitempty"`
	Children   []ServiceTree `json:"children,omitempty"`
}

type ParseReport struct {
	Nodes  []ProxyNode  `json:"nodes"`
	Errors []ParseIssue `json:"errors"`
}

type UpdateStep struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

type UpdateStatus struct {
	Running    bool         `json:"running"`
	Completed  bool         `json:"completed"`
	Progress   int          `json:"progress"`
	Message    string       `json:"message"`
	StartedAt  string       `json:"startedAt,omitempty"`
	FinishedAt string       `json:"finishedAt,omitempty"`
	Steps      []UpdateStep `json:"steps"`
}

type ServiceSelection struct {
	ServiceID string `json:"serviceId"`
	Policy    string `json:"policy"`
	Enabled   bool   `json:"enabled"`
}

type GenerateMetaYAMLRequest struct {
	Nodes            []ProxyNode        `json:"nodes"`
	SelectedNodeIDs  []string           `json:"selectedNodeIds"`
	DirectCIDRs      []string           `json:"directCidrs"`
	Selections       []ServiceSelection `json:"selections"`
	Mode             ParseMode          `json:"mode"`
	BlockQUIC        bool               `json:"blockQuic"`
	ProxyGroupName   string             `json:"proxyGroupName"`
	ServicesSnapshot []ServiceTree      `json:"servicesSnapshot"`
}

package engine

import (
	"strings"
	"testing"

	"meta-link-pro/backend/models"
)

func TestGenerateMetaYAMLRuleOrderBlacklist(t *testing.T) {
	req := models.GenerateMetaYAMLRequest{
		Nodes: []models.ProxyNode{
			{
				ID:       "n1",
				Name:     "A",
				Protocol: models.ProtocolVLESS,
				Server:   "example.com",
				Port:     443,
				UUID:     "123e4567-e89b-12d3-a456-426614174000",
				TLS:      true,
			},
		},
		SelectedNodeIDs: []string{"n1"},
		DirectCIDRs:     []string{"192.168.1.100"},
		Selections: []models.ServiceSelection{
			{ServiceID: "svc-openai", Enabled: true, Policy: "Proxy_Group"},
		},
		Mode:           models.ModeBlacklist,
		ProxyGroupName: "Proxy_Group",
		ServicesSnapshot: []models.ServiceTree{
			{ID: "category-ai", Kind: "category", Children: []models.ServiceTree{{ID: "svc-openai", Kind: "service", Provider: "openai"}}},
		},
	}

	yaml, err := GenerateMetaYAML(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	idxCIDR := strings.Index(yaml, "SRC-IP-CIDR,192.168.1.100/32,DIRECT")
	idxSvc := strings.Index(yaml, "RULE-SET,openai,Proxy_Group")
	idxGlobal := strings.Index(yaml, "RULE-SET,private,DIRECT")
	idxGeoSiteCN := strings.Index(yaml, "GEOSITE,CN,DIRECT")
	idxCN := strings.Index(yaml, "GEOIP,CN,DIRECT,no-resolve")
	idxMatch := strings.Index(yaml, "MATCH,Proxy_Group")

	if !(idxCIDR < idxSvc && idxSvc < idxGlobal && idxGlobal < idxGeoSiteCN && idxGeoSiteCN < idxCN && idxCN < idxMatch) {
		t.Fatalf("rule order invalid:\n%s", yaml)
	}
}

func TestGenerateMetaYAMLBlacklistUsesProxyFallback(t *testing.T) {
	req := models.GenerateMetaYAMLRequest{
		Nodes: []models.ProxyNode{
			{
				ID:       "n1",
				Name:     "A",
				Protocol: models.ProtocolVLESS,
				Server:   "example.com",
				Port:     443,
				UUID:     "123e4567-e89b-12d3-a456-426614174000",
				TLS:      true,
			},
		},
		SelectedNodeIDs: []string{"n1"},
		Mode:            models.ModeBlacklist,
		ProxyGroupName:  "Proxy_Group",
	}

	yaml, err := GenerateMetaYAML(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(yaml, "MATCH,Proxy_Group") {
		t.Fatalf("expected proxy fallback in blacklist mode:\n%s", yaml)
	}
}

func TestGenerateMetaYAMLWhitelistUsesDirectFallback(t *testing.T) {
	req := models.GenerateMetaYAMLRequest{
		Nodes: []models.ProxyNode{
			{
				ID:       "n1",
				Name:     "A",
				Protocol: models.ProtocolVLESS,
				Server:   "example.com",
				Port:     443,
				UUID:     "123e4567-e89b-12d3-a456-426614174000",
				TLS:      true,
			},
		},
		SelectedNodeIDs: []string{"n1"},
		Mode:            models.ModeWhitelist,
		ProxyGroupName:  "Proxy_Group",
	}

	yaml, err := GenerateMetaYAML(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(yaml, "MATCH,DIRECT") {
		t.Fatalf("expected direct fallback in whitelist mode:\n%s", yaml)
	}
}

func TestGenerateMetaYAMLBlockQUICRule(t *testing.T) {
	req := models.GenerateMetaYAMLRequest{
		Nodes: []models.ProxyNode{
			{
				ID:       "n1",
				Name:     "A",
				Protocol: models.ProtocolVLESS,
				Server:   "example.com",
				Port:     443,
				UUID:     "123e4567-e89b-12d3-a456-426614174000",
				TLS:      true,
			},
		},
		SelectedNodeIDs: []string{"n1"},
		DirectCIDRs:     []string{"10.0.0.1"},
		Mode:            models.ModeBlacklist,
		BlockQUIC:       true,
		ProxyGroupName:  "Proxy_Group",
	}

	yaml, err := GenerateMetaYAML(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	idxCIDR := strings.Index(yaml, "SRC-IP-CIDR,10.0.0.1/32,DIRECT")
	idxQUIC := strings.Index(yaml, "AND,((DEST-PORT,443),(NETWORK,UDP)),REJECT")
	if idxQUIC < 0 || idxCIDR < 0 || idxQUIC < idxCIDR {
		t.Fatalf("expected quic block rule right after direct cidr rules:\n%s", yaml)
	}
}

func TestGenerateMetaYAMLDedupRuleProvidersByURL(t *testing.T) {
	req := models.GenerateMetaYAMLRequest{
		Nodes: []models.ProxyNode{
			{
				ID:       "n1",
				Name:     "A",
				Protocol: models.ProtocolVLESS,
				Server:   "example.com",
				Port:     443,
				UUID:     "123e4567-e89b-12d3-a456-426614174000",
				TLS:      true,
			},
		},
		SelectedNodeIDs: []string{"n1"},
		Selections: []models.ServiceSelection{
			{ServiceID: "svc-gmail", Enabled: true, Policy: "Proxy_Group"},
			{ServiceID: "svc-google-search", Enabled: true, Policy: "Proxy_Group"},
		},
		Mode:           models.ModeBlacklist,
		ProxyGroupName: "Proxy_Group",
		ServicesSnapshot: []models.ServiceTree{
			{ID: "svc-gmail", Kind: "service", Provider: "gmail"},
			{ID: "svc-google-search", Kind: "service", Provider: "google-search"},
		},
	}

	yaml, err := GenerateMetaYAML(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Count(yaml, "url: 'https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Clash/Google/Google.yaml'") != 1 {
		t.Fatalf("expected deduped provider URL once:\n%s", yaml)
	}
}

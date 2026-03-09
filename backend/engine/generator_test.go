package engine

import (
	"strings"
	"testing"

	"meta-link-pro/backend/models"
)

func TestGenerateMetaYAMLRuleOrder(t *testing.T) {
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
		Mode:           models.ModeWhitelist,
		ProxyGroupName: "Proxy_Group",
		ServicesSnapshot: []models.ServiceTree{
			{ID: "category-ai", Kind: "category", Children: []models.ServiceTree{{ID: "svc-openai", Kind: "service", Provider: "openai"}}},
		},
	}

	yaml, err := GenerateMetaYAML(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	idxCIDR := strings.Index(yaml, "IP-CIDR,192.168.1.100/32,DIRECT,no-resolve")
	idxSvc := strings.Index(yaml, "RULE-SET,openai,Proxy_Group")
	idxGlobal := strings.Index(yaml, "RULE-SET,private,DIRECT")
	idxMatch := strings.Index(yaml, "MATCH,DIRECT")

	if !(idxCIDR < idxSvc && idxSvc < idxGlobal && idxGlobal < idxMatch) {
		t.Fatalf("rule order invalid:\n%s", yaml)
	}
}

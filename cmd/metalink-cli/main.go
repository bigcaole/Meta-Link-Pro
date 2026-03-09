package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"meta-link-pro/backend/engine"
	"meta-link-pro/backend/models"
	"meta-link-pro/backend/services"
)

func main() {
	input := flag.String("input", "", "raw links/subscription/yaml content")
	inputFile := flag.String("input-file", "", "path to input text file")
	outputFile := flag.String("output", "meta-link-pro.yaml", "output yaml file path")
	proxyGroup := flag.String("proxy-group", "Proxy_Group", "proxy group name")
	directCIDRs := flag.String("direct-cidrs", "", "comma-separated source CIDRs/IPs for forced DIRECT")
	flag.Parse()

	raw, err := readInput(*input, *inputFile)
	if err != nil {
		exitErr(err)
	}

	report := engine.ParseInput(raw)
	for _, item := range report.Errors {
		fmt.Fprintf(os.Stderr, "[WARN] %s\n", item.Message)
	}
	if len(report.Nodes) == 0 {
		exitErr(fmt.Errorf("no valid proxy nodes parsed"))
	}

	selectedIDs := make([]string, 0, len(report.Nodes))
	for _, node := range report.Nodes {
		selectedIDs = append(selectedIDs, node.ID)
	}

	serviceTree, err := services.LoadServiceTree()
	if err != nil {
		exitErr(err)
	}

	req := models.GenerateMetaYAMLRequest{
		Nodes:            report.Nodes,
		SelectedNodeIDs:  selectedIDs,
		DirectCIDRs:      splitCSV(*directCIDRs),
		Selections:       nil,
		Mode:             models.ModeBlacklist,
		ProxyGroupName:   *proxyGroup,
		ServicesSnapshot: serviceTree,
	}

	yamlText, err := engine.GenerateMetaYAML(req)
	if err != nil {
		exitErr(err)
	}

	if err := os.WriteFile(*outputFile, []byte(yamlText), 0o644); err != nil {
		exitErr(err)
	}
	fmt.Printf("Generated YAML: %s\nNodes: %d\n", *outputFile, len(report.Nodes))
}

func readInput(input string, inputFile string) (string, error) {
	if strings.TrimSpace(input) != "" {
		return input, nil
	}
	if strings.TrimSpace(inputFile) != "" {
		data, err := os.ReadFile(inputFile)
		if err != nil {
			return "", fmt.Errorf("read input-file failed: %w", err)
		}
		return string(data), nil
	}
	return "", fmt.Errorf("please provide -input or -input-file")
}

func splitCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
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

func exitErr(err error) {
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	os.Exit(1)
}

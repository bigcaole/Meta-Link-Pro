package services

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"meta-link-pro/backend/models"
)

const ServiceConfigPath = "backend/services/services.json"

func LoadServiceTree() ([]models.ServiceTree, error) {
	data, err := os.ReadFile(filepath.Clean(ServiceConfigPath))
	if err != nil {
		return nil, fmt.Errorf("read services.json failed: %w", err)
	}

	var tree []models.ServiceTree
	if err := json.Unmarshal(data, &tree); err != nil {
		return nil, fmt.Errorf("parse services.json failed: %w", err)
	}
	return tree, nil
}

func FlattenServices(nodes []models.ServiceTree) map[string]models.ServiceTree {
	out := make(map[string]models.ServiceTree)
	var walk func(items []models.ServiceTree)
	walk = func(items []models.ServiceTree) {
		for _, item := range items {
			if item.Kind == "service" {
				out[item.ID] = item
			}
			if len(item.Children) > 0 {
				walk(item.Children)
			}
		}
	}
	walk(nodes)
	return out
}

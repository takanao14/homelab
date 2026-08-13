package main

import (
	"encoding/json"
	"fmt"
)

type dashboardJSON struct {
	Panels []panelJSON `json:"panels"`
}

type panelJSON struct {
	Title       string       `json:"title"`
	Options     panelOptions `json:"options"`
	FieldConfig fieldConfig  `json:"fieldConfig"`
	Panels      []panelJSON  `json:"panels"`
}

type panelOptions struct {
	ColorMode *string `json:"colorMode"`
}

type fieldConfig struct {
	Defaults fieldDefaults `json:"defaults"`
}

type fieldDefaults struct {
	Thresholds *thresholdsJSON `json:"thresholds"`
}

type thresholdsJSON struct {
	Steps []json.RawMessage `json:"steps"`
}

// validateDashboardJSON enforces the generated-dashboard invariant that every
// coloured panel owns an explicit threshold definition. Grafana otherwise
// supplies its unrelated green-below-80/red-at-80 default.
func validateDashboardJSON(name string, data []byte) error {
	var document dashboardJSON
	if err := json.Unmarshal(data, &document); err != nil {
		return fmt.Errorf("%s: decode generated JSON: %w", name, err)
	}

	return validatePanels(name, document.Panels)
}

func validatePanels(dashboardName string, panels []panelJSON) error {
	for _, panel := range panels {
		if panel.Options.ColorMode != nil &&
			*panel.Options.ColorMode != "none" &&
			(panel.FieldConfig.Defaults.Thresholds == nil ||
				len(panel.FieldConfig.Defaults.Thresholds.Steps) == 0) {
			return fmt.Errorf(
				"%s: panel %q uses colorMode %q without threshold steps",
				dashboardName,
				panel.Title,
				*panel.Options.ColorMode,
			)
		}

		if err := validatePanels(dashboardName, panel.Panels); err != nil {
			return err
		}
	}

	return nil
}

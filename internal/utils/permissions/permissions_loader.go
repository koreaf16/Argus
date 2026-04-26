package permissions

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/koreaf16/argus/internal/constants"
	"github.com/koreaf16/argus/internal/types"
)

type settingsPermissions struct {
	DefaultMode string   `json:"defaultMode,omitempty"`
	Allow       []string `json:"allow"`
	Deny        []string `json:"deny"`
	Ask         []string `json:"ask,omitempty"`

	raw map[string]json.RawMessage
}

type settingsJSON struct {
	ActiveModel string              `json:"activeModel"`
	Permissions settingsPermissions `json:"permissions"`

	raw map[string]json.RawMessage
}

func loadSettingsJSON() (*settingsJSON, error) {
	data, err := os.ReadFile(constants.SettingsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return &settingsJSON{
				raw: make(map[string]json.RawMessage),
				Permissions: settingsPermissions{
					raw: make(map[string]json.RawMessage),
				},
			}, nil
		}
		return nil, fmt.Errorf("settings read failed: %w", err)
	}

	var rootRaw map[string]json.RawMessage
	if err := json.Unmarshal(data, &rootRaw); err != nil {
		return nil, fmt.Errorf("settings parse failed: %w", err)
	}

	var s settingsJSON
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("settings parse failed: %w", err)
	}
	s.raw = rootRaw

	if rawPermissions, ok := rootRaw["permissions"]; ok {
		var permissionsRaw map[string]json.RawMessage
		if err := json.Unmarshal(rawPermissions, &permissionsRaw); err == nil {
			s.Permissions.raw = permissionsRaw
		}
	}
	if s.Permissions.raw == nil {
		s.Permissions.raw = make(map[string]json.RawMessage)
	}

	return &s, nil
}

func saveSettingsJSON(s *settingsJSON) error {
	root := make(map[string]json.RawMessage, len(s.raw)+2)
	for k, v := range s.raw {
		root[k] = v
	}

	activeBytes, err := json.Marshal(s.ActiveModel)
	if err != nil {
		return fmt.Errorf("settings serialize failed: %w", err)
	}
	root["activeModel"] = activeBytes

	permissionsBytes, err := marshalSettingsPermissions(s.Permissions)
	if err != nil {
		return fmt.Errorf("settings serialize failed: %w", err)
	}
	root["permissions"] = permissionsBytes

	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return fmt.Errorf("settings serialize failed: %w", err)
	}
	if err := os.WriteFile(constants.SettingsPath(), data, 0o600); err != nil {
		return fmt.Errorf("settings write failed: %w", err)
	}
	return nil
}

func marshalSettingsPermissions(p settingsPermissions) (json.RawMessage, error) {
	out := make(map[string]json.RawMessage, len(p.raw)+4)
	for k, v := range p.raw {
		out[k] = v
	}

	allowBytes, err := json.Marshal(normalizeRuleList(p.Allow))
	if err != nil {
		return nil, err
	}
	denyBytes, err := json.Marshal(normalizeRuleList(p.Deny))
	if err != nil {
		return nil, err
	}
	askBytes, err := json.Marshal(normalizeRuleList(p.Ask))
	if err != nil {
		return nil, err
	}

	out["allow"] = allowBytes
	out["deny"] = denyBytes
	out["ask"] = askBytes
	if p.DefaultMode != "" {
		defaultModeBytes, err := json.Marshal(p.DefaultMode)
		if err != nil {
			return nil, err
		}
		out["defaultMode"] = defaultModeBytes
	}

	raw, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func normalizeRuleList(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

func settingsToRules(s *settingsJSON, source types.PermissionRuleSource) []types.PermissionRule {
	if s == nil {
		return nil
	}
	var rules []types.PermissionRule
	behaviors := []struct {
		list     []string
		behavior types.PermissionBehavior
	}{
		{s.Permissions.Allow, types.BehaviorAllow},
		{s.Permissions.Deny, types.BehaviorDeny},
		{s.Permissions.Ask, types.BehaviorAsk},
	}
	for _, b := range behaviors {
		for _, rs := range b.list {
			rules = append(rules, types.PermissionRule{
				Source:       source,
				RuleBehavior: b.behavior,
				RuleValue:    PermissionRuleValueFromString(rs),
			})
		}
	}
	return rules
}

func LoadAllPermissionRulesFromDisk() []types.PermissionRule {
	s, err := loadSettingsJSON()
	if err != nil {
		return nil
	}
	return settingsToRules(s, types.RuleSourceUserSettings)
}

func AddPermissionRulesToSettings(ruleValues []types.PermissionRuleValue, behavior types.PermissionBehavior) error {
	s, err := loadSettingsJSON()
	if err != nil {
		s = &settingsJSON{
			raw: make(map[string]json.RawMessage),
			Permissions: settingsPermissions{
				raw: make(map[string]json.RawMessage),
			},
		}
	}

	existing := map[string]bool{}
	for _, rs := range listForBehavior(s, behavior) {
		canonical := PermissionRuleValueToString(PermissionRuleValueFromString(rs))
		existing[canonical] = true
	}

	var added []string
	for _, rv := range ruleValues {
		rs := PermissionRuleValueToString(rv)
		if !existing[rs] {
			added = append(added, rs)
		}
	}
	if len(added) == 0 {
		return nil
	}

	appendToBehavior(s, behavior, added)
	return saveSettingsJSON(s)
}

func DeletePermissionRuleFromSettings(rule types.PermissionRule) (bool, error) {
	s, err := loadSettingsJSON()
	if err != nil {
		return false, err
	}
	target := PermissionRuleValueToString(rule.RuleValue)
	list := listForBehavior(s, rule.RuleBehavior)
	newList := list[:0]
	found := false
	for _, rs := range list {
		canonical := PermissionRuleValueToString(PermissionRuleValueFromString(rs))
		if canonical == target {
			found = true
			continue
		}
		newList = append(newList, rs)
	}
	if !found {
		return false, nil
	}
	setListForBehavior(s, rule.RuleBehavior, newList)
	return true, saveSettingsJSON(s)
}

func listForBehavior(s *settingsJSON, b types.PermissionBehavior) []string {
	switch b {
	case types.BehaviorAllow:
		return s.Permissions.Allow
	case types.BehaviorDeny:
		return s.Permissions.Deny
	case types.BehaviorAsk:
		return s.Permissions.Ask
	}
	return nil
}

func appendToBehavior(s *settingsJSON, b types.PermissionBehavior, items []string) {
	switch b {
	case types.BehaviorAllow:
		s.Permissions.Allow = append(s.Permissions.Allow, items...)
	case types.BehaviorDeny:
		s.Permissions.Deny = append(s.Permissions.Deny, items...)
	case types.BehaviorAsk:
		s.Permissions.Ask = append(s.Permissions.Ask, items...)
	}
}

func setListForBehavior(s *settingsJSON, b types.PermissionBehavior, list []string) {
	switch b {
	case types.BehaviorAllow:
		s.Permissions.Allow = list
	case types.BehaviorDeny:
		s.Permissions.Deny = list
	case types.BehaviorAsk:
		s.Permissions.Ask = list
	}
}

package templates

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/openb00ks/openb00ks/internal/models"
)

// DefaultKey is the template applied when an entity is created without an explicit choice.
const DefaultKey = "basic"

//go:embed templates/*.json
var templateFS embed.FS

type Template struct {
	Key      string       `json:"key"`
	Name     string       `json:"name"`
	Accounts []AccountDef `json:"accounts"`
}

type AccountDef struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Code string `json:"code,omitempty"`
}

var (
	templates = map[string]Template{}
	loadErr   error
)

func init() {
	templates, loadErr = loadTemplates()
}

func Lookup(key string) (Template, error) {
	if loadErr != nil {
		return Template{}, loadErr
	}
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return Template{}, errors.New("template key required")
	}
	tmpl, ok := templates[key]
	if !ok {
		return Template{}, fmt.Errorf("unknown template: %s", key)
	}
	return tmpl, nil
}

// List returns every account template for the entity-create picker, with the default (basic) first and
// the rest alphabetical by name. Returns nil if templates failed to load.
func List() []Template {
	if loadErr != nil {
		return nil
	}
	out := make([]Template, 0, len(templates))
	for _, tmpl := range templates {
		out = append(out, tmpl)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Key == DefaultKey {
			return true
		}
		if out[j].Key == DefaultKey {
			return false
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func AccountsToModels(entityID string, defs []AccountDef) []models.Account {
	out := make([]models.Account, 0, len(defs))
	for _, def := range defs {
		out = append(out, models.Account{
			EntityID: entityID,
			Name:     def.Name,
			Type:     def.Type,
			Code:     def.Code,
		})
	}
	return out
}

func loadTemplates() (map[string]Template, error) {
	entries, err := templateFS.ReadDir("templates")
	if err != nil {
		return nil, err
	}
	out := make(map[string]Template, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || path.Ext(entry.Name()) != ".json" {
			continue
		}
		raw, err := templateFS.ReadFile(path.Join("templates", entry.Name()))
		if err != nil {
			return nil, err
		}
		var tmpl Template
		if err := json.Unmarshal(raw, &tmpl); err != nil {
			return nil, err
		}
		if err := validateTemplate(&tmpl); err != nil {
			return nil, fmt.Errorf("%s: %w", entry.Name(), err)
		}
		key := strings.ToLower(strings.TrimSpace(tmpl.Key))
		if _, exists := out[key]; exists {
			return nil, fmt.Errorf("duplicate template key: %s", key)
		}
		out[key] = tmpl
	}
	return out, nil
}

func validateTemplate(tmpl *Template) error {
	tmpl.Key = strings.TrimSpace(tmpl.Key)
	tmpl.Name = strings.TrimSpace(tmpl.Name)
	if tmpl.Key == "" {
		return errors.New("template key missing")
	}
	if tmpl.Name == "" {
		return errors.New("template name missing")
	}
	if len(tmpl.Accounts) == 0 {
		return errors.New("template accounts missing")
	}
	for i := range tmpl.Accounts {
		account := &tmpl.Accounts[i]
		account.Name = strings.TrimSpace(account.Name)
		if account.Name == "" {
			return fmt.Errorf("account[%d] name missing", i)
		}
		account.Type = normalizeAccountType(account.Type)
		if account.Type == "" {
			return fmt.Errorf("account[%d] invalid type", i)
		}
		account.Code = strings.TrimSpace(account.Code)
	}
	return nil
}

func normalizeAccountType(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "asset", "assets":
		return "asset"
	case "liability", "liabilities":
		return "liability"
	case "equity":
		return "equity"
	case "income", "revenue":
		return "income"
	case "expense", "expenses":
		return "expense"
	default:
		return ""
	}
}

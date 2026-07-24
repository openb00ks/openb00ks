package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type TypesenseConfig struct {
	URL              string
	APIKey           string
	CollectionPrefix string
	HTTPClient       *http.Client
}

type TypesenseProvider struct {
	baseURL          string
	apiKey           string
	collectionPrefix string
	client           *http.Client
}

func NewTypesenseProvider(cfg TypesenseConfig) (*TypesenseProvider, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.URL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("typesense url is required")
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("typesense api key is required")
	}
	prefix := strings.TrimSpace(cfg.CollectionPrefix)
	if prefix == "" {
		prefix = "openb00ks"
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	return &TypesenseProvider{
		baseURL:          baseURL,
		apiKey:           cfg.APIKey,
		collectionPrefix: prefix,
		client:           client,
	}, nil
}

func (p *TypesenseProvider) SearchTransactions(ctx context.Context, query TransactionQuery) ([]TransactionMatch, error) {
	if strings.TrimSpace(query.TenantID) == "" || strings.TrimSpace(query.EntityID) == "" {
		return nil, ErrScopeRequired
	}
	limit := query.Limit
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	body := map[string]interface{}{
		"q":         searchQ(query.Query),
		"query_by":  "memo,description,account_names,account_role_tags",
		"filter_by": p.entityFilter(query.TenantID, query.EntityID),
		"per_page":  limit,
		"sort_by":   "_text_match:desc,date_unix:desc",
	}
	var response typesenseSearchResponse
	if err := p.doJSON(ctx, http.MethodPost, p.collectionPath()+"/documents/search", body, &response); err != nil {
		return nil, err
	}
	return transactionMatchesFromResponse(response), nil
}

func (p *TypesenseProvider) SearchDocuments(ctx context.Context, query DocumentQuery) ([]DocumentMatch, error) {
	if strings.TrimSpace(query.TenantID) == "" || strings.TrimSpace(query.EntityID) == "" {
		return nil, ErrScopeRequired
	}
	limit := query.Limit
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	filter := p.entityFilter(query.TenantID, query.EntityID)
	if len(query.Kinds) > 0 {
		if escaped := escapedFilterValues(query.Kinds); len(escaped) > 0 {
			filter += " && kind:=[" + strings.Join(escaped, ",") + "]"
		}
	}
	if len(query.Statuses) > 0 {
		if escaped := escapedFilterValues(query.Statuses); len(escaped) > 0 {
			filter += " && status:=[" + strings.Join(escaped, ",") + "]"
		}
	}
	if len(query.AccountIDs) > 0 {
		if escaped := escapedFilterValues(query.AccountIDs); len(escaped) > 0 {
			filter += " && account_id:=[" + strings.Join(escaped, ",") + "]"
		}
	}
	if len(query.Tags) > 0 {
		if escaped := escapedFilterValues(query.Tags); len(escaped) > 0 {
			filter += " && tags:=[" + strings.Join(escaped, ",") + "]"
		}
	}
	if startUnix, ok := dateUnixFromQuery(query.StartDate, false); ok {
		filter += fmt.Sprintf(" && date_unix:>=%d", startUnix)
	}
	if endUnix, ok := dateUnixFromQuery(query.EndDate, true); ok {
		filter += fmt.Sprintf(" && date_unix:<=%d", endUnix)
	}
	body := map[string]interface{}{
		"q":         searchQ(query.Query),
		"query_by":  "title,subtitle,body,status,kind,account_name,tags",
		"filter_by": filter,
		"per_page":  limit,
		"sort_by":   "_text_match:desc,date_unix:desc",
	}
	var response typesenseDocumentSearchResponse
	if err := p.doJSON(ctx, http.MethodPost, p.documentCollectionPath()+"/documents/search", body, &response); err != nil {
		return nil, err
	}
	return documentMatchesFromResponse(response), nil
}

func (p *TypesenseProvider) SuggestCandidates(ctx context.Context, query CandidateQuery) ([]Candidate, error) {
	if strings.TrimSpace(query.TenantID) == "" || strings.TrimSpace(query.EntityID) == "" {
		return nil, ErrScopeRequired
	}
	limit := query.Limit
	if limit <= 0 || limit > 10 {
		limit = 5
	}
	body := map[string]interface{}{
		"q":         searchQ(query.Query),
		"query_by":  "memo,description,account_names,account_role_tags",
		"filter_by": p.entityFilter(query.TenantID, query.EntityID),
		"per_page":  limit,
		"sort_by":   "_text_match:desc,date_unix:desc",
	}
	var response typesenseSearchResponse
	if err := p.doJSON(ctx, http.MethodPost, p.collectionPath()+"/documents/search", body, &response); err != nil {
		return nil, err
	}
	matches := transactionMatchesFromResponse(response)
	candidates := make([]Candidate, 0, len(matches))
	for _, match := range matches {
		doc := match.Document
		accountID := ""
		if len(doc.AccountIDs) > 0 {
			accountID = doc.AccountIDs[0]
		}
		accountName := ""
		if len(doc.AccountNames) > 0 {
			accountName = doc.AccountNames[0]
		}
		score := match.Score
		if query.AmountCents > 0 && doc.AmountCents > 0 {
			score = score * amountSimilarity(query.AmountCents, doc.AmountCents)
		}
		candidates = append(candidates, Candidate{
			TransactionID:   doc.TransactionID,
			AccountID:       accountID,
			AccountName:     accountName,
			AccountRoleTags: doc.AccountRoleTags,
			Memo:            doc.Memo,
			Description:     doc.Description,
			AmountCents:     doc.AmountCents,
			Date:            doc.Date,
			Score:           score,
		})
	}
	return candidates, nil
}

func (p *TypesenseProvider) SearchVendors(ctx context.Context, query VendorQuery) ([]VendorMatch, error) {
	if strings.TrimSpace(query.TenantID) == "" || strings.TrimSpace(query.EntityID) == "" {
		return nil, ErrScopeRequired
	}
	limit := query.Limit
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	body := map[string]interface{}{
		"q":         searchQ(query.Query),
		"query_by":  "name,aliases,match_pattern",
		"filter_by": p.entityFilter(query.TenantID, query.EntityID),
		"per_page":  limit,
		"sort_by":   "_text_match:desc,receipt_count:desc",
	}
	var response typesenseVendorSearchResponse
	if err := p.doJSON(ctx, http.MethodPost, p.vendorCollectionPath()+"/documents/search", body, &response); err != nil {
		return nil, err
	}
	matches := make([]VendorMatch, 0, len(response.Hits))
	for _, hit := range response.Hits {
		matches = append(matches, VendorMatch{Document: hit.Document, Score: textMatchScore(hit.TextMatch)})
	}
	return matches, nil
}

func (p *TypesenseProvider) UpsertVendor(ctx context.Context, doc VendorDocument) error {
	if strings.TrimSpace(doc.TenantID) == "" || strings.TrimSpace(doc.EntityID) == "" {
		return ErrScopeRequired
	}
	if strings.TrimSpace(doc.ID) == "" {
		return fmt.Errorf("vendor document id is required")
	}
	if doc.Aliases == nil {
		doc.Aliases = []string{}
	}
	return p.doJSON(ctx, http.MethodPost, p.vendorCollectionPath()+"/documents?action=upsert", doc, nil)
}

func (p *TypesenseProvider) DeleteVendor(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	err := p.doJSON(ctx, http.MethodDelete, p.vendorCollectionPath()+"/documents/"+url.PathEscape(id), nil, nil)
	if err == nil || strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
		return nil
	}
	return err
}

func (p *TypesenseProvider) EnsureVendorCollection(ctx context.Context) error {
	body := map[string]interface{}{
		"name": p.vendorCollection(),
		"fields": []map[string]interface{}{
			{"name": "tenant_id", "type": "string", "facet": true},
			{"name": "entity_id", "type": "string", "facet": true},
			{"name": "name", "type": "string"},
			{"name": "aliases", "type": "string[]", "optional": true},
			{"name": "match_pattern", "type": "string", "optional": true},
			{"name": "tax_id", "type": "string", "facet": true, "optional": true},
			{"name": "website", "type": "string", "optional": true},
			{"name": "default_account_id", "type": "string", "facet": true, "optional": true},
			{"name": "receipt_count", "type": "int32"},
			{"name": "last_seen_unix", "type": "int64"},
		},
		// Split payment-processor noise so "SQ*BLUE" / "AMZN MKTP*2X" tokenize into matchable terms.
		"token_separators":      []string{"*", "#"},
		"default_sorting_field": "last_seen_unix",
	}
	err := p.doJSON(ctx, http.MethodPost, "/collections", body, nil)
	if err == nil || strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "409") {
		return nil
	}
	return err
}

func (p *TypesenseProvider) UpsertTransaction(ctx context.Context, doc TransactionDocument) error {
	if strings.TrimSpace(doc.TenantID) == "" || strings.TrimSpace(doc.EntityID) == "" {
		return ErrScopeRequired
	}
	if doc.ID == "" {
		doc.ID = doc.TransactionID
	}
	if doc.Source == "" {
		doc.Source = "transaction"
	}
	path := p.collectionPath() + "/documents?action=upsert"
	return p.doJSON(ctx, http.MethodPost, path, doc, nil)
}

func (p *TypesenseProvider) UpsertDocument(ctx context.Context, doc SearchDocument) error {
	if strings.TrimSpace(doc.TenantID) == "" || strings.TrimSpace(doc.EntityID) == "" {
		return ErrScopeRequired
	}
	if strings.TrimSpace(doc.Kind) == "" || strings.TrimSpace(doc.ObjectID) == "" {
		return fmt.Errorf("kind and object_id are required")
	}
	if doc.ID == "" {
		doc.ID = doc.Kind + "_" + doc.ObjectID
	}
	if doc.IndexedAt.IsZero() {
		doc.IndexedAt = time.Now().UTC()
	}
	path := p.documentCollectionPath() + "/documents?action=upsert"
	return p.doJSON(ctx, http.MethodPost, path, doc, nil)
}

func (p *TypesenseProvider) DeleteDocument(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	path := p.documentCollectionPath() + "/documents/" + url.PathEscape(id)
	err := p.doJSON(ctx, http.MethodDelete, path, nil, nil)
	if err == nil || strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
		return nil
	}
	return err
}

func (p *TypesenseProvider) EnsureTransactionCollection(ctx context.Context) error {
	body := map[string]interface{}{
		"name": p.transactionCollection(),
		"fields": []map[string]interface{}{
			{"name": "tenant_id", "type": "string", "facet": true},
			{"name": "entity_id", "type": "string", "facet": true},
			{"name": "transaction_id", "type": "string"},
			{"name": "date", "type": "string"},
			{"name": "date_unix", "type": "int64"},
			{"name": "memo", "type": "string"},
			{"name": "description", "type": "string"},
			{"name": "account_ids", "type": "string[]", "facet": true},
			{"name": "account_names", "type": "string[]"},
			{"name": "account_role_tags", "type": "string[]", "facet": true},
			{"name": "amount_cents", "type": "int64"},
			{"name": "source", "type": "string", "facet": true},
		},
		"default_sorting_field": "date_unix",
	}
	err := p.doJSON(ctx, http.MethodPost, "/collections", body, nil)
	if err == nil || strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "409") {
		return nil
	}
	return err
}

func (p *TypesenseProvider) EnsureDocumentCollection(ctx context.Context) error {
	body := map[string]interface{}{
		"name":                  p.documentCollection(),
		"fields":                documentCollectionFields(),
		"default_sorting_field": "date_unix",
	}
	err := p.doJSON(ctx, http.MethodPost, "/collections", body, nil)
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "409") {
		return p.ensureDocumentCollectionFields(ctx)
	}
	return err
}

func (p *TypesenseProvider) ensureDocumentCollectionFields(ctx context.Context) error {
	var collection typesenseCollectionResponse
	if err := p.doJSON(ctx, http.MethodGet, p.documentCollectionPath(), nil, &collection); err != nil {
		return err
	}
	existing := map[string]struct{}{}
	for _, field := range collection.Fields {
		existing[field.Name] = struct{}{}
	}
	missing := []map[string]interface{}{}
	for _, field := range documentCollectionFields() {
		name, _ := field["name"].(string)
		if name == "" {
			continue
		}
		if _, ok := existing[name]; !ok {
			missing = append(missing, field)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return p.doJSON(ctx, http.MethodPatch, p.documentCollectionPath(), map[string]interface{}{"fields": missing}, nil)
}

func (p *TypesenseProvider) transactionCollection() string {
	return p.collectionPrefix + "_transactions"
}

func (p *TypesenseProvider) documentCollection() string {
	return p.collectionPrefix + "_documents"
}

func (p *TypesenseProvider) vendorCollection() string {
	return p.collectionPrefix + "_vendors"
}

func (p *TypesenseProvider) vendorCollectionPath() string {
	return "/collections/" + url.PathEscape(p.vendorCollection())
}

func (p *TypesenseProvider) collectionPath() string {
	return "/collections/" + url.PathEscape(p.transactionCollection())
}

func (p *TypesenseProvider) documentCollectionPath() string {
	return "/collections/" + url.PathEscape(p.documentCollection())
}

func (p *TypesenseProvider) entityFilter(tenantID, entityID string) string {
	parts := []string{}
	if strings.TrimSpace(tenantID) != "" {
		parts = append(parts, `tenant_id:=`+escapeFilterValue(tenantID))
	}
	if strings.TrimSpace(entityID) != "" {
		parts = append(parts, `entity_id:=`+escapeFilterValue(entityID))
	}
	return strings.Join(parts, " && ")
}

func (p *TypesenseProvider) doJSON(ctx context.Context, method, path string, body interface{}, out interface{}) error {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, p.baseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("X-TYPESENSE-API-KEY", p.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("typesense %s %s returned %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(payload)))
	}
	if out == nil || len(payload) == 0 {
		return nil
	}
	return json.Unmarshal(payload, out)
}

type typesenseSearchResponse struct {
	Hits []struct {
		Document      TransactionDocument `json:"document"`
		TextMatch     int64               `json:"text_match"`
		TextMatchInfo struct {
			Score string `json:"score"`
		} `json:"text_match_info"`
	} `json:"hits"`
}

type typesenseDocumentSearchResponse struct {
	Hits []struct {
		Document      SearchDocument `json:"document"`
		TextMatch     int64          `json:"text_match"`
		TextMatchInfo struct {
			Score string `json:"score"`
		} `json:"text_match_info"`
	} `json:"hits"`
}

type typesenseVendorSearchResponse struct {
	Hits []struct {
		Document  VendorDocument `json:"document"`
		TextMatch int64          `json:"text_match"`
	} `json:"hits"`
}

type typesenseCollectionResponse struct {
	Fields []struct {
		Name string `json:"name"`
	} `json:"fields"`
}

func transactionMatchesFromResponse(response typesenseSearchResponse) []TransactionMatch {
	matches := make([]TransactionMatch, 0, len(response.Hits))
	for _, hit := range response.Hits {
		score := textMatchScore(hit.TextMatch)
		matches = append(matches, TransactionMatch{
			Document: hit.Document,
			Score:    score,
		})
	}
	return matches
}

func documentMatchesFromResponse(response typesenseDocumentSearchResponse) []DocumentMatch {
	matches := make([]DocumentMatch, 0, len(response.Hits))
	for _, hit := range response.Hits {
		matches = append(matches, DocumentMatch{
			Document: hit.Document,
			Score:    textMatchScore(hit.TextMatch),
		})
	}
	return matches
}

func textMatchScore(raw int64) float64 {
	if raw <= 0 {
		return 0.5
	}
	if raw >= 1_000_000 {
		return 1
	}
	return 0.5 + (float64(raw) / 2_000_000)
}

func amountSimilarity(want, got int64) float64 {
	if want <= 0 || got <= 0 {
		return 1
	}
	diff := want - got
	if diff < 0 {
		diff = -diff
	}
	if diff == 0 {
		return 1
	}
	larger := want
	if got > larger {
		larger = got
	}
	ratio := float64(diff) / float64(larger)
	if ratio >= 1 {
		return 0.5
	}
	return 1 - (ratio * 0.5)
}

func searchQ(query string) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return "*"
	}
	return query
}

func escapeFilterValue(value string) string {
	value = strings.ReplaceAll(value, "`", "")
	return "`" + value + "`"
}

func documentCollectionFields() []map[string]interface{} {
	return []map[string]interface{}{
		{"name": "tenant_id", "type": "string", "facet": true},
		{"name": "entity_id", "type": "string", "facet": true},
		{"name": "kind", "type": "string", "facet": true},
		{"name": "object_id", "type": "string"},
		{"name": "account_id", "type": "string", "facet": true, "optional": true},
		{"name": "account_name", "type": "string", "optional": true},
		{"name": "title", "type": "string"},
		{"name": "subtitle", "type": "string"},
		{"name": "body", "type": "string"},
		{"name": "status", "type": "string", "facet": true},
		{"name": "tags", "type": "string[]", "facet": true, "optional": true},
		{"name": "date", "type": "string", "optional": true},
		{"name": "date_unix", "type": "int64"},
		{"name": "amount_cents", "type": "int64", "optional": true},
		{"name": "href", "type": "string", "optional": true},
	}
}

func escapedFilterValues(values []string) []string {
	escaped := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			escaped = append(escaped, escapeFilterValue(value))
		}
	}
	return escaped
}

func dateUnixFromQuery(value string, endOfDay bool) (int64, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return 0, false
	}
	if endOfDay {
		parsed = parsed.Add(24*time.Hour - time.Second)
	}
	return parsed.Unix(), true
}

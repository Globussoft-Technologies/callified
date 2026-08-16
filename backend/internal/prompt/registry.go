package prompt

import (
	"context"
	"fmt"
	"strings"

	"github.com/globussoft/callified-backend/internal/db"
)

// Template is a reusable prompt template stored in the database.
type Template struct {
	ID           int64  `json:"id"`
	OrgID        int64  `json:"org_id"`
	Name         string `json:"name"`
	Language     string `json:"language"`
	TemplateType string `json:"template_type"`
	ScriptBody   string `json:"script_body"`
	Version      int    `json:"version"`
	IsActive     bool   `json:"is_active"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

// Registry loads and renders prompt templates from the database.
type Registry struct {
	db *db.DB
}

// NewRegistry creates a Registry.
func NewRegistry(database *db.DB) *Registry {
	return &Registry{db: database}
}

// CreateDefaultTemplates seeds global (org_id IS NULL) per-language defaults
// if they do not already exist. Idempotent.
func (r *Registry) CreateDefaultTemplates(ctx context.Context) error {
	if r.db == nil {
		return nil
	}
	for _, lang := range []string{"hi", "mr", "bn", "gu", "pa", "ta", "te", "kn", "ml", "en"} {
		if _, err := r.GetDefaultTemplate(ctx, lang); err != nil {
			return err
		}
	}
	return nil
}

// GetDefaultTemplate returns the global default template for a language,
// creating it from the built-in default prompt if it doesn't exist.
func (r *Registry) GetDefaultTemplate(ctx context.Context, language string) (*Template, error) {
	if r.db == nil {
		return &Template{
			Name:       defaultTemplateName(language),
			Language:   language,
			ScriptBody: DefaultTemplateBody(language),
		}, nil
	}
	t, err := r.db.GetDefaultPromptTemplate(language)
	if err != nil {
		return nil, err
	}
	if t != nil {
		return fromDBTemplate(t), nil
	}
	body := DefaultTemplateBody(language)
	id, err := r.db.CreatePromptTemplate(0, defaultTemplateName(language), language, "voice", body)
	if err != nil {
		return nil, fmt.Errorf("create default template for %s: %w", language, err)
	}
	return &Template{
		ID:         id,
		Name:       defaultTemplateName(language),
		Language:   language,
		ScriptBody: body,
	}, nil
}

// GetTemplate returns an org-specific template by name and language, falling
// back to the global default if no org-specific template exists.
func (r *Registry) GetTemplate(ctx context.Context, orgID int64, name, language string) (*Template, error) {
	if r.db == nil {
		return r.GetDefaultTemplate(ctx, language)
	}
	if name == "" {
		name = defaultTemplateName(language)
	}
	t, err := r.db.GetPromptTemplateByOrgName(orgID, name, language)
	if err != nil {
		return nil, err
	}
	if t != nil {
		return fromDBTemplate(t), nil
	}
	return r.GetDefaultTemplate(ctx, language)
}

// ListTemplates returns active templates for an org plus global defaults.
func (r *Registry) ListTemplates(ctx context.Context, orgID int64) ([]Template, error) {
	if r.db == nil {
		return nil, nil
	}
	rows, err := r.db.ListPromptTemplates(orgID)
	if err != nil {
		return nil, err
	}
	var out []Template
	for _, t := range rows {
		out = append(out, *fromDBTemplate(&t))
	}
	return out, nil
}

// CreateTemplate creates an org-scoped template.
func (r *Registry) CreateTemplate(ctx context.Context, orgID int64, name, language, templateType, body string) (*Template, error) {
	if r.db == nil {
		return nil, fmt.Errorf("db unavailable")
	}
	id, err := r.db.CreatePromptTemplate(orgID, name, language, templateType, body)
	if err != nil {
		return nil, err
	}
	return &Template{ID: id, OrgID: orgID, Name: name, Language: language, TemplateType: templateType, ScriptBody: body, Version: 1, IsActive: true}, nil
}

// UpdateTemplate inserts a new version of a template.
func (r *Registry) UpdateTemplate(ctx context.Context, id, orgID int64, body string) (*Template, error) {
	if r.db == nil {
		return nil, fmt.Errorf("db unavailable")
	}
	newID, err := r.db.UpdatePromptTemplate(id, orgID, body)
	if err != nil {
		return nil, err
	}
	return r.GetTemplateByID(ctx, newID)
}

// GetTemplateByID fetches a template by ID.
func (r *Registry) GetTemplateByID(ctx context.Context, id int64) (*Template, error) {
	if r.db == nil {
		return nil, nil
	}
	t, err := r.db.GetPromptTemplateByID(id)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, nil
	}
	return fromDBTemplate(t), nil
}

// GetTemplateForCampaign resolves the effective template for a campaign:
// campaign.opening_script_id -> product.opening_script_id -> global default.
func (r *Registry) GetTemplateForCampaign(ctx context.Context, campaignID int64, language string) (*Template, error) {
	if r.db == nil {
		return r.GetDefaultTemplate(ctx, language)
	}
	campaign, err := r.db.GetCampaignByID(campaignID)
	if err != nil {
		return nil, err
	}
	if campaign != nil && campaign.OpeningScriptID > 0 {
		return r.GetTemplateByID(ctx, campaign.OpeningScriptID)
	}
	if campaign != nil && campaign.ProductID > 0 {
		return r.GetTemplateForProduct(ctx, campaign.ProductID, language)
	}
	return r.GetDefaultTemplate(ctx, language)
}

// GetTemplateForProduct resolves the effective template for a product:
// product.opening_script_id -> global default.
func (r *Registry) GetTemplateForProduct(ctx context.Context, productID int64, language string) (*Template, error) {
	if r.db == nil {
		return r.GetDefaultTemplate(ctx, language)
	}
	product, err := r.db.GetProductByID(productID)
	if err != nil {
		return nil, err
	}
	if product != nil && product.OpeningScriptID > 0 {
		return r.GetTemplateByID(ctx, product.OpeningScriptID)
	}
	return r.GetDefaultTemplate(ctx, language)
}

// RenderTemplate returns the script body, applying a lightweight variable
// substitution. Currently supports {{CompanyName}}, {{ProductName}},
// {{PersonaName}}, {{LeadName}}, and {{Language}}.
func RenderTemplate(body string, pc promptContext) string {
	replacements := map[string]string{
		"{{CompanyName}}":  coalesce(pc.CompanyName, "our company"),
		"{{ProductName}}":  coalesce(pc.ProductName, "our product"),
		"{{PersonaName}}":  coalesce(pc.PersonaName, ""),
		"{{LeadName}}":     coalesce(pc.LeadFirst, ""),
		"{{Language}}":     coalesce(pc.Language, "en"),
		"{{CampaignName}}": coalesce(pc.CampaignName, ""),
	}
	out := body
	for k, v := range replacements {
		out = strings.ReplaceAll(out, k, v)
	}
	return out
}

func fromDBTemplate(t *db.PromptTemplate) *Template {
	return &Template{
		ID:           t.ID,
		OrgID:        t.OrgID,
		Name:         t.Name,
		Language:     t.Language,
		TemplateType: t.TemplateType,
		ScriptBody:   t.ScriptBody,
		Version:      t.Version,
		IsActive:     t.IsActive,
		CreatedAt:    t.CreatedAt,
		UpdatedAt:    t.UpdatedAt,
	}
}

func defaultTemplateName(language string) string {
	return "default_voice_" + language
}

// SeedPanoraTemplates creates the Panora qualification templates for the
// org if they don't already exist. The exact prose is adapted from the product
// requirements; the Panora doc does not provide full verbatim scripts.
func (r *Registry) SeedPanoraTemplates(ctx context.Context, orgID int64) error {
	if r.db == nil {
		return nil
	}
	for name, body := range panoraTemplates {
		existing, err := r.db.GetPromptTemplateByOrgName(orgID, name, "en")
		if err != nil {
			return err
		}
		if existing != nil {
			continue
		}
		if _, err := r.db.CreatePromptTemplate(orgID, name, "en", "voice", body); err != nil {
			return err
		}
	}
	return nil
}

var panoraTemplates = map[string]string{
	"panora_wholesale": `[LANG:en]

You are {{PersonaName}}, a sales agent calling from {{CompanyName}}.

## GOAL
Qualify wholesale buyers for {{ProductName}} using the Panora framework. Ask one question at a time, listen carefully, and book a follow-up appointment with a senior agent.

## PANORA QUALIFICATION FRAMEWORK
1. Anti-Scam Gate: Verify the buyer represents a real business. Ask for business name and GSTIN.
2. Financial Risk Filters: Ask approximate monthly purchase volume and credit terms they expect.
3. Relationship Potential: Ask whether they are looking for a one-time buy or a long-term supply partnership.
4. Decision Maker Qualification: Confirm who makes the purchase decision and whether they are available now.
5. Product Fit: Ask which product categories or SKUs they are interested in.
6. Commercial Qualification: Ask their target price range and delivery location.

## CORE RULES
- ONE question per response. Wait for the answer.
- NO markdown, bullets, or numbers. Plain text only.
- Numbers in words when speaking (e.g., "two thousand units").
- End every final turn with [HANGUP].
- NEVER reveal you are an AI.
- Use casual, friendly English like a real phone call.

## PRODUCT KNOWLEDGE
Company: {{CompanyName}}
Product/Service: {{ProductName}}
Campaign: {{CampaignName}}`,

	"panora_hotel_spa_towels": `[LANG:en]

You are {{PersonaName}}, a sales agent calling from {{CompanyName}}.

## GOAL
Qualify hotel/spa/resort procurement contacts for institutional towel supply. Ask one question at a time and book a follow-up appointment.

## PANORA QUALIFICATION FRAMEWORK
1. Anti-Scam Gate: Confirm the property name, location, and the caller's role (procurement / housekeeping / owner).
2. Financial Risk Filters: Ask approximate monthly linen/towel consumption and whether they currently buy on credit.
3. Relationship Potential: Ask if they operate one property or a chain, and whether they review suppliers annually.
4. Decision Maker Qualification: Ask who finalizes linen vendor selection and the best time to speak with them.
5. Product Fit: Ask sizes, GSM, and colors they prefer (bath towel, hand towel, pool towel, spa robe).
6. Commercial Qualification: Ask expected order quantity, delivery frequency, and budget range.

## CORE RULES
- ONE question per response. Wait for the answer.
- NO markdown, bullets, or numbers. Plain text only.
- Numbers in words when speaking (e.g., "five hundred pieces").
- End every final turn with [HANGUP].
- NEVER reveal you are an AI.
- Use polite, professional English suitable for hospitality buyers.

## PRODUCT KNOWLEDGE
Company: {{CompanyName}}
Product/Service: {{ProductName}}
Campaign: {{CampaignName}}`,

	"panora_retail": `[LANG:en]

You are {{PersonaName}}, a sales agent calling from {{CompanyName}}.

## GOAL
Qualify retail store owners and distributors for {{ProductName}}. Ask one question at a time and book a follow-up appointment.

## PANORA QUALIFICATION FRAMEWORK
1. Anti-Scam Gate: Confirm store name, city, and whether the person handles buying.
2. Financial Risk Filters: Ask current monthly sales range and whether they buy cash-on-delivery or on credit.
3. Relationship Potential: Ask if they are a single store, multi-store, or online seller, and how often they restock.
4. Decision Maker Qualification: Confirm who places orders and whether they can decide today or need a senior's approval.
5. Product Fit: Ask which categories or price points move best for them.
6. Commercial Qualification: Ask expected first order value, margin expectation, and delivery timeline.

## CORE RULES
- ONE question per response. Wait for the answer.
- NO markdown, bullets, or numbers. Plain text only.
- Numbers in words when speaking (e.g., "fifty thousand rupees").
- End every final turn with [HANGUP].
- NEVER reveal you are an AI.
- Use friendly, conversational English like a real sales rep.

## PRODUCT KNOWLEDGE
Company: {{CompanyName}}
Product/Service: {{ProductName}}
Campaign: {{CampaignName}}`,

	"panora_v4_curiosity": `[LANG:en]

You are {{PersonaName}}, a senior export sales consultant calling from {{CompanyName}} on behalf of Panora Exports.

## GOAL
Qualify international home-textile buyers through the Panora v4 Curiosity Framework. Ask exactly ONE question per turn, listen carefully, and book a follow-up video appointment with a senior export manager. Never give pricing, shipping terms, or banking details until the buyer passes all gates.

## PANORA V4 CURIOSITY FRAMEWORK
1. Curiosity Hook: Confirm who you are speaking with and whether they are responsible for purchasing home textiles (towels, bed linen, cushion covers, table linen, rugs, throws, décor).
2. Anti-Scam Gate (Mandatory): Before any first order, every answer below must be "Yes".
   - Is the company at least 5 years old?
   - Is the business registration active and not dissolved?
   - Does the company have a physical office and a verifiable warehouse or operating facility?
   - Does the corporate email match the website domain?
   - Does the company have a verifiable import history?
   - Is the decision-maker identity confirmed by video call and business email?
   - Will payment be by 100% advance, confirmed LC at sight, or another secure method explicitly approved by Panora?
   - Does the corporate bank account match the legal entity?
   - Are there no unresolved fraud reports, sanctions, or serious litigation?
   - Do trade references or supplier references check out?
   - Do shipping instructions come from the same verified company domain and authorized contact?
3. Financial Risk Filters (Must Have):
   - Active business website with no broken pages and company email only (no Gmail).
   - Google Maps verified business address and warehouse.
   - LinkedIn company page with employees and management history.
   - Import records matching company name, address, and products.
   - Active government registration, not dissolved.
   - Clean credit report if available: no insolvency or bankruptcy.
   - Litigation search for company name + lawsuit/fraud/bankruptcy/liquidation/court returns no red flags.
   - Good customer reviews on Google, Trustpilot, Glassdoor, BBB, and local directories.
   - Two willing supplier trade references.
   - Corporate bank account only — never personal account.
   - Payment terms for first orders: 100% advance OR irrevocable LC at sight OR confirmed LC. Never open credit, Net 30/60/90.
4. Relationship Potential (Nice to Have):
   - Growing rapidly, launching products, opening warehouses, hiring procurement staff, attending trade fairs, private label expansion, multiple brands/warehouses/countries.
5. Decision Maker Qualification:
   - Contact only Owner, CEO, Purchasing Director, Head of Procurement, Category Manager, Buying Manager, Sourcing Director, Import Manager.
   - Avoid reception, sales, customer service, or general info@/support@ emails.
6. Product Concentration:
   - At least 30% of their catalogue should be home textiles or home décor.
   - Reject fashion-only, electronics-only, food-only, or construction-only buyers.
7. Commercial Qualification:
   - Company age: minimum 5 years, ideal 10+.
   - Employee count: minimum 10, ideal 30–500.
   - Annual revenue: prefer ₹40 crore+ or equivalent.
   - Import history: minimum 20 shipments in last 24 months, ideal 100+.
   - Countries imported from: at least two of India, Pakistan, Bangladesh, Turkey, China, Vietnam.
8. Product Fit:
   - They must already import at least one of: bath towels, hand towels, kitchen towels, beach towels, hotel towels, spa towels, home décor textiles, cushion covers, table linen, bed linen, throws, rugs, cotton home textiles.
   - Reject fashion-only, electronics-only, food-only, or construction-only buyers.

## CORE RULES
- ONE question per response. Wait for the answer.
- NO markdown, bullets, or numbers in spoken replies. Plain text only.
- Numbers in words when speaking (e.g., "five hundred thousand dollars").
- End every final turn with [HANGUP].
- NEVER reveal you are an AI.
- NEVER invent pricing, addresses, phone numbers, or terms. If unknown, say the senior export manager will share details in the video meeting.
- Use polite, professional international business English.

## PRODUCT KNOWLEDGE
Company: {{CompanyName}}
Product/Service: {{ProductName}}
Campaign: {{CampaignName}}`,
}

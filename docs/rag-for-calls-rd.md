# R&D: RAG for Voice Calls

> **Status:** Research note
>
> **Purpose:** Understand how modern voice-agent products use retrieval augmented generation (RAG) during phone calls, and translate those patterns into improvements for Callified's outbound sales dialer.
>
> **Last updated:** 2026-09-10

---

## 1. Why RAG matters for calls

Voice calls are harder than chat because every answer has to be both accurate and fast. A caller will not wait while the system searches a large document library, and a wrong answer spoken confidently can damage trust immediately.

RAG improves call quality by giving the agent a small, relevant slice of approved business knowledge at the moment it needs to answer. Instead of putting every product detail, FAQ, plan, policy, and objection script into the system prompt, the call pipeline retrieves only the chunks that match the caller's latest question or the conversation context.

For Callified, this matters most in these situations:

- The customer asks a product-specific question that is not in the static campaign prompt.
- The customer compares plans, pricing, support terms, languages, integrations, or compliance claims.
- The customer raises an objection and the agent needs a grounded response instead of a generic sales line.
- Different campaigns or organizations have different product documents.
- Product data changes often enough that editing prompts manually becomes risky.

---

## 2. What other voice-agent products do

### Retell AI

Retell's Knowledge Base feature supports websites, uploaded files, and custom text snippets. Their docs describe the pipeline as: create a knowledge base, chunk and embed the source material, store it in a vector database, then retrieve relevant chunks during calls and pass them into the LLM as contextual knowledge.

Notable design choices:

- Retrieval is automatic when a knowledge base is linked to an agent.
- Retrieval uses the transcript so far rather than only the latest user utterance.
- The retrieved content is appended to the model prompt under a dedicated knowledge-base section.
- Customers can tune the number of retrieved chunks and a similarity threshold.
- Retell recommends using knowledge as supporting context, not as agent instructions.
- Their best-practice guidance favors structured Markdown, focused sections, specific wording, and avoiding ambiguous pronouns.
- Retell exposes post-call retrieval traces through a `knowledge_base_retrieved_contents_url`, which helps teams debug what context was used for each response.

Research implication for Callified:

Retell treats RAG as part of the real-time call loop, not just as an admin upload feature. The strongest lesson is observability: if an answer is bad, operators need to see which chunks were retrieved for that turn.

Sources:

- https://docs.retellai.com/build/knowledge-base
- https://docs.retellai.com/api-references/get-call
- https://www.retellai.com/features/knowledge-base

### Vapi

Vapi supports RAG through query tools and knowledge bases. Their Query Tool approach is more explicit than Retell's automatic retrieval model: the assistant is instructed to call a named tool when it needs product, pricing, troubleshooting, or documentation knowledge.

Notable design choices:

- Files are uploaded first, then attached to a query tool.
- Multiple knowledge bases can be configured under one query tool.
- Each knowledge base has a description that helps route the assistant to the correct source.
- Vapi explicitly recommends adding system-prompt instructions telling the assistant when to call the query tool.
- Their documentation-agent example recommends 512-token chunks with 50-token overlap for documentation retrieval.
- Vapi highlights post-session analysis as a loop for improving retrieval quality over time.

Research implication for Callified:

For latency-sensitive sales calls, Callified can combine both styles: automatically retrieve for likely knowledge questions, but expose retrieval to the LLM as an explicit internal step when intent classification says the user asked for facts. This avoids retrieving on every filler, greeting, or interruption.

Sources:

- https://docs.vapi.ai/knowledge-base/using-query-tool
- https://docs.vapi.ai/assistants/examples/docs-agent

### Thoughtly

Thoughtly's Genius Knowledge Base uses RAG to augment voice agents with business-specific content. Their docs describe ingestion, AI processing into structured Q&A, agent integration, and call-time enhancement. Thoughtly also lets automations attach a specific Genius source to a call.

Notable design choices:

- Knowledge can be scoped to a single call or contact action.
- Selecting a single source can reduce hallucinations and control RAG database size.
- Contact attributes, metadata, tags, dispositions, and call results sit next to the call workflow.
- Knowledge maintenance includes add, edit, delete, and get-source-content actions.

Research implication for Callified:

Per-call knowledge scoping is especially relevant for outbound campaigns. A campaign should be able to say: "for this dial, use only this product KB, this pricing sheet, and this objection library."

Sources:

- https://docs.thoughtly.com/genius/getting-started
- https://docs.thoughtly.com/automations/actions

### Bland AI

Bland positions calls around pathways, tools, dynamic data, metadata, guardrails, and call logs. Their call logs expose transcript, audio, variables, citations, and per-turn decision data. Their send-call API supports dynamic request data, tools, guardrails, and citation schema IDs.

Notable design choices:

- Calls can include dynamic data at dispatch time.
- Tools can be used during the call for live API access.
- Call logs include context and citations for debugging agent behavior.
- Guardrails and citations are treated as part of the call configuration, not an afterthought.

Research implication for Callified:

RAG should not be isolated from call analytics. Retrieved chunks, citations, and the final outcome should be reviewable together so the team can answer: "Did better knowledge improve conversion, call duration, objections handled, or appointment booking?"

Sources:

- https://docs.bland.ai/tutorials/call-logs
- https://docs.bland.ai/api-v1/post/calls

---

## 3. Market pattern summary

| Pattern | Product examples | Why it matters |
|---|---|---|
| Upload URLs, PDFs, docs, text | Retell, Vapi, Thoughtly | Lets business teams update knowledge without engineering work |
| Chunk + embed + vector search | Retell, Vapi, Thoughtly | Keeps prompts short while preserving factual coverage |
| Runtime retrieval during calls | Retell, Thoughtly, Vapi | Gives the agent current context at the moment of response |
| Knowledge scoping by agent/campaign/call | Retell node-level KB, Vapi multiple KBs, Thoughtly per-call Genius | Prevents unrelated documents from polluting answers |
| Retrieval knobs | Retell chunks + similarity threshold | Lets teams balance recall, hallucination risk, latency, and prompt size |
| Explicit prompt/tool policy | Vapi | Reduces unnecessary searches and teaches the model when to rely on KB |
| Retrieval observability | Retell, Bland | Debugs wrong answers by showing what the model saw |
| Guardrails/refusal behavior | Retell, Bland | Prevents the agent from inventing unsupported facts |

---

## 4. Callified today

Callified already has a RAG microservice and knowledge admin APIs:

- `rag_service.py` exposes `/retrieve`, `/ingest`, and `/knowledge`.
- `rag.py` uses `sentence-transformers/all-MiniLM-L6-v2` and FAISS per org.
- `backend/internal/rag/client.go` calls the RAG service.
- `backend/internal/api/knowledge.go` lets admins upload and delete knowledge files.
- WhatsApp uses RAG through `backend/internal/wa/agent.go`.

The main gap: the live voice pipeline does not yet retrieve knowledge during calls. Product knowledge is currently assembled into the prompt, but uploaded RAG files are not part of the real-time voice answer loop.

This means Callified has the foundation, but not the most valuable voice use case.

---

## 5. Recommended Callified direction

### v1: Retrieval before selected voice turns

Add RAG retrieval inside the voice response path only when the caller asks a fact-seeking question.

Candidate trigger:

- User asks about pricing, plan limits, features, integrations, support, onboarding, compliance, contract terms, refund/cancellation, product comparison, or implementation details.
- User raises an objection that maps to a known product/campaign FAQ.
- LLM intent classification returns a knowledge intent.

Do not retrieve for:

- Greetings, silence, filler, acknowledgement, interruptions, language repair, hangup intent, or simple scheduling answers.

High-level flow:

```text
Customer speech
  -> STT transcript
  -> intent/knowledge trigger check
  -> optional RAG retrieve(org_id, campaign/product scope, query)
  -> append "## Retrieved Knowledge" to internal prompt
  -> LLM response
  -> TTS
  -> store retrieval trace with call turn
```

### v2: Campaign-scoped knowledge

Let campaigns select which knowledge files or product knowledge groups are eligible for retrieval.

Recommended hierarchy:

1. Organization knowledge: general company policies and brand facts.
2. Product knowledge: product docs, pricing, FAQs, integrations.
3. Campaign knowledge: offer-specific scripts, seasonal discounts, objection handling.
4. Lead/call memory: prior call summaries and known lead preferences.

Retrieval should prefer the narrowest available scope first. For example, a campaign-specific pricing document should beat a broad company brochure.

### v3: Retrieval traces and analytics

Persist retrieval metadata per call turn:

- `call_id`
- `lead_id`
- `org_id`
- user utterance or query text
- retrieved file IDs
- chunk IDs
- scores/distances
- generated response ID or timestamp
- whether the caller accepted, objected, converted, or dropped

Then expose this in call review:

- "Knowledge used" beside transcript turns.
- "No relevant KB found" warnings.
- A bad-answer debug view showing retrieved chunks.
- Conversion analysis by KB file, campaign, product, and objection type.

### v4: Quality controls

Add controls similar to market leaders:

- `top_k` per campaign, default 3.
- similarity threshold, default around 0.60 to 0.70 after calibration.
- max retrieved token budget.
- refusal rule: if retrieved knowledge is missing or weak, do not invent exact pricing, legal, compliance, or contractual claims.
- freshness metadata for uploaded docs.
- source priority: campaign > product > org.
- per-language retrieval query normalization for Hindi, Marathi, Tamil, Telugu, Bengali, Gujarati, Kannada, Malayalam, Punjabi, and English.

---

## 6. Implementation notes for this repo

### Backend voice path

Likely integration points:

- `backend/internal/wshandler/pipeline.go` for per-turn LLM response flow.
- `backend/internal/wshandler/handler.go` for call context and session state.
- `backend/internal/prompt/builder.go` for prompt sections and internal safety rules.
- `backend/internal/rag/client.go` for retrieving chunks.

The retrieved context should be injected as internal system data, not spoken verbatim unless it answers the caller's question naturally.

Suggested prompt section:

```text
## RETRIEVED KNOWLEDGE
Use this as factual grounding for the next customer-facing reply.
If this section is empty or irrelevant, do not invent exact facts.
Never mention document names, chunk IDs, embeddings, RAG, or retrieval.

<chunks here>
```

### RAG service improvements

Current `rag.py` uses fixed character chunks and returns one joined string. For voice-grade retrieval, consider:

- return structured chunks instead of one string;
- include `filename`, chunk index, score, and text;
- support similarity threshold filtering;
- normalize embeddings for cosine similarity or clearly calibrate FAISS L2 distances;
- use token-aware chunking instead of fixed character splitting;
- support PDF, TXT, Markdown, DOCX, CSV, and pasted text sources;
- store metadata with org, product, campaign, language, source type, uploaded_at, and version.

### Database additions

Possible new tables:

```sql
knowledge_chunks (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  org_id BIGINT NOT NULL,
  knowledge_file_id BIGINT NOT NULL,
  product_id BIGINT NULL,
  campaign_id BIGINT NULL,
  language VARCHAR(20) NULL,
  chunk_index INT NOT NULL,
  text MEDIUMTEXT NOT NULL,
  metadata JSON NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

call_rag_retrievals (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  org_id BIGINT NOT NULL,
  call_id VARCHAR(128) NOT NULL,
  lead_id BIGINT NULL,
  query_text TEXT NOT NULL,
  retrieved_chunks JSON NOT NULL,
  latency_ms INT NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

FAISS can remain the vector index in v1, while MySQL stores durable metadata and traceability.

---

## 7. Product experience

Admins should be able to:

- upload product PDFs, pricing docs, FAQ sheets, and objection scripts;
- attach knowledge files to products and campaigns;
- see indexing status and chunk count;
- test a query before using the KB in production;
- preview which chunks would be retrieved;
- inspect call transcripts with knowledge citations;
- disable a bad document without deleting historical call traces.

Agents/team leads should be able to:

- see whether knowledge helped or failed in a call review;
- flag an answer as unsupported or outdated;
- suggest a new FAQ from a real customer question.

---

## 8. Evaluation plan

Use a fixed benchmark set before enabling RAG in production:

- 50 product factual questions.
- 30 pricing/plan questions.
- 30 objection-handling questions.
- 20 unsupported questions where the correct behavior is refusal or fallback.
- 20 multilingual questions across the supported Indian languages.

Track:

- retrieval hit rate;
- answer groundedness;
- hallucination rate;
- average added latency;
- call turn time from final STT to first TTS audio;
- appointment or conversion rate;
- fallback/escalation rate;
- human QA score.

Ship target for v1:

- retrieval adds less than 150 ms p95 when the RAG service is warm;
- no retrieval on more than 50% of ordinary non-factual turns;
- hallucination rate on exact pricing/compliance questions below 2% in QA;
- every RAG-powered answer has a stored retrieval trace.

---

## 9. Risks

| Risk | Mitigation |
|---|---|
| Latency makes calls feel slow | Retrieve only for knowledge-intent turns; cache repeated queries per call; keep `top_k` low |
| Wrong chunks cause wrong answers | Use campaign/product scoping, thresholds, structured docs, and retrieval trace review |
| Agent quotes internal docs awkwardly | Use concise prompt rules: answer naturally, do not mention retrieval, do not read chunks verbatim |
| Old docs create stale answers | Add freshness metadata, admin status controls, and source versioning |
| Cross-tenant data leakage | Enforce `org_id` in every retrieval path and keep per-org indexes isolated |
| Hallucinated exact claims | Add refusal rule for missing or weak evidence |
| Multilingual query mismatch | Translate or normalize retrieval query while preserving response language |

---

## 10. Proposed roadmap

### Phase 1: Voice RAG MVP

- Add RAG retrieval to the voice response path for knowledge-intent turns.
- Return structured chunks from `rag_service.py`.
- Inject a compact retrieved-knowledge block into the LLM turn prompt.
- Add soft-fail behavior so calls continue if RAG is unavailable.
- Add unit tests for retrieval injection and no-retrieval paths.

### Phase 2: Campaign/product scoping

- Add product/campaign associations for knowledge files.
- Filter retrieval by `org_id` plus optional product/campaign scope.
- Add admin UI controls to attach KB files to campaigns.

### Phase 3: Observability

- Store per-turn retrieval traces.
- Show retrieved chunks in call review.
- Add QA flags for unsupported or stale answers.

### Phase 4: Retrieval quality

- Add token-aware chunking.
- Add similarity threshold tuning.
- Add query rewriting for multilingual calls.
- Add a benchmark suite and dashboards.

---

## 11. Recommended v1 acceptance criteria

- A caller can ask a product question during a live voice call and the agent answers using uploaded knowledge.
- A caller can ask an unsupported question and the agent avoids inventing facts.
- RAG failures are logged but do not drop the call.
- Retrieval is org-scoped and cannot return another tenant's chunks.
- Every retrieved answer can be audited after the call.
- Admin-uploaded knowledge remains usable by WhatsApp as before.

---

## 12. Bottom line

Competitors are converging on the same architecture: business-controlled knowledge sources, vector retrieval during the call, scoped context injection, strict refusal behavior, and post-call retrieval observability.

Callified already has the ingestion service, FAISS index, admin APIs, and WhatsApp integration. The highest-leverage R&D direction is to move RAG into the live voice turn loop, then make it scoped, observable, and measurable.

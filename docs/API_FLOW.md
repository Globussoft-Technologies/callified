# Callified API Flow — Login → Campaigns → Leads → AI Dial

This document describes the end-to-end API flow for authenticating with email/password, listing campaigns, adding a lead, triggering an AI call, and retrieving the call summary.

---

## Base URL

```
https://app.callified.ai/api
```

All authenticated requests must include the header:

```
Authorization: Bearer <access_token>
```

---

## Step 1 — Login (get token)

Obtain a JWT by sending the user's email and password.

**Endpoint**

```http
POST /api/auth/login
```

**Headers**

```
Content-Type: application/json
```

**Request body**

```json
{
  "email": "admin@example.com",
  "password": "your-password"
}
```

**Example response — 200 OK**

```json
{
  "access_token": "eyJhbGciOiJIUzI1NiIs...",
  "token_type": "bearer",
  "user": {
    "id": 1,
    "email": "admin@example.com",
    "full_name": "Admin User",
    "role": "Admin",
    "org_id": 1,
    "org_name": "Acme Inc"
  }
}
```

> Save the `access_token` value. It is required for every subsequent request.

### Postman

1. Create a new request.
2. Method: `POST`.
3. URL: `https://app.callified.ai/api/auth/login`.
4. Go to **Body** → select **raw** → select **JSON**.
5. Paste the request body above.
6. Click **Send**.
7. Copy the `access_token` from the response.

---

## Step 2 — List campaigns

List all campaigns for the authenticated user's organisation. Filter for `status: "active"` to see running campaigns.

**Endpoint**

```http
GET /api/campaigns
```

**Headers**

```
Authorization: Bearer <access_token>
```

**Example response — 200 OK**

```json
[
  {
    "id": 42,
    "org_id": 1,
    "product_id": 7,
    "name": "Summer Voice Campaign",
    "status": "active",
    "tts_provider": "elevenlabs",
    "tts_voice_id": "Rachel",
    "tts_language": "en",
    "lead_source": "Website",
    "channel": "voice",
    "product_name": "AI Sales Bot",
    "created_at": "2026-06-15 09:30:00",
    "stats": {
      "total": 150,
      "called": 89,
      "qualified": 34,
      "appointments": 12
    }
  }
]
```

### Postman

1. Create a new request.
2. Method: `GET`.
3. URL: `https://app.callified.ai/api/campaigns`.
4. Go to **Headers**.
5. Add key `Authorization` with value `Bearer <access_token>`.
6. Click **Send**.
7. Note the `id` of the campaign you want to use.

---

## Step 3 — Add a lead

Adding a lead is a two-step process: create the lead, then enrol it into a campaign.

### 3a. Create the lead

**Endpoint**

```http
POST /api/leads
```

**Headers**

```
Content-Type: application/json
Authorization: Bearer <access_token>
```

**Request body**

```json
{
  "first_name": "Rahul",
  "last_name": "Sharma",
  "phone": "011-1234-5678",
  "source": "Website",
  "interest": "AI Sales Bot"
}
```

> Phone formats accepted after the landline fix:
> - `9876543210` (10-digit mobile)
> - `01112345678` (landline with STD code)
> - `+919876543210` / `+91-11-1234-5678`

**Example response — 201 Created**

```json
{
  "id": 101
}
```

### Postman — create lead

1. Method: `POST`.
2. URL: `https://app.callified.ai/api/leads`.
3. **Headers**: `Content-Type: application/json`, `Authorization: Bearer <access_token>`.
4. **Body** → **raw** → **JSON**: paste the request body.
5. Click **Send** and copy the returned `id`.

---

### 3b. Enrol lead into campaign

**Endpoint**

```http
POST /api/campaigns/{campaign_id}/leads
```

Replace `{campaign_id}` with the campaign ID from Step 2.

**Headers**

```
Content-Type: application/json
Authorization: Bearer <access_token>
```

**Request body**

```json
{
  "lead_ids": [101]
}
```

**Example response — 200 OK**

```json
{
  "added": 1
}
```

### Postman — add to campaign

1. Method: `POST`.
2. URL: `https://app.callified.ai/api/campaigns/42/leads` (replace `42` with your campaign ID).
3. **Headers**: `Content-Type: application/json`, `Authorization: Bearer <access_token>`.
4. **Body** → **raw** → **JSON**: paste `{"lead_ids": [<lead_id>]}`.
5. Click **Send**.

---

## Step 4 — AI dial the lead

Trigger an outbound AI call to the lead using the campaign's voice settings.

**Endpoint**

```http
POST /api/campaigns/{campaign_id}/dial/{lead_id}
```

**Headers**

```
Authorization: Bearer <access_token>
```

**Request body**

None.

**Example response — 200 OK**

```json
{
  "dialed": true
}
```

### Postman — AI dial

1. Method: `POST`.
2. URL: `https://app.callified.ai/api/campaigns/42/dial/101` (replace IDs as needed).
3. **Headers**: `Authorization: Bearer <access_token>`.
4. **Body**: none.
5. Click **Send**.

---

## Step 5 — Retrieve call summary / transcript

After the call completes, fetch transcripts and the AI-generated review.

### 5a. All transcripts for a lead

**Endpoint**

```http
GET /api/leads/{lead_id}/transcripts
```

**Headers**

```
Authorization: Bearer <access_token>
```

**Example response — 200 OK**

```json
[
  {
    "id": 501,
    "lead_id": 101,
    "campaign_id": 42,
    "transcript": [
      {"role": "agent", "text": "Hello, this is Rachel from Acme."},
      {"role": "user", "text": "Hi, tell me more."}
    ],
    "recording_url": "/api/recordings/rec_2026_abc.wav",
    "tts_language": "en",
    "call_duration_s": 56.78,
    "created_at": "2026-07-02 14:22:10"
  }
]
```

### Postman

1. Method: `GET`.
2. URL: `https://app.callified.ai/api/leads/101/transcripts`.
3. **Headers**: `Authorization: Bearer <access_token>`.
4. Click **Send**.

---

### 5b. AI review for a transcript

Use the `id` from the transcript object above as `{transcript_id}`.

**Endpoint**

```http
GET /api/transcripts/{transcript_id}/review
```

**Example response — 200 OK**

```json
{
  "id": 55,
  "transcript_id": 501,
  "org_id": 1,
  "quality_score": 8.5,
  "sentiment": "positive",
  "appointment_booked": true,
  "failure_reason": "",
  "what_went_well": "Agent greeted clearly and asked discovery questions.",
  "what_went_wrong": "",
  "summary": "Customer showed strong interest and agreed to a demo.",
  "insights": "Use this opening for similar leads.",
  "prompt_improvement_suggestion": "Add pricing mention earlier.",
  "created_at": "2026-07-02 14:25:00"
}
```

### Postman

1. Method: `GET`.
2. URL: `https://app.callified.ai/api/transcripts/501/review`.
3. **Headers**: `Authorization: Bearer <access_token>`.
4. Click **Send**.

---

## Complete curl script

```bash
# 1. Login
TOKEN=$(curl -s -X POST https://app.callified.ai/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@example.com","password":"your-password"}' | jq -r '.access_token')

# 2. List campaigns
curl -s https://app.callified.ai/api/campaigns \
  -H "Authorization: Bearer $TOKEN" | jq '.[] | {id, name, status}'

# 3a. Create lead (Delhi landline example)
LEAD_ID=$(curl -s -X POST https://app.callified.ai/api/leads \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"first_name":"Rahul","last_name":"Sharma","phone":"011-1234-5678","source":"Website"}' | jq -r '.id')

# 3b. Add lead to campaign 42
curl -s -X POST https://app.callified.ai/api/campaigns/42/leads \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"lead_ids\":[$LEAD_ID]}"

# 4. AI dial
curl -s -X POST https://app.callified.ai/api/campaigns/42/dial/$LEAD_ID \
  -H "Authorization: Bearer $TOKEN"

# 5. Get transcripts
curl -s https://app.callified.ai/api/leads/$LEAD_ID/transcripts \
  -H "Authorization: Bearer $TOKEN"
```

---

## Postman collection tips

- Create a collection named **Callified Flow**.
- Add a collection variable `base_url` = `https://app.callified.ai/api`.
- Add a collection variable `token` (empty initially).
- In the **Login** request, add a **Tests** script to save the token automatically:

```js
var jsonData = pm.response.json();
pm.collectionVariables.set("token", jsonData.access_token);
```

- For all other requests, set the header:

```
Authorization: Bearer {{token}}
```

- Use Postman variables like `{{base_url}}`, `{{campaign_id}}`, and `{{lead_id}}` so you only need to update values in one place.

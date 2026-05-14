# WhatsApp Feature Guide

## What Works

### 1. Channel Setup (Config Modal)
Go to **WhatsApp Comms → ⚙️ Config**

| Field | Description |
|-------|-------------|
| Provider | Select **WaSender** (or Gupshup, Wati, Meta, etc.) |
| Personal Access Token | From wasenderapi.com → Profile |
| Source Phone | Your WhatsApp number (e.g. `7795740488`) |
| Webhook Secret | Optional — adds signature verification on inbound webhooks |
| Default Product | Product the AI will represent in all chats |
| Auto-Reply | Channel-level ON/OFF for AI responses |

> **Save Configuration** must be clicked for any change to take effect.

---

### 2. WhatsApp Session (WaSender QR Flow)
1. Open the **WhatsApp Session** panel (right side of the page)
2. Click **Connect** → a QR code appears
3. On your phone: WhatsApp → Linked Devices → Link a Device → scan QR
4. Status changes to **CONNECTED** and the linked number is shown
5. QR expires every ~45 seconds — page auto-refreshes it while you scan

**Disconnect & Re-scan** — use this if the session shows connected but messages aren't sending (phone unlinked the device without warning).

---

### 3. Inbox — Receiving Messages
- Inbound WhatsApp messages appear in the **WhatsApp Inbox** automatically
- Each contact gets its own conversation thread
- Unread indicator and last-message preview shown in the list
- Search by name or phone number

---

### 4. AI Auto-Reply
AI replies are sent automatically when a customer messages in.

**Two levels of control:**

| Level | Where | How |
|-------|-------|-----|
| Channel (all conversations) | Config modal → Auto-Reply toggle | Must click **Save Configuration** |
| Per conversation | Chat header → AI Auto-Reply toggle | Takes effect immediately |

**Mute a conversation** — click `⋮` menu on any thread → **🔇 Mute**. AI skips that thread but messages still save. Unmute to resume.

---

### 5. Product-Based AI Replies
The AI uses your product's details to answer customer questions.

**Setup:**
1. Go to **Products** tab → add your product
2. Enter the website URL → click **🔍 Scrape Website** (AI extracts product info)
3. Add **Manual Notes** (pricing, USPs, objection handling)
4. Click **✨ Auto-Generate Persona & Call Flow** → saves agent persona
5. Go to **WhatsApp → Config** → select the product as **Default Product** → **Save**

**What the AI uses:**
- Agent Persona → AI's identity and tone
- Scraped Info → product knowledge to answer questions
- Manual Notes → extra context (pricing, FAQs)
- Conversation Guide → how to steer the chat

---

### 6. Manual Send
Type a message in the chat input box → **Send**. Manual messages are saved to the conversation history and sent via the configured provider.

---

## How to Change the Webhook URL in WaSender

### Via WaSender Dashboard
1. Log in at [wasenderapi.com](https://wasenderapi.com)
2. Go to **Sessions** → open your session
3. Find the **Webhook URL** field
4. Paste the new URL: `https://YOUR_DOMAIN/wa/webhook/wasender`
5. Save

### Via Command Line (faster)

**Step 1 — Get your session ID:**
```bash
curl -s https://www.wasenderapi.com/api/whatsapp-sessions \
  -H "Authorization: Bearer <YOUR_PAT>" | python3 -m json.tool
```
Note the `"id"` value from the response.

**Step 2 — Update the webhook URL:**
```bash
curl -s -X PUT https://www.wasenderapi.com/api/whatsapp-sessions/<SESSION_ID> \
  -H "Authorization: Bearer <YOUR_PAT>" \
  -H "Content-Type: application/json" \
  -d '{"webhook_url": "https://testgo2.callified.ai/wa/webhook/wasender"}'
```

**Step 3 — Verify:**
```bash
curl -s https://www.wasenderapi.com/api/whatsapp-sessions/<SESSION_ID> \
  -H "Authorization: Bearer <YOUR_PAT>" | python3 -m json.tool | grep webhook
```
Expected output:
```
"webhook_url": "https://testgo2.callified.ai/wa/webhook/wasender"
```

---

## Webhook URLs by Provider

| Provider | Webhook URL |
|----------|-------------|
| WaSender | `https://testgo2.callified.ai/wa/webhook/wasender` |
| Gupshup | `https://testgo2.callified.ai/wa/webhook/gupshup` |
| Wati | `https://testgo2.callified.ai/wa/webhook/wati` |
| Meta (Cloud API) | `https://testgo2.callified.ai/wa/webhook/meta` |
| Interakt | `https://testgo2.callified.ai/wa/webhook/interakt` |
| AiSensei | `https://testgo2.callified.ai/wa/webhook/aisensei` |

> The **Copy** button in the Config modal always shows the correct URL for the currently selected provider.

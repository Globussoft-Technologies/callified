package prompt

import "strings"

// langExtraRules holds per-language rule blocks ported from the Python
// prompt_builder.py changes on `main` (commits d5b4420 — forward-signal
// detection — and 4aa3fa3 — rejection detection, direct-question answering,
// Devanagari numbered-list ban). Languages not listed here contribute no
// extras; the universal CORE RULES in buildDefaultPrompt cover them.
//
// The token "{{LEAD}}" is replaced with the lead's first name at use time.
var langExtraRules = map[string]string{
	"hi": `## RULE — FORWARD SIGNAL (LISTEN FIRST)
- Customer bole 'बढ़ाओ', 'हां बताओ', 'बोलो', 'आगे बताओ', 'batao', 'haan batao', 'yes tell me', 'haan badhao' → yeh AFFIRMATIVE hai, customer sun raha hai aur aage badhna chahta hai.
- Wrong: Customer: 'हां बढ़ाओ' → AI: 'Abhi bhi interest hai ismein?' (Dobara qualify kiya! Customer already haan bol chuka hai!)
- Right: Customer: 'हां बढ़ाओ' → AI: 'Badhiya! Aaj ya kal kab free hain?'
- Wrong: Customer: 'हां बताओ' → AI: 'haan ji boliye?' (Loop! Agla step shuru karo.)
- Right: Customer: 'हां बताओ' → AI: 'Toh {{LEAD}} ji, aapko call aayega details ke liye, kab free hain?'

## RULE 5B — REJECTION DETECTION
- Customer bole 'बस', 'रहने दो', 'नहीं चाहिए', 'मत करो', 'ठीक है बाद में', 'बाद में बात करते हैं', 'अभी नहीं' → turant warm acknowledge karo + [HANGUP]. Dobara pitch mat karo.
- Wrong: Customer: 'बस रहने दो' → AI: 'Arey ji, ek baar toh dekhenge na...' (Phir se pitch! Galat!)
- Right: Customer: 'बस रहने दो' → AI: 'Bilkul {{LEAD}} ji, koi baat nahi. Jab chahen tab call karna. Thank you! [HANGUP]'
- Wrong: Customer: 'नहीं चाहिए' → AI: 'Dekhiye, ek baar senior se milte hain...' (Pitch phir se! Galat!)
- Right: Customer: 'नहीं चाहिए' → AI: 'Theek hai, no problem. Thank you! [HANGUP]'

## RULE 5C — DIRECT QUESTION ANSWERING
- Customer specific sawaal pooche (kya, kaise, kitna, faida, kahan, price, benefit) → seedha 1-line jawab do. Kabhi same pitch block dobara mat chalao.
- Wrong: Customer: 'mujhe kya faida hoga?' → AI: 'Humari company mein teen tarah ki services hain...' (Pitch recycle! Galat!)
- Right: Customer: 'mujhe kya faida hoga?' → AI: 'Aapko benefit milta hai: details senior share karenge. Aur jaanna chahenge?'
- Ek direct sawaal = ek direct jawab (1 sentence) + 1 follow-up question. Tabhi agle topic par jao.

## RULE — NO DEVANAGARI NUMBERED LISTS
- '१.', '२.', '३.' kabhi mat likho. TTS robo ki tarah padhta hai.
- Wrong: 'Fayde: १. kamai २. rozgaar ३. service' (Devanagari list! Galat!)
- Right: 'Teen fayde hain: kamai, rozgaar, aur service. Kaunsa jaanna chahenge?'
`,

	"bn": `## RULE — REJECTION DETECTION (Bengali)
- Customer bole 'থাক', 'পরে করব', 'এখন না', 'দরকার নেই' → sange acknowledge karo + [HANGUP]. Abar pitch koro na.
- Wrong: Customer: 'থাক থাক' → AI: 'Arey, ekbar dekhle bhalo lagbe...' (Pitch phir se! Bhul!)
- Right: Customer: 'থাক' → AI: 'ঠিক আছে {{LEAD}} ji, koi baat nahi. Jokhon chaiben tokhon call korun. Thank you! [HANGUP]'
`,

	"mr": `## RULE — REJECTION DETECTION (Marathi)
- Customer bolla 'राहू दे', 'नको', 'नंतर बघतो', 'आत्ता नाही' → lagech acknowledge kar + [HANGUP]. Punha pitch karu nakos.
- Wrong: Customer: 'राहू दे' → AI: 'Arey, ekda bagha na...' (Punha pitch! Galat!)
- Right: Customer: 'राहू दे' → AI: 'Theek ahe {{LEAD}} ji, koi harkat nahi. Dhanyavad! [HANGUP]'

## RULE — FORWARD SIGNAL (Marathi)
- Customer bolla 'हो सांग', 'पुढे सांग', 'बोल', 'सांग' → AFFIRMATIVE. Seedha next step la ja.
- Wrong: Customer: 'हो सांग' → AI: 'Tumhala interest aahe ka?' (Punha qualify! Customer ho bolla aahe!)
- Right: Customer: 'हो सांग' → AI: 'Toh {{LEAD}} ji, aaj kinva udya kadhi free aahat?'

## RULE — DIRECT QUESTION (Marathi)
- Customer specific prashna vicharala (kay, kase, kiti, faayda, kuthe, price) → 1-line jawab + 1 follow-up. Pitch block punha chalvu nakos.
- Right: Customer: 'maza faayda kay?' → AI: 'Faayda miltoy: details senior sanga til. Aankhin jaanun ghyayche aahe ka?'

## RULE — NO DEVANAGARI NUMBERED LISTS (Marathi)
- '१.', '२.', '३.' kadhi lihu nakos. TTS robo sarkhe vachte.
- Right: 'Teen fayde: kamai, training, ani service. Konta jaanun ghyaycha?'
`,

	"en": `## RULE — FORWARD SIGNAL (English)
- Customer says 'yes tell me', 'go on', 'go ahead', 'continue', 'sure tell me', 'ok ok' → AFFIRMATIVE. Move directly to the next step. Don't re-qualify.
- Wrong: Customer: 'yes go on' → AI: 'Are you still interested in this?' (Re-qualified! Customer already said yes!)
- Right: Customer: 'yes go on' → AI: 'Great! Are you free today or tomorrow for a quick call?'

## RULE — REJECTION DETECTION (English)
- Customer says 'no thanks', 'not interested', 'leave it', 'later', 'maybe later', 'not now', 'don't need it', 'I'll pass' → warmly acknowledge + [HANGUP]. Do NOT re-pitch.
- Wrong: Customer: 'not interested' → AI: 'Sir, just hear me out for a minute...' (Re-pitch! Wrong!)
- Right: Customer: 'not interested' → AI: 'No problem {{LEAD}}, thanks for your time. Have a good day! [HANGUP]'

## RULE — DIRECT QUESTION (English)
- Customer asks a specific question (what, how, how much, benefit, where, price) → answer in ONE sentence + ask one follow-up. Never replay the full pitch block.
- Wrong: Customer: 'what's the benefit for me?' → AI: 'We have three types of services...' (Pitch recycle! Wrong!)
- Right: Customer: 'what's the benefit for me?' → AI: 'You get the benefit explained in our product knowledge. Want to know more?'
`,

	"gu": `## RULE — FORWARD SIGNAL (Gujarati)
- Customer bole 'હા કહો', 'આગળ બોલો', 'કહો', 'haan kaho' → AFFIRMATIVE. Sidha next step par jao. Punarayi qualify na karo.
- Wrong: Customer: 'હા કહો' → AI: 'tamne interest chhe ne?' (Punarayi qualify! Customer ha bolyo chhe!)
- Right: Customer: 'હા કહો' → AI: 'Toh {{LEAD}} ji, aaje ke kale kyare free chho?'

## RULE — REJECTION DETECTION (Gujarati)
- Customer bole 'નહીં જોઈએ', 'રહેવા દો', 'નથી જોઈતું', 'પછી', 'હમણાં નહીં' → warm acknowledge + [HANGUP]. Punarayi pitch na karo.
- Wrong: Customer: 'નહીં જોઈએ' → AI: 'arey ek vakhat to jovo na...' (Pitch fari! Khotu!)
- Right: Customer: 'નહીં જોઈએ' → AI: 'thik chhe {{LEAD}} ji, vandho nathi. Aabhar! [HANGUP]'

## RULE — DIRECT QUESTION (Gujarati)
- Customer specific prashna puchhe (shu, kevi rite, ketlu, fayda, kya, price) → 1-line jawab + 1 follow-up. Pitch block fari na chalavo.
- Right: Customer: 'maro fayda shu chhe?' → AI: 'Faydo malshe: details senior khashe. Vadhare jaanavu chhe?'

## RULE — NO GUJARATI NUMBERED LISTS
- '૧.', '૨.', '૩.' kadi na lakho. TTS robo jevu vanche chhe.
`,

	"pa": `## RULE — FORWARD SIGNAL (Punjabi)
- Customer kahe 'ਹਾਂ ਦੱਸੋ', 'ਅੱਗੇ ਦੱਸੋ', 'ਬੋਲੋ', 'haan dasso' → AFFIRMATIVE. Sidha next step te jaao. Dobara qualify na karo.
- Wrong: Customer: 'ਹਾਂ ਦੱਸੋ' → AI: 'tuhanu interest hai?' (Dobara qualify! Customer haan keh chukya!)
- Right: Customer: 'ਹਾਂ ਦੱਸੋ' → AI: 'Te {{LEAD}} ji, aaj jaan kal kado free ho?'

## RULE — REJECTION DETECTION (Punjabi)
- Customer kahe 'ਨਹੀਂ ਚਾਹੀਦਾ', 'ਛੱਡ ਦਿਓ', 'ਬਾਅਦ ਵਿੱਚ', 'ਹੁਣ ਨਹੀਂ', 'ਨਹੀਂ ਲੋੜ' → warm acknowledge + [HANGUP]. Dobara pitch na karo.
- Wrong: Customer: 'ਨਹੀਂ ਚਾਹੀਦਾ' → AI: 'ek vaari sun lao ji...' (Pitch phir! Galat!)
- Right: Customer: 'ਨਹੀਂ ਚਾਹੀਦਾ' → AI: 'Theek hai {{LEAD}} ji, koi gal nahi. Dhanvaad! [HANGUP]'

## RULE — DIRECT QUESTION (Punjabi)
- Customer specific sawaal puchhe (ki, kiven, kinna, faida, kithe, price) → 1-line jawab + 1 follow-up. Pitch block dobara na chalao.
- Right: Customer: 'mera faida ki hai?' → AI: 'Faida milda hai: details senior dasange. Hor jaanna chahuoge?'

## RULE — NO GURMUKHI NUMBERED LISTS
- '੧.', '੨.', '੩.' kadi na likho. TTS robo vargi padhda hai.
`,

	"ta": `## RULE — FORWARD SIGNAL (Tamil)
- Customer 'ஆமா சொல்லுங்க', 'மேலே சொல்லுங்க', 'சொல்', 'aamaa sollu' sonna → AFFIRMATIVE. Next step ku po. Marubadi qualify pannaadhe.
- Wrong: Customer: 'ஆமா சொல்லுங்க' → AI: 'unga ku interest irukka?' (Marubadi qualify! Customer aama sonnaaru!)
- Right: Customer: 'ஆமா சொல்லுங்க' → AI: '{{LEAD}} sir, indru illa naalai eppa free ah irukinga?'

## RULE — REJECTION DETECTION (Tamil)
- Customer 'வேண்டாம்', 'விட்டு விடுங்க', 'அப்புறமா', 'இப்போ வேணாம்', 'தேவையில்ல' sonna → warm acknowledge + [HANGUP]. Marubadi pitch pannaadhe.
- Wrong: Customer: 'வேண்டாம்' → AI: 'oru thadava paarunga sir...' (Pitch marubadi! Thappu!)
- Right: Customer: 'வேண்டாம்' → AI: 'sari {{LEAD}} sir, parava illa. Nandri! [HANGUP]'

## RULE — DIRECT QUESTION (Tamil)
- Customer specific kelvi ketta (enna, eppadi, evvalavu, payan, enga, price) → 1-line badhil + 1 follow-up. Pitch block marubadi va vidaadhe.
- Right: Customer: 'enaku enna payan?' → AI: 'Payan kidaikum: details senior solvar. Innum theriya venuma?'

## RULE — NO TAMIL NUMBERED LISTS
- '௧.', '௨.', '௩.' epodhum ezhudaadhe. TTS robo madhiri padikkum.
`,

	"te": `## RULE — FORWARD SIGNAL (Telugu)
- Customer 'అవును చెప్పండి', 'ముందుకు చెప్పండి', 'చెప్పు', 'avunu cheppandi' annappudu → AFFIRMATIVE. Direct ga next step ki vellandi. Marala qualify cheyyakandi.
- Wrong: Customer: 'అవును చెప్పండి' → AI: 'meeku interest unda?' (Marala qualify! Customer avunu annaru!)
- Right: Customer: 'అవును చెప్పండి' → AI: '{{LEAD}} garu, ee roju leda repu eppudu free ga unnaru?'

## RULE — REJECTION DETECTION (Telugu)
- Customer 'వద్దు', 'వదిలేయండి', 'తరువాత', 'ఇప్పుడు కాదు', 'avasaram ledu' annappudu → warm acknowledge + [HANGUP]. Marala pitch cheyyakandi.
- Wrong: Customer: 'వద్దు' → AI: 'okasari vinandi sir...' (Pitch marala! Tappu!)
- Right: Customer: 'వద్దు' → AI: 'sare {{LEAD}} garu, parledu. Dhanyavadalu! [HANGUP]'

## RULE — DIRECT QUESTION (Telugu)
- Customer specific prashna adigithe (emi, ela, entha, prayojanam, ekkada, price) → 1-line samaadhanam + 1 follow-up. Pitch block marala cheyyakandi.
- Right: Customer: 'naaku prayojanam emiti?' → AI: 'Prayojanam untundi: details senior cheptharu. Inka teluskovaalanukuntunnara?'

## RULE — NO TELUGU NUMBERED LISTS
- '౧.', '౨.', '౩.' eppudu rayakandi. TTS robo laaga chaduvuthundi.
`,

	"kn": `## RULE — FORWARD SIGNAL (Kannada)
- Customer 'ಹೌದು ಹೇಳಿ', 'ಮುಂದೆ ಹೇಳಿ', 'ಹೇಳಿ', 'haudu heli' anda kudale → AFFIRMATIVE. Neravaagi next step ge hogi. Matte qualify maadabedi.
- Wrong: Customer: 'ಹೌದು ಹೇಳಿ' → AI: 'nimage interest ide na?' (Matte qualify! Customer houdU andidaare!)
- Right: Customer: 'ಹೌದು ಹೇಳಿ' → AI: '{{LEAD}} sir, ivattu illa naale yavaaga free iddiri?'

## RULE — REJECTION DETECTION (Kannada)
- Customer 'ಬೇಡ', 'ಬಿಡಿ', 'ನಂತರ', 'ಈಗ ಬೇಡ', 'agatya illa' anda kudale → warm acknowledge + [HANGUP]. Matte pitch maadabedi.
- Wrong: Customer: 'ಬೇಡ' → AI: 'ondu sala kelidhare olleyadu sir...' (Pitch matte! Tappu!)
- Right: Customer: 'ಬೇಡ' → AI: 'sari {{LEAD}} sir, parvagilla. Dhanyavaada! [HANGUP]'

## RULE — DIRECT QUESTION (Kannada)
- Customer specific prashne kelidare (yenu, hege, eshtu, prayojana, yelli, price) → 1-line uttara + 1 follow-up. Pitch block matte maadabedi.
- Right: Customer: 'nange prayojana enu?' → AI: 'Prayojana ide: details senior heLthare. Mattashtu tilkollabeke?'

## RULE — NO KANNADA NUMBERED LISTS
- '೧.', '೨.', '೩.' yendigu bareyabedi. TTS robo thara odutte.
`,

	"ml": `## RULE — FORWARD SIGNAL (Malayalam)
- Customer 'അതേ പറയൂ', 'പറയൂ', 'തുടരൂ', 'aathe parayoo' parayumbol → AFFIRMATIVE. Neeraavi next step lekku po. Veendum qualify cheyyaruth.
- Wrong: Customer: 'അതേ പറയൂ' → AI: 'ningalkku interest undo?' (Veendum qualify! Customer aathe paranju!)
- Right: Customer: 'അതേ പറയൂ' → AI: '{{LEAD}} sir, innu allenkil naale eppozhaanu free?'

## RULE — REJECTION DETECTION (Malayalam)
- Customer 'വേണ്ട', 'വിടൂ', 'പിന്നീട്', 'ഇപ്പോൾ വേണ്ട', 'aavashyam illa' parayumbol → warm acknowledge + [HANGUP]. Veendum pitch cheyyaruth.
- Wrong: Customer: 'വേണ്ട' → AI: 'oru pravashyam kelkku sir...' (Pitch veendum! Thett!)
- Right: Customer: 'വേണ്ട' → AI: 'sheri {{LEAD}} sir, kuzhappam illa. Nanni! [HANGUP]'

## RULE — DIRECT QUESTION (Malayalam)
- Customer specific chodyam chodichaal (enthu, engane, ethraa, gunam, evide, price) → 1-line uttaram + 1 follow-up. Pitch block veendum cheyyaruth.
- Right: Customer: 'enikku gunam enthaanu?' → AI: 'Gunam undu: details senior parayum. Koodutalum ariyaano?'

## RULE — NO MALAYALAM NUMBERED LISTS
- '൧.', '൨.', '൩.' onnum ezhuthaaruth. TTS robo pole vaayikkum.
`,
}

// extraRulesFor returns the per-language rule extras with the lead's first
// name interpolated. Empty string when the language has no extras configured.
func extraRulesFor(language, leadFirst string) string {
	tmpl, ok := langExtraRules[language]
	if !ok || tmpl == "" {
		return ""
	}
	if leadFirst == "" {
		leadFirst = "the lead"
	}
	return strings.ReplaceAll(tmpl, "{{LEAD}}", leadFirst)
}

package repository

// Code generated from the course content data files. DO NOT EDIT BY HAND.
//
// Source of truth (skillofied-app/src/components/courses/modules/):
//   JavaCourse/JavaCourseData.ts
//   TestingCourse/TestingCourseData.ts
//   GolangCourse/GolangCourseData.ts
//   FullStackCourse/FullstackCourseData.ts
//   SqlCourse/SqlCourseData.ts
//   FrontendCourse/Module*.tsx
//   MarketingCourses/SeoCourseData.ts, MarketingCourses/DigitalMarketingCourseData.ts
//
// Regenerate with: node scripts/gen-quiz-seed.js
//
// Module IDs are namespaced by course ("java-m1", "golang-m1", "frontend-m1")
// because every course numbers its modules from m1 and quiz_keys is keyed on
// (module_id, question_id). The prefix must match what the course's
// ModuleQuiz call site submits.

type quizKey struct {
	moduleID   string
	questionID int
	correctAns string
}

// quizAnswerKeys holds 1172 answer keys across 180 modules.
var quizAnswerKeys = []quizKey{
	// dm-m1 (6 questions)
	{"dm-m1", 1, "It is a conversion problem, so more traffic will not help"},
	{"dm-m1", 2, "Customer interviews, support messages, reviews and search queries"},
	{"dm-m1", 3, "Whether the split changes what you would actually say or do"},
	{"dm-m1", 4, "Search captures demand that already exists; social has to create it"},
	{"dm-m1", 5, "Reach borrowed from a platform can be reduced or withdrawn without notice"},
	{"dm-m1", 6, "The second, on cost per order: about GBP 12.86 against GBP 25"},

	// dm-m2 (6 questions)
	{"dm-m2", 1, "Inconsistency rather than content quality"},
	{"dm-m2", 2, "The blank-page problem — you never decide what to post from nothing"},
	{"dm-m2", 3, "Keeping people on the platform"},
	{"dm-m2", 4, "It is demoted by platforms and erodes audience trust"},
	{"dm-m2", 5, "It escalates — the deletion becomes the story"},
	{"dm-m2", 6, "Saves and comments show stronger interest than likes, so this is your best-performing post"},

	// dm-m3 (6 questions)
	{"dm-m3", 1, "So the ad copy and landing page can closely match the query"},
	{"dm-m3", 2, "In queries that should have been excluded by negative keywords"},
	{"dm-m3", 3, "The creative"},
	{"dm-m3", 4, "Gross margin"},
	{"dm-m3", 5, "Automated strategies need conversion volume to learn from"},
	{"dm-m3", 6, "Plan with your own shop data — platform conversions are modelled estimates"},

	// dm-m4 (6 questions)
	{"dm-m4", 1, "No algorithm decides whether your message reaches the people on your list"},
	{"dm-m4", 2, "Solving one specific problem with immediate value"},
	{"dm-m4", 3, "Spam complaints and bounces that damage delivery for your genuine subscribers too"},
	{"dm-m4", 4, "Abandoned cart"},
	{"dm-m4", 5, "Most of the recoverable orders come back from the reminder alone, so an early code gives away margin and teaches people to abandon baskets"},
	{"dm-m4", 6, "Opens are inflated by privacy features such as Apple Mail Privacy Protection, so judge this campaign on its clicks and orders"},

	// dm-m5 (6 questions)
	{"dm-m5", 1, "Wrong traffic — a targeting or message-match problem"},
	{"dm-m5", 2, "Stopping the test as soon as the result looks significant"},
	{"dm-m5", 3, "Use qualitative evidence such as recordings and support questions, then judge reasoned changes on trend"},
	{"dm-m5", 4, "An element that is broken, slow or gives no feedback"},
	{"dm-m5", 5, "Masking sensitive inputs, disclosing it and honouring consent — the recordings are personal data"},
	{"dm-m5", 6, "It would need roughly 80,000 visitors per variant at 95% confidence and 80% power, so the test can never finish — improve the page by other means"},

	// dm-m6 (6 questions)
	{"dm-m6", 1, "40%, because in GA4 bounce rate is simply the opposite of engagement rate"},
	{"dm-m6", 2, "No event has been marked as a key event in the admin settings"},
	{"dm-m6", 3, "Direct is the leftover bucket, so a large share usually points to untagged links such as email, QR codes or PDFs"},
	{"dm-m6", 4, "Each tagged internal click starts a new session and re-credits the campaign, destroying the record of the real traffic source"},
	{"dm-m6", 5, "The number of orders recorded in the business's own order system, with the platform figures shown separately and labelled"},
	{"dm-m6", 6, "Pause paid social in one region for a fixed period and compare total orders against the rest of the business"},

	// dm-m7 (6 questions)
	{"dm-m7", 1, "Cut it into several social posts, an email, a short video and reusable sales replies over the following weeks"},
	{"dm-m7", 2, "Very little that you can reuse, because too many variables changed to identify the cause"},
	{"dm-m7", 3, "Watch that exact moment to find what caused it — often a logo, a pause or an early sales pitch — and remove it"},
	{"dm-m7", 4, "The smaller one, because audience fit and demonstrated engagement predict results far better than follower count"},
	{"dm-m7", 5, "Clear and prominent disclosure such as #ad at the start of the caption, because gifted content with brand control is advertising"},
	{"dm-m7", 6, "Send only to those who consented or who fall within the narrow soft opt-in, identify the sender, and include a working opt-out in every message"},

	// dm-m8 (6 questions)
	{"dm-m8", 1, "No: that is personal data in a public tool, so anonymise and aggregate first, or use an invented example"},
	{"dm-m8", 2, "Delete both figures, supply your real numbers, and add no benchmark you cannot source"},
	{"dm-m8", 3, "Because UK advertising rules say ads must not mislead or exaggerate a product, and that applies however the image was made"},
	{"dm-m8", 4, "Producing eight different subject lines for you to shortlist and test"},
	{"dm-m8", 5, "Spot-check a random sample against the raw text, verify the quotes word for word, and search the raw text for terms you expect"},
	{"dm-m8", 6, "Refuse: fake reviews are illegal in the UK and breach platform rules, and propose a way to collect real reviews instead"},

	// dm-m9 (6 questions)
	{"dm-m9", 1, "It has no number, no date and no baseline, so the plan cannot fail and cannot be judged"},
	{"dm-m9", 2, "Because tracking added afterwards cannot recover data you already lost, and success is easier to define honestly before anyone is defending a result"},
	{"dm-m9", 3, "State plainly that the goal was missed — 24 against a target of 40 — then explain what worked, where it broke, and what you would do next"},
	{"dm-m9", 4, "The raw numbers, baseline and timeframe — \"up 200%\" may mean 2 to 6, and omitting them makes the reader distrust the rest"},
	{"dm-m9", 5, "Genuine volunteer marketing for a local charity, written up with baseline and results"},
	{"dm-m9", 6, "Several warning signs — no team, mismatched expectations, unpaid trial work, no stated measure of success — so go in with direct questions before accepting"},

	// dm-m1-assignment (6 questions)
	{"dm-m1-assignment", 1, "Fix the booking step first: 36 of 900 is 4 per cent, so doubling traffic doubles the cost while the same leak loses the same share"},
	{"dm-m1-assignment", 2, "None of those numbers answers the question, because none of them is tied to a stated goal or to orders and enquiries"},
	{"dm-m1-assignment", 3, "None of those details would change the wording, the offer or the channel choice, so the persona cannot influence any decision"},
	{"dm-m1-assignment", 4, "Paid search has little existing demand to capture, so demand must be created first through social, content and partnerships"},
	{"dm-m1-assignment", 5, "Two separate sources for one newsletter, and internal clicks wrongly recorded as new visits from outside the site"},
	{"dm-m1-assignment", 6, "CAC is GBP 20 and LTV is about GBP 94.50, because LTV must be calculated on gross profit rather than on total sales"},

	// dm-m2-assignment (6 questions)
	{"dm-m2-assignment", 1, "Keep a cadence she can hold on a busy week; a schedule that collapses in a month costs more reach than two steady posts"},
	{"dm-m2-assignment", 2, "The Reel is doing a different job: it is building reach and interest that later promotional posts depend on, and it produced three times the clicks of the promotion anyway"},
	{"dm-m2-assignment", 3, "Acknowledge the specific double charge publicly, give a named next step and a timeframe, then move to a private message for her account details"},
	{"dm-m2-assignment", 4, "Correct it once, plainly and pleasantly, with the evidence, leave the comment up, and stop replying after that"},
	{"dm-m2-assignment", 5, "Ask her, get her agreement in writing, and check she is happy with the specific photo and wording before it goes out"},
	{"dm-m2-assignment", 6, "Growth at that speed usually means bought or incentivised followers, which will not buy plants and will lower the engagement rate that drives reach"},

	// dm-m3-assignment (6 questions)
	{"dm-m3-assignment", 1, "Wait: targets need enough conversion volume to learn from, and a target far below the actual CPA will starve the campaign of traffic"},
	{"dm-m3-assignment", 2, "Exact match includes close variants, which covers the same meaning, plurals and common alternative spellings"},
	{"dm-m3-assignment", 3, "Both platforms are claiming credit for some of the same sign-ups, so the totals overlap; plan from the gym's own figure of 24"},
	{"dm-m3-assignment", 4, "Shorten the audience window, exclude recent buyers and add more creative variations, rather than widening the audience to 180 days"},
	{"dm-m3-assignment", 5, "The two audiences overlap, so the ad sets are competing for the same people and pushing up costs"},
	{"dm-m3-assignment", 6, "The tags must not fire until the visitor gives consent, and the banner must let them genuinely refuse, under the UK PECR and UK GDPR rules"},

	// dm-m4-assignment (6 questions)
	{"dm-m4-assignment", 1, "The address was collected for prescription reminders with no marketing opt-out offered, so the soft opt-in does not cleanly apply — ask for consent before marketing to them"},
	{"dm-m4-assignment", 2, "Move to sending from the gym’s own domain with SPF, DKIM and DMARC set up, add one-click unsubscribe, and bring the complaint rate under 0.3%"},
	{"dm-m4-assignment", 3, "A pre-ticked box is not valid consent, and prize entrants are a poor fit anyway — email them once to ask them to opt in, and only keep those who do"},
	{"dm-m4-assignment", 4, "Treat B as the better performer and change the tool to decide on clicks, because privacy features inflate opens and clicks reflect real interest"},
	{"dm-m4-assignment", 5, "The cart workflow has no working exit condition — set \"exit on order placed\" and re-test it by placing a real order"},
	{"dm-m4-assignment", 6, "Sending to a large unengaged segment raises bounces and complaints and lowers engagement signals, so the campaign is more likely to land in spam for the 2,200 people who do care"},

	// dm-m5-assignment (6 questions)
	{"dm-m5-assignment", 1, "At this traffic no colour test could ever reach a trustworthy result — watch session recordings and fix the clearest problems instead"},
	{"dm-m5-assignment", 2, "Keep running to the planned end date — results fluctuate, and stopping at the first good-looking moment manufactures winners that do not exist"},
	{"dm-m5-assignment", 3, "Load the tool only after analytics consent, keep input masking on so typed text is never captured, describe it in the privacy policy and set a retention period"},
	{"dm-m5-assignment", 4, "The message match — make the page headline and button offer the free survey the advert promised"},
	{"dm-m5-assignment", 5, "With counts this small the gap is well within normal random variation, so the correct conclusion is that no reliable difference has been shown"},
	{"dm-m5-assignment", 6, "Friction inside the form — check which field people stop on using form analytics or recordings, then remove or reword it"},

	// dm-m6-assignment (6 questions)
	{"dm-m6-assignment", 1, "Sessions are visits, not customers; the comparison is also affected by an extra weekend and a consent-banner change, so the rise should be reported with both caveats"},
	{"dm-m6-assignment", 2, "Whether the event is firing more than once per submission, for example on a thank-you page that people reload or because the tag was installed twice"},
	{"dm-m6-assignment", 3, "The rows cannot be merged retrospectively; report the combined figure manually with a note, then agree a lowercase hyphenated convention and a shared link log"},
	{"dm-m6-assignment", 4, "A redirect between the short link and the final page is stripping the query string, so the UTMs never arrive"},
	{"dm-m6-assignment", 5, "Explain that the gap is mostly view-through counting and longer windows rather than the model, so switching will not make them match and will break month-to-month comparability"},
	{"dm-m6-assignment", 6, "Most of the gap is view-through and window length rather than a tracking fault, so the two tools are behaving as expected"},

	// dm-m7-assignment (6 questions)
	{"dm-m7-assignment", 1, "It should become a piece of published content, since the answer has already been written eleven times"},
	{"dm-m7-assignment", 2, "A 400-word post is too thin to yield six genuinely different pieces, so one deeper monthly asset would repurpose far better"},
	{"dm-m7-assignment", 3, "Cut the logo and the wide shot, open mid-action, and add on-screen text so the video works with the sound off"},
	{"dm-m7-assignment", 4, "Creative fatigue — the same audience has seen it too often, so a refreshed creative is needed"},
	{"dm-m7-assignment", 5, "Refuse, because the CAP Code requires paid content to be obviously identifiable as advertising and both brand and creator are responsible"},
	{"dm-m7-assignment", 6, "Pause the affiliate's account while you investigate, check for self-referrals and cookie stuffing, and apply the clawback terms in the agreement"},

	// dm-m8-assignment (6 questions)
	{"dm-m8-assignment", 1, "Use AI for structure and plain-English drafting only, supply the approved claims yourself, and have a qualified person approve the final wording"},
	{"dm-m8-assignment", 2, "Delete the whole comparison, because it came from nowhere you can trace"},
	{"dm-m8-assignment", 3, "Adding the business, the audience, the real facts it may use, and a ban on inventing anything"},
	{"dm-m8-assignment", 4, "AI content is acceptable where it is helpful and people-first; scaled content abuse — mass pages made mainly to game rankings — is the spam violation"},
	{"dm-m8-assignment", 5, "Someone will paste customer or confidential data into a public tool because nobody told them not to"},
	{"dm-m8-assignment", 6, "\"Below are 400 anonymised reviews. Group the complaints into themes, give the number in each, and quote three word for word. Do not create a theme that is not in the text.\""},

	// dm-m9-assignment (6 questions)
	{"dm-m9-assignment", 1, "Work backwards: 80 sales at 2% means roughly 4,000 relevant visits, then check whether the budget can realistically buy that"},
	{"dm-m9-assignment", 2, "An office manager asked to sort a birthday cake by Thursday who does not want to look careless"},
	{"dm-m9-assignment", 3, "It lets you move money to whichever channel is actually performing once real data arrives"},
	{"dm-m9-assignment", 4, "Replace tasks with decisions and reasons, and add baseline numbers, results and a timeframe"},
	{"dm-m9-assignment", 5, "State the result plainly, explain what you diagnosed and why, and say specifically what you would do differently"},
	{"dm-m9-assignment", 6, "Agency gives broad, fast experience across many clients; in-house gives depth on one business and lets you see results over months"},

	// frontend-m1 (4 questions)
	{"frontend-m1", 1, "A. HyperText Markup Language"},
	{"frontend-m1", 2, "B. CSS"},
	{"frontend-m1", 3, "C. JavaScript"},
	{"frontend-m1", 4, "D. Database"},

	// frontend-m2 (6 questions)
	{"frontend-m2", 1, "A. alt"},
	{"frontend-m2", 2, "B. <h1>"},
	{"frontend-m2", 3, "C. <a>"},
	{"frontend-m2", 4, "D. <ul>"},
	{"frontend-m2", 5, "A. <br>"},
	{"frontend-m2", 6, "B. <input type=\"checkbox\">"},

	// frontend-m3 (5 questions)
	{"frontend-m3", 1, "A. Cascading Style Sheets"},
	{"frontend-m3", 2, "B. color"},
	{"frontend-m3", 3, "C. padding"},
	{"frontend-m3", 4, "D. Removes element from the page completely"},
	{"frontend-m3", 5, "A. ID (#main)"},

	// frontend-m4 (5 questions)
	{"frontend-m4", 1, "A. display: flex"},
	{"frontend-m4", 2, "B. Horizontal alignment (main axis)"},
	{"frontend-m4", 3, "C. grid-template-columns"},
	{"frontend-m4", 4, "D. Wraps items to next line when no space"},
	{"frontend-m4", 5, "A. Fraction"},

	// frontend-m5 (4 questions)
	{"frontend-m5", 1, "A. Design for mobile first, then scale up"},
	{"frontend-m5", 2, "B. @media"},
	{"frontend-m5", 3, "C. Controls how page scales on mobile devices"},
	{"frontend-m5", 4, "D. vw"},

	// frontend-m6 (5 questions)
	{"frontend-m6", 1, "A. var"},
	{"frontend-m6", 2, "B. \"object\""},
	{"frontend-m6", 3, "C. ==="},
	{"frontend-m6", 4, "D. push()"},
	{"frontend-m6", 5, "A. Block scoped"},

	// frontend-m7 (5 questions)
	{"frontend-m7", 1, "A. querySelectorAll()"},
	{"frontend-m7", 2, "B. e.preventDefault()"},
	{"frontend-m7", 3, "C. User's browser"},
	{"frontend-m7", 4, "D. elem.classList.add(\"name\")"},
	{"frontend-m7", 5, "A. Session storage clears when tab/browser is closed"},

	// frontend-m8 (5 questions)
	{"frontend-m8", 1, "A. It throws a TypeError"},
	{"frontend-m8", 2, "B. Arrow functions inherit \"this\" from the parent scope"},
	{"frontend-m8", 3, "C. Backticks ``"},
	{"frontend-m8", 4, "D. B = [...A]"},
	{"frontend-m8", 5, "A. extends"},

	// frontend-m9 (5 questions)
	{"frontend-m9", 1, "A. Page UI freezes while task executes"},
	{"frontend-m9", 2, "B. Pending, Resolved, Rejected"},
	{"frontend-m9", 3, "C. using try...catch blocks"},
	{"frontend-m9", 4, "D. response.json()"},
	{"frontend-m9", 5, "A. async"},

	// frontend-m10 (5 questions)
	{"frontend-m10", 1, "A. git init"},
	{"frontend-m10", 2, "B. git add index.html"},
	{"frontend-m10", 3, "C. GitHub is a hosting service for Git repositories"},
	{"frontend-m10", 4, "D. git checkout feature-login"},
	{"frontend-m10", 5, "A. git log"},

	// frontend-m11 (5 questions)
	{"frontend-m11", 1, "A. It updates only changed parts of the UI, improving performance"},
	{"frontend-m11", 2, "B. class"},
	{"frontend-m11", 3, "C. Inside a single parent container (e.g. <div> or Fragment)"},
	{"frontend-m11", 4, "D. Read-only (immutable) from inside the receiving component"},
	{"frontend-m11", 5, "A. npm create vite@latest"},

	// frontend-m12 (5 questions)
	{"frontend-m12", 1, "A. Props are read-only (passed down), State is local (managed inside component)"},
	{"frontend-m12", 2, "B. The current state and a function to update it"},
	{"frontend-m12", 3, "C. The form input value is driven by React state"},
	{"frontend-m12", 4, "D. To help React identify which items have changed, been added, or been removed"},
	{"frontend-m12", 5, "A. &&"},

	// frontend-m13 (5 questions)
	{"frontend-m13", 1, "A. Empty dependency array []"},
	{"frontend-m13", 2, "B. To create mutable references that persist across renders without triggering a re-render"},
	{"frontend-m13", 3, "C. useMemo memoizes computed values, useCallback memoizes function instances"},
	{"frontend-m13", 4, "D. prefix with the word \"use\" (e.g. useFetch)"},
	{"frontend-m13", 5, "A. Return a function inside the effect body"},

	// frontend-m14 (5 questions)
	{"frontend-m14", 1, "A. Page changes happen client-side in the browser without reloading the page"},
	{"frontend-m14", 2, "B. <Link>"},
	{"frontend-m14", 3, "C. useParams() hook"},
	{"frontend-m14", 4, "D. <Outlet />"},
	{"frontend-m14", 5, "A. useNavigate() hook"},

	// frontend-m15 (5 questions)
	{"frontend-m15", 1, "A. useEffect"},
	{"frontend-m15", 2, "B. Fetch doesn't throw errors on 404 or 500 status codes"},
	{"frontend-m15", 3, "C. POST"},
	{"frontend-m15", 4, "D. LocalStorage or HTTP-only cookies"},
	{"frontend-m15", 5, "A. Communicating request status to the user to improve UX"},

	// frontend-m16 (5 questions)
	{"frontend-m16", 1, "A. Passing props through multiple nested components that don't need them, just to reach a deep child component"},
	{"frontend-m16", 2, "B. Dispatching an action object"},
	{"frontend-m16", 3, "C. Zustand is lightweight and does not require Provider wrapper setups"},
	{"frontend-m16", 4, "D. React.lazy() & Suspense"},
	{"frontend-m16", 5, "A. Provider & Consumer"},

	// frontend-m17 (5 questions)
	{"frontend-m17", 1, "A. Transpiles, minifies, and tree-shakes code files into a compact build bundle"},
	{"frontend-m17", 2, "B. VITE_"},
	{"frontend-m17", 3, "C. A & CNAME"},
	{"frontend-m17", 4, "D. Between 50 to 160 characters"},
	{"frontend-m17", 5, "A. Vercel"},

	// frontend-assessment (4 questions)
	{"frontend-assessment", 1, "A. Margin, Border, Padding, Content"},
	{"frontend-assessment", 2, "B. Data deletion timeline"},
	{"frontend-assessment", 3, "C. It skips rendering updates"},
	{"frontend-assessment", 4, "D. Linking local repositories to remote GitHub locations"},

	// fullstack-m1 (1 questions)
	{"fullstack-m1", 1, "article"},

	// fullstack-m2 (1 questions)
	{"fullstack-m2", 1, "PUT"},

	// genai-m1 (8 questions)
	{"genai-m1", 1, "It lets one process wait on many I/O-bound calls at once"},
	{"genai-m1", 2, "metadata: dict = field(default_factory=dict)"},
	{"genai-m1", 3, "Parse inside a try/except and handle failure explicitly"},
	{"genai-m1", 4, "A set"},
	{"genai-m1", 5, "Nothing — they document the expected shape for readers and tools"},
	{"genai-m1", 6, "It stalls every other coroutine on the event loop"},
	{"genai-m1", 7, "Provider SDKs move fast and can change response shapes between minor versions"},
	{"genai-m1", 8, "It contains swapping providers, adding fallback and logging cost to one place"},

	// genai-m2 (8 questions)
	{"genai-m2", 1, "It may be predicting \"not fraud\" every time"},
	{"genai-m2", 2, "Overfitting"},
	{"genai-m2", 3, "The next token in the text acts as the label"},
	{"genai-m2", 4, "A single final estimate of real-world performance"},
	{"genai-m2", 5, "Recall"},
	{"genai-m2", 6, "Word order and synonymy"},
	{"genai-m2", 7, "Overfitted the prompt to a tiny sample"},
	{"genai-m2", 8, "0.0 for precision by convention, rather than crashing on the division"},

	// genai-m3 (8 questions)
	{"genai-m3", 1, "System prompt, history, context and the response together"},
	{"genai-m3", 2, "At or near 0"},
	{"genai-m3", 3, "Attention compute grows quadratically with sequence length"},
	{"genai-m3", 4, "Produces the most plausible continuation given its weights"},
	{"genai-m3", 5, "How sharply probability concentrates on the leading tokens"},
	{"genai-m3", 6, "It restricts sampling to the smallest set of tokens whose probabilities sum to p"},
	{"genai-m3", 7, "Output tokens"},
	{"genai-m3", 8, "Only that the output is well-formed"},

	// genai-m4 (8 questions)
	{"genai-m4", 1, "It preserves the instruction/data boundary that injection attacks exploit"},
	{"genai-m4", 2, "Malicious instructions hidden in retrieved documents or web pages"},
	{"genai-m4", 3, "Retry once with the validation error included in the prompt"},
	{"genai-m4", 4, "Only by schema validation after parsing"},
	{"genai-m4", 5, "Tone and framing"},
	{"genai-m4", 6, "Insert it literally by substituting in a single pass"},
	{"genai-m4", 7, "So reasoning can be logged for debugging while users see only the conclusion"},
	{"genai-m4", 8, "Their tokens are billed on every single request"},

	// genai-m5 (8 questions)
	{"genai-m5", 1, "Direction carries meaning while magnitude often reflects incidental length"},
	{"genai-m5", 2, "Re-embed the entire corpus"},
	{"genai-m5", 3, "Inside the query itself"},
	{"genai-m5", 4, "Cosine matches, dot product rewards the longer one"},
	{"genai-m5", 5, "A little recall"},
	{"genai-m5", 6, "Each chunk stays a coherent unit that can answer a question"},
	{"genai-m5", 7, "Hybrid search blending vector similarity with keyword matching"},
	{"genai-m5", 8, "It is what makes verifiable citations possible later"},

	// genai-m6 (8 questions)
	{"genai-m6", 1, "Whether retrieval surfaced the correct chunk at all"},
	{"genai-m6", 2, "Whether the correct chunk appeared in the top k results"},
	{"genai-m6", 3, "A fabricated citation — a detectable hallucination"},
	{"genai-m6", 4, "Re-running an interrupted ingestion job does not create duplicate chunks"},
	{"genai-m6", 5, "It reads the query and the document together rather than encoding each separately"},
	{"genai-m6", 6, "Decline to answer"},
	{"genai-m6", 7, "At the beginning and end, where models attend most reliably"},
	{"genai-m6", 8, "A golden set of 50–100 real questions with expected sources"},

	// genai-m7 (8 questions)
	{"genai-m7", 1, "Your application code, after validating the arguments"},
	{"genai-m7", 2, "A confused agent will loop indefinitely and keep billing you"},
	{"genai-m7", 3, "Return the error to the model as an observation so it can recover"},
	{"genai-m7", 4, "A loop that takes actions with real side effects"},
	{"genai-m7", 5, "The model returns plain text instead of a tool call"},
	{"genai-m7", 6, "Tool-selection accuracy degrades as the list grows"},
	{"genai-m7", 7, "Not giving it a delete tool at all"},
	{"genai-m7", 8, "Summarise the evicted turns and keep the summary"},

	// genai-m8 (8 questions)
	{"genai-m8", 1, "Streaming TTS from the first sentence rather than the full response"},
	{"genai-m8", 2, "Converting the chart to text destroys the visual information you needed"},
	{"genai-m8", 3, "Text inside an image can carry a prompt injection"},
	{"genai-m8", 4, "Every stage leaves an artefact you can inspect, log and evaluate"},
	{"genai-m8", 5, "Retrieval silently searches for something that does not exist"},
	{"genai-m8", 6, "About 800ms to one second"},
	{"genai-m8", 7, "The one contributing the most milliseconds"},
	{"genai-m8", 8, "Cancelling in-flight generation and synthesis immediately"},

	// genai-m9 (8 questions)
	{"genai-m9", 1, "RAG"},
	{"genai-m9", 2, "A small number of inserted low-rank matrices, with the base frozen"},
	{"genai-m9", 3, "The best prompt on the original model, on the same held-out set"},
	{"genai-m9", 4, "To be inconsistent"},
	{"genai-m9", 5, "Continue the text, possibly with more questions"},
	{"genai-m9", 6, "You are nudging an already instruction-tuned model toward one output shape"},
	{"genai-m9", 7, "Quantisation of the frozen base to reduce memory"},
	{"genai-m9", 8, "Catastrophic forgetting of abilities outside the tuned task"},

	// genai-m10 (8 questions)
	{"genai-m10", 1, "Without it, retries synchronise and re-create the overload"},
	{"genai-m10", 2, "Time to first token"},
	{"genai-m10", 3, "A request validation error"},
	{"genai-m10", 4, "Cancellation must propagate to the provider call or you keep paying for tokens"},
	{"genai-m10", 5, "Spend is rarely even — attribution finds the tenant or feature that dominates"},
	{"genai-m10", 6, "Serving a stale answer to a subtly different question"},
	{"genai-m10", 7, "Show the retrieved passages without a generated summary"},
	{"genai-m10", 8, "It leaves headroom for spikes and other clients sharing the quota"},

	// genai-m11 (8 questions)
	{"genai-m11", 1, "Responses return 200 OK while being wrong"},
	{"genai-m11", 2, "Validate the judge against human ratings"},
	{"genai-m11", 3, "The model may have started inventing answers instead of declining"},
	{"genai-m11", 4, "A few very slow requests barely move the average but ruin those users' experience"},
	{"genai-m11", 5, "Prompt version"},
	{"genai-m11", 6, "Before the request leaves your network"},
	{"genai-m11", 7, "A number in the answer that appears nowhere in the context"},
	{"genai-m11", 8, "Checks applied around the model, on input and output"},

	// genai-m12 (8 questions)
	{"genai-m12", 1, "Find the underlying problem and who the users are"},
	{"genai-m12", 2, "At ingestion and inside the retrieval query, from day one"},
	{"genai-m12", 3, "Hosted model APIs are ruled out; you need open-weight models running locally"},
	{"genai-m12", 4, "What does a wrong answer cost you?"},
	{"genai-m12", 5, "Ingestion and data extraction"},
	{"genai-m12", 6, "Retrieval cannot filter on it, so access must be re-checked against the source system"},
	{"genai-m12", 7, "One document type and one team, shipped in weeks and measurable"},
	{"genai-m12", 8, "Naming it first is what makes your other numbers credible"},

	// genai-m1-assignment (5 questions)
	{"genai-m1-assignment", 1, "Strip a surrounding code fence, then parse inside try/except and handle failure explicitly"},
	{"genai-m1-assignment", 2, "Walk the results once with a dict keyed on id, keeping the higher score"},
	{"genai-m1-assignment", 3, "The call runs normally; hints are not enforced unless a tool like mypy checks them"},
	{"genai-m1-assignment", 4, "Run them concurrently with asyncio.gather, since the time is spent waiting on I/O"},
	{"genai-m1-assignment", 5, "A lock file or pinned versions installed into a virtual environment"},

	// genai-m10-assignment (5 questions)
	{"genai-m10-assignment", 1, "3,200ms"},
	{"genai-m10-assignment", 2, "They retry in synchronised waves and re-create the overload; jitter spreads them out"},
	{"genai-m10-assignment", 3, "Attribution by tenant and feature is what locates the cost driver"},
	{"genai-m10-assignment", 4, "A 503 from the provider"},
	{"genai-m10-assignment", 5, "The threshold was too permissive, so a subtly different question was served a wrong cached answer"},

	// genai-m11-assignment (5 questions)
	{"genai-m11-assignment", 1, "One request took 9 seconds — the average of 1,986ms describes no real user"},
	{"genai-m11-assignment", 2, "Deterministic checks — cheap, repeatable and suitable to run in CI on every change"},
	{"genai-m11-assignment", 3, "Tracking a quality metric daily against a rolling baseline and alerting on a sustained drop"},
	{"genai-m11-assignment", 4, "Pinpointing which change coincided with the regression instead of guessing"},
	{"genai-m11-assignment", 5, "Check that numbers in the answer appear in the context"},

	// genai-m12-assignment (5 questions)
	{"genai-m12-assignment", 1, "About 174 days — 50 million pages at 200 per minute"},
	{"genai-m12-assignment", 2, "Exclude the chunk — the user shares no group with its ACL"},
	{"genai-m12-assignment", 3, "A narrow, measurable assistant for the highest-volume ticket type, with the baseline time recorded"},
	{"genai-m12-assignment", 4, "State the trade-off plainly: on-premises open-weight models, with the quality gap measured on their own tasks"},
	{"genai-m12-assignment", 5, "Name the error rate and how it is being measured and reduced — it makes the other results credible"},

	// genai-m2-assignment (5 questions)
	{"genai-m2-assignment", 1, "Precision 0.90, recall 0.75"},
	{"genai-m2-assignment", 2, "Epoch 4 — the point before the model began memorising the training data"},
	{"genai-m2-assignment", 3, "A simple keyword baseline that the model must beat to justify itself"},
	{"genai-m2-assignment", 4, "It has leaked into model selection and no longer gives an unbiased estimate"},
	{"genai-m2-assignment", 5, "The cancer screen — a missed case costs far more than a false alarm"},

	// genai-m3-assignment (5 questions)
	{"genai-m3-assignment", 1, "About 6k tokens — output shares the window with everything else"},
	{"genai-m3-assignment", 2, "It rises toward 1.0 — lower temperature sharpens the distribution"},
	{"genai-m3-assignment", 3, "The corpus is about 120M tokens — over a hundred times larger than the window, and cost scales with every request"},
	{"genai-m3-assignment", 4, "To avoid numeric overflow; subtracting a constant does not change the resulting probabilities"},
	{"genai-m3-assignment", 5, "The event is after its training data and it generated a plausible continuation anyway"},

	// genai-m4-assignment (5 questions)
	{"genai-m4-assignment", 1, "Refuse to render and name the missing placeholder"},
	{"genai-m4-assignment", 2, "Treat retrieved content as untrusted data, keep it out of the instruction channel, and limit what tools the model can reach"},
	{"genai-m4-assignment", 3, "Validating the parsed object against the schema and retrying with the specific error"},
	{"genai-m4-assignment", 4, "Version prompts and run them against a fixed evaluation set before release"},
	{"genai-m4-assignment", 5, "Real inputs that resemble production, including awkward edge cases, with correct outputs"},

	// genai-m5-assignment (5 questions)
	{"genai-m5-assignment", 1, "0 — the vectors are orthogonal"},
	{"genai-m5-assignment", 2, "1 — cosine ignores magnitude and only compares direction"},
	{"genai-m5-assignment", 3, "Split on headings first, then on paragraph boundaries, and store the heading with each chunk"},
	{"genai-m5-assignment", 4, "Filtering after ranking starves results and risks leakage; the tenant filter belongs inside the query"},
	{"genai-m5-assignment", 5, "Dot product rewards vector magnitude, which often tracks length rather than relevance"},

	// genai-m6-assignment (5 questions)
	{"genai-m6-assignment", 1, "Add overlap between adjacent chunks, then confirm with recall@k that retrieval improved"},
	{"genai-m6-assignment", 2, "Mostly in generation — retrieval is finding the right material, so look at prompt, context order and grounding"},
	{"genai-m6-assignment", 3, "Flag doc-9 as a fabricated citation, since it can be verified against what was supplied"},
	{"genai-m6-assignment", 4, "Deterministic ids from a content hash, so re-ingesting upserts rather than inserts"},
	{"genai-m6-assignment", 5, "A cross-encoder reranker that scores each query–document pair together"},

	// genai-m7-assignment (5 questions)
	{"genai-m7-assignment", 1, "A maximum step count that ends the run with a clear error"},
	{"genai-m7-assignment", 2, "Reject the call — the allow-list decides, not the existence of a schema"},
	{"genai-m7-assignment", 3, "Reject it without executing and return the validation error to the model as an observation"},
	{"genai-m7-assignment", 4, "Require human approval above a threshold and keep an audit trail of every call"},
	{"genai-m7-assignment", 5, "When one agent's toolset is too large to select reliably and the subtasks are genuinely separable"},

	// genai-m8-assignment (5 questions)
	{"genai-m8-assignment", 1, "The LLM first-token stage — at 700ms it is the largest share of the 400ms overrun"},
	{"genai-m8-assignment", 2, "As soon as \"Your order.\" is complete, without waiting for the rest"},
	{"genai-m8-assignment", 3, "A cascade — transcribe, then LLM — so each stage leaves an inspectable artefact"},
	{"genai-m8-assignment", 4, "Conversion to text can silently lose information; a native vision model reads the image directly"},
	{"genai-m8-assignment", 5, "Barge-in handling that cancels in-flight generation and playback when speech is detected"},

	// genai-m9-assignment (5 questions)
	{"genai-m9-assignment", 1, "Leakage — validation scores become inflated and stop reflecting unseen data"},
	{"genai-m9-assignment", 2, "Before the training run, by a validation pass that counts and reports problems"},
	{"genai-m9-assignment", 3, "RAG over the price list — prices change and must be current and citable"},
	{"genai-m9-assignment", 4, "65,536 — two matrices of 4,096 × 8"},
	{"genai-m9-assignment", 5, "The base model with your best prompt and few-shot examples on the same held-out set"},

	// golang-m1 (6 questions)
	{"golang-m1", 1, "go run"},
	{"golang-m1", 2, "Google"},
	{"golang-m1", 3, "A compiled executable binary"},
	{"golang-m1", 4, "main() in package main"},
	{"golang-m1", 5, "Simple grammar and explicit, acyclic imports"},
	{"golang-m1", 6, "Compilation fails with an error"},

	// golang-m2 (6 questions)
	{"golang-m2", 1, "%T"},
	{"golang-m2", 2, "0"},
	{"golang-m2", 3, "Only inside functions"},
	{"golang-m2", 4, "float64(n)"},
	{"golang-m2", 5, "A value fixed at compile time"},
	{"golang-m2", 6, "3"},

	// golang-m3 (6 questions)
	{"golang-m3", 1, "for"},
	{"golang-m3", 2, "No; you must write fallthrough"},
	{"golang-m3", 3, "for condition { }"},
	{"golang-m3", 4, "Skips to the next iteration"},
	{"golang-m3", 5, "Breaking out of an outer loop"},
	{"golang-m3", 6, "if v := f(); v > 0 { }"},

	// golang-m4 (6 questions)
	{"golang-m4", 1, "Using _ blank identifier"},
	{"golang-m4", 2, "A []int slice"},
	{"golang-m4", 3, "f(s...)"},
	{"golang-m4", 4, "A function value that captures surrounding variables"},
	{"golang-m4", 5, "By value; a copy is passed"},
	{"golang-m4", 6, "Without one the call stack grows until it overflows"},

	// golang-m5 (6 questions)
	{"golang-m5", 1, "append"},
	{"golang-m5", 2, "An array has a fixed length that is part of its type"},
	{"golang-m5", 3, "The zero value of the value type"},
	{"golang-m5", 4, "v, ok := m[key]"},
	{"golang-m5", 5, "No; it is deliberately unspecified"},
	{"golang-m5", 6, "They share the same underlying array"},

	// golang-m6 (6 questions)
	{"golang-m6", 1, "Capitalize the first letter"},
	{"golang-m6", 2, "When it modifies the struct or the struct is large"},
	{"golang-m6", 3, "Promotion of the embedded type's fields and methods"},
	{"golang-m6", 4, "Uses key name and omits the field when empty"},
	{"golang-m6", 5, "It is skipped"},
	{"golang-m6", 6, "Small types combine without fragile class hierarchies"},

	// golang-m7 (6 questions)
	{"golang-m7", 1, "None (implicit)"},
	{"golang-m7", 2, "A value of any type"},
	{"golang-m7", 3, "It panics"},
	{"golang-m7", 4, "s, ok := v.(string)"},
	{"golang-m7", 5, "It has every method the interface declares"},
	{"golang-m7", 6, "Branching on a value's dynamic type"},

	// golang-m8 (6 questions)
	{"golang-m8", 1, "LIFO (Last In First Out)"},
	{"golang-m8", 2, "As the last return value"},
	{"golang-m8", 3, "Regains control in a deferred function during a panic"},
	{"golang-m8", 4, "fmt.Errorf(\"context: %w\", err)"},
	{"golang-m8", 5, "Rarely; only for unrecoverable programmer errors"},
	{"golang-m8", 6, "When the defer statement runs"},

	// golang-m9 (6 questions)
	{"golang-m9", 1, "go.mod"},
	{"golang-m9", 2, "Adds missing and removes unused module requirements"},
	{"golang-m9", 3, "Starting its name with an uppercase letter"},
	{"golang-m9", 4, "Checksums of module dependencies"},
	{"golang-m9", 5, "Packages rooted at internal's parent"},
	{"golang-m9", 6, "One"},

	// golang-m10 (6 questions)
	{"golang-m10", 1, "encoding/json"},
	{"golang-m10", 2, "It closes the file on every return path"},
	{"golang-m10", 3, "os.ReadFile"},
	{"golang-m10", 4, "bufio.Scanner"},
	{"golang-m10", 5, "encoding/csv"},
	{"golang-m10", 6, "Returns nil without error"},

	// golang-m11 (6 questions)
	{"golang-m11", 1, "go test -race"},
	{"golang-m11", 2, "The sender blocks until a receiver is ready"},
	{"golang-m11", 3, "Waiting for a set of goroutines to finish"},
	{"golang-m11", 4, "Proceeds with whichever is ready first"},
	{"golang-m11", 5, "So callers can cancel it or set a deadline"},
	{"golang-m11", 6, "The number of jobs processed concurrently"},

	// golang-m12 (6 questions)
	{"golang-m12", 1, "_test.go"},
	{"golang-m12", 2, "func TestXxx(t *testing.T)"},
	{"golang-m12", 3, "Many cases share one test body"},
	{"golang-m12", 4, "go test -bench=."},
	{"golang-m12", 5, "Stops the current test immediately"},
	{"golang-m12", 6, "Tests can substitute a fake implementation"},

	// golang-m13 (6 questions)
	{"golang-m13", 1, "net/http"},
	{"golang-m13", 2, "func(w http.ResponseWriter, r *http.Request)"},
	{"golang-m13", 3, "http.ListenAndServe(\":8080\", mux)"},
	{"golang-m13", 4, "A function that wraps and returns an http.Handler"},
	{"golang-m13", 5, "201 Created"},
	{"golang-m13", 6, "Invalid shapes fail early with a clear error"},

	// golang-m14 (6 questions)
	{"golang-m14", 1, "To prevent SQL Injection"},
	{"golang-m14", 2, "It is a pooled handle meant to be long-lived"},
	{"golang-m14", 3, "defer rows.Close() and check rows.Err()"},
	{"golang-m14", 4, "Commit or Rollback on a sql.Tx"},
	{"golang-m14", 5, "Versioned, repeatable schema changes across environments"},
	{"golang-m14", 6, "Data-access code from business logic"},

	// golang-m15 (6 questions)
	{"golang-m15", 1, "binding"},
	{"golang-m15", 2, "gin.Default()"},
	{"golang-m15", 3, "c.Param(\"id\")"},
	{"golang-m15", 4, "c.ShouldBindJSON(&req)"},
	{"golang-m15", 5, "HTTP handling stays thin and business logic is testable"},
	{"golang-m15", 6, "Create a router group and call Use on it"},

	// golang-m16 (6 questions)
	{"golang-m16", 1, "bcrypt"},
	{"golang-m16", 2, "Hashing is one-way, so a leak does not reveal them"},
	{"golang-m16", 3, "The signature"},
	{"golang-m16", 4, "It limits the damage if an access token is stolen"},
	{"golang-m16", 5, "Which browser origins may call your API"},
	{"golang-m16", 6, "Who you are versus what you may do"},

	// golang-m17 (6 questions)
	{"golang-m17", 1, "Protocol Buffers"},
	{"golang-m17", 2, "Independent deployment at the cost of network complexity"},
	{"golang-m17", 3, "It decouples them and absorbs spikes asynchronously"},
	{"golang-m17", 4, "A .proto file"},
	{"golang-m17", 5, "Static binaries need almost nothing else to run"},
	{"golang-m17", 6, "Finding the current address of a service instance"},

	// golang-m18 (6 questions)
	{"golang-m18", 1, "Generates a single self-contained binary"},
	{"golang-m18", 2, "GOOS=linux GOARCH=amd64 go build"},
	{"golang-m18", 3, "The same build runs in every environment without code changes"},
	{"golang-m18", 4, "A small runtime image without the build toolchain"},
	{"golang-m18", 5, "Running several related containers together"},
	{"golang-m18", 6, "Run tests and fail fast on errors"},

	// java-m1 (20 questions)
	{"java-m1", 1, "A. James Gosling"},
	{"java-m1", 2, "B. Oak"},
	{"java-m1", 3, "C. Write Once, Run Anywhere (WORA)"},
	{"java-m1", 4, "D. JRE and development tools like 'javac'"},
	{"java-m1", 5, "A. Java Virtual Machine (JVM)"},
	{"java-m1", 6, "B. .class"},
	{"java-m1", 7, "C. Java Bytecode is platform-independent, but the JVM is platform-dependent."},
	{"java-m1", 8, "D. Garbage Collection"},
	{"java-m1", 9, "A. public static void main(String[] args)"},
	{"java-m1", 10, "B. Welcome.java"},
	{"java-m1", 11, "C. Providing an all-in-one text editor, build automation tool, and debugger"},
	{"java-m1", 12, "D. javac Test.java"},
	{"java-m1", 13, "A. The method does not return any value when it finishes executing."},
	{"java-m1", 14, "B. Strong type checking and exception handling mechanisms"},
	{"java-m1", 15, "C. java Demo"},
	{"java-m1", 16, "D. PATH"},
	{"java-m1", 17, "A. System.out.println(\"Hello World\");"},
	{"java-m1", 18, "B. To enable the JVM to call the method without creating an instance of the class first"},
	{"java-m1", 19, "C. The JDK is a superset that includes the complete JRE plus development tools."},
	{"java-m1", 20, "D. They mark the beginning of a single-line text comment."},

	// java-m2 (20 questions)
	{"java-m2", 1, "C. _variable$5"},
	{"java-m2", 2, "D. String"},
	{"java-m2", 3, "A. Widening casting happens automatically; narrowing casting must be done manually."},
	{"java-m2", 4, "B. /** Documentation comment */"},
	{"java-m2", 5, "C. next() reads input up to the next whitespace delimiter, while nextLine() reads the entire line until a newline character."},
	{"java-m2", 6, "D. %f"},
	{"java-m2", 7, "A. They skip evaluating the second condition if the overall result is already determined by the first condition."},
	{"java-m2", 8, "B. a=7, b=12"},
	{"java-m2", 9, "C. 2.0"},
	{"java-m2", 10, "D. 9"},
	{"java-m2", 11, "A. Output: 1020"},
	{"java-m2", 12, "B. 30 Output"},
	{"java-m2", 13, "C. -1"},
	{"java-m2", 14, "D. 10"},
	{"java-m2", 15, "A. -128"},
	{"java-m2", 16, "B. 5.68"},
	{"java-m2", 17, "C. n1=20, n2=10"},
	{"java-m2", 18, "D. 30"},
	{"java-m2", 19, "A. true"},
	{"java-m2", 20, "B. B"},

	// java-m3 (20 questions)
	{"java-m3", 1, "D. double"},
	{"java-m3", 2, "A. The program falls through, executing subsequent case blocks sequentially until a break or the end of the switch is encountered."},
	{"java-m3", 3, "B. The inner 'if' condition is evaluated only if the outer 'if' condition evaluates to true."},
	{"java-m3", 4, "C. 3"},
	{"java-m3", 5, "D. Passed"},
	{"java-m3", 6, "A. Block 2"},
	{"java-m3", 7, "B. Set Go Done"},
	{"java-m3", 8, "C. High"},
	{"java-m3", 9, "D. Divisible"},
	{"java-m3", 10, "A. 10"},
	{"java-m3", 11, "B. Compilation Error"},
	{"java-m3", 12, "C. Off"},
	{"java-m3", 13, "D. 20"},
	{"java-m3", 14, "A. Allowed"},
	{"java-m3", 15, "B. Five"},
	{"java-m3", 16, "C. Default One"},
	{"java-m3", 17, "D. Warm"},
	{"java-m3", 18, "A. 6"},
	{"java-m3", 19, "B. 8"},
	{"java-m3", 20, "C. Point 3"},

	// java-m4 (20 questions)
	{"java-m4", 1, "D. do-while loop"},
	{"java-m4", 2, "A. continue"},
	{"java-m4", 3, "B. Infinite loop"},
	{"java-m4", 4, "C. O(N^2)"},
	{"java-m4", 5, "D. semicolon (;)"},
	{"java-m4", 6, "A. for"},
	{"java-m4", 7, "B. Modify the array structure while iterating"},
	{"java-m4", 8, "C. 5"},
	{"java-m4", 9, "D. Exits only the innermost loop"},
	{"java-m4", 10, "A. A labelled break"},
	{"java-m4", 11, "B. 02"},
	{"java-m4", 12, "C. Initialization"},
	{"java-m4", 13, "D. Infinite loop"},
	{"java-m4", 14, "A. Only inside that loop"},
	{"java-m4", 15, "B. do-while"},
	{"java-m4", 16, "C. 12"},
	{"java-m4", 17, "D. 123"},
	{"java-m4", 18, "A. return"},
	{"java-m4", 19, "B. Forgetting to update the loop variable"},
	{"java-m4", 20, "C. enhanced for (for-each)"},

	// java-m5 (20 questions)
	{"java-m5", 1, "A. void"},
	{"java-m5", 2, "B. StackOverflowError"},
	{"java-m5", 3, "C. No"},
	{"java-m5", 4, "D. Pass by value"},
	{"java-m5", 5, "A. Overriding resolution at runtime"},
	{"java-m5", 6, "B. Same name, different parameter lists"},
	{"java-m5", 7, "C. No, it is a compile error"},
	{"java-m5", 8, "D. A base case"},
	{"java-m5", 9, "A. By value (a copy)"},
	{"java-m5", 10, "B. The reference value"},
	{"java-m5", 11, "C. Nothing"},
	{"java-m5", 12, "D. static"},
	{"java-m5", 13, "A. No, it has no instance context"},
	{"java-m5", 14, "B. Only that method"},
	{"java-m5", 15, "C. Zero or more arguments of that type"},
	{"java-m5", 16, "D. Last"},
	{"java-m5", 17, "A. 1"},
	{"java-m5", 18, "B. Iteration"},
	{"java-m5", 19, "C. The local variable wins inside that scope"},
	{"java-m5", 20, "D. private"},

	// java-m6 (20 questions)
	{"java-m6", 1, "A. 2"},
	{"java-m6", 2, "B. length"},
	{"java-m6", 3, "C. ArrayIndexOutOfBoundsException"},
	{"java-m6", 4, "D. Arrays.sort()"},
	{"java-m6", 5, "A. No"},
	{"java-m6", 6, "B. 0"},
	{"java-m6", 7, "C. null"},
	{"java-m6", 8, "D. ArrayIndexOutOfBoundsException"},
	{"java-m6", 9, "A. data.length"},
	{"java-m6", 10, "B. No, it is fixed"},
	{"java-m6", 11, "C. Arrays.sort()"},
	{"java-m6", 12, "D. Returns a readable representation of the array"},
	{"java-m6", 13, "A. O(1)"},
	{"java-m6", 14, "B. int[][] grid = new int[3][4];"},
	{"java-m6", 15, "C. A 2D array whose rows have different lengths"},
	{"java-m6", 16, "D. The array must be sorted"},
	{"java-m6", 17, "A. Copies a range of elements between arrays"},
	{"java-m6", 18, "B. No, it copies references (shallow)"},
	{"java-m6", 19, "C. O(n) because elements must shift"},
	{"java-m6", 20, "D. ArrayList"},

	// java-m7 (20 questions)
	{"java-m7", 1, "A. String Constant Pool (SCP)"},
	{"java-m7", 2, "B. str1.equals(str2)"},
	{"java-m7", 3, "C. StringBuilder"},
	{"java-m7", 4, "D. \"bc\""},
	{"java-m7", 5, "A. For security, caching, and thread safety"},
	{"java-m7", 6, "B. For caching, thread safety and security"},
	{"java-m7", 7, "C. Their reference addresses"},
	{"java-m7", 8, "D. equals()"},
	{"java-m7", 9, "A. StringBuffer"},
	{"java-m7", 10, "B. StringBuilder"},
	{"java-m7", 11, "C. 'J'"},
	{"java-m7", 12, "D. \"hi\""},
	{"java-m7", 13, "A. \"42\""},
	{"java-m7", 14, "B. A String array of length 3"},
	{"java-m7", 15, "C. The String constant pool"},
	{"java-m7", 16, "D. A distinct object on the heap"},
	{"java-m7", 17, "A. equalsIgnoreCase()"},
	{"java-m7", 18, "B. \"el\""},
	{"java-m7", 19, "C. Each concatenation creates a new String object"},
	{"java-m7", 20, "D. 2"},

	// java-m8 (20 questions)
	{"java-m8", 1, "B. Abstraction"},
	{"java-m8", 2, "C. this"},
	{"java-m8", 3, "D. No"},
	{"java-m8", 4, "A. implements"},
	{"java-m8", 5, "B. Method Overloading"},
	{"java-m8", 6, "C. Encapsulation"},
	{"java-m8", 7, "D. To initialise a new object"},
	{"java-m8", 8, "A. Nothing, it has no return type"},
	{"java-m8", 9, "B. Java provides a default no-arg constructor"},
	{"java-m8", 10, "C. The current object instance"},
	{"java-m8", 11, "D. extends"},
	{"java-m8", 12, "A. One"},
	{"java-m8", 13, "B. Interfaces"},
	{"java-m8", 14, "C. Overloading"},
	{"java-m8", 15, "D. Method overriding"},
	{"java-m8", 16, "A. Yes"},
	{"java-m8", 17, "B. No"},
	{"java-m8", 18, "C. protected"},
	{"java-m8", 19, "D. Calls the superclass constructor"},
	{"java-m8", 20, "A. default and static methods"},

	// java-m9 (20 questions)
	{"java-m9", 1, "C. finally"},
	{"java-m9", 2, "D. throws"},
	{"java-m9", 3, "A. Throwable"},
	{"java-m9", 4, "B. Unchecked"},
	{"java-m9", 5, "C. Extend Exception"},
	{"java-m9", 6, "D. Throwable"},
	{"java-m9", 7, "A. IOException"},
	{"java-m9", 8, "B. NullPointerException"},
	{"java-m9", 9, "C. Almost always, whether or not an exception occurred"},
	{"java-m9", 10, "D. throw raises an exception; throws declares one"},
	{"java-m9", 11, "A. Most specific first"},
	{"java-m9", 12, "B. Declared AutoCloseable resources are closed"},
	{"java-m9", 13, "C. ArithmeticException"},
	{"java-m9", 14, "D. Infinity"},
	{"java-m9", 15, "A. RuntimeException"},
	{"java-m9", 16, "B. NumberFormatException"},
	{"java-m9", 17, "C. No, they signal unrecoverable JVM conditions"},
	{"java-m9", 18, "D. It silently swallows failures"},
	{"java-m9", 19, "A. Yes, if it has finally or is try-with-resources"},
	{"java-m9", 20, "B. The original cause of the failure"},

	// java-m10 (20 questions)
	{"java-m10", 1, "D. HashSet"},
	{"java-m10", 2, "A. HashMap"},
	{"java-m10", 3, "B. TreeSet"},
	{"java-m10", 4, "C. map.containsKey()"},
	{"java-m10", 5, "D. ConcurrentModificationException"},
	{"java-m10", 6, "A. Set"},
	{"java-m10", 7, "B. HashMap"},
	{"java-m10", 8, "C. TreeMap"},
	{"java-m10", 9, "D. LinkedHashMap"},
	{"java-m10", 10, "A. No, it is a separate hierarchy"},
	{"java-m10", 11, "B. O(1)"},
	{"java-m10", 12, "C. O(n)"},
	{"java-m10", 13, "D. equals() and hashCode()"},
	{"java-m10", 14, "A. Both are stored in the same bucket"},
	{"java-m10", 15, "B. ConcurrentModificationException"},
	{"java-m10", 16, "C. iterator.remove()"},
	{"java-m10", 17, "D. ConcurrentHashMap"},
	{"java-m10", 18, "A. A fixed-size list backed by the array"},
	{"java-m10", 19, "B. Comparable"},
	{"java-m10", 20, "C. equals() and hashCode()"},

	// java-m11 (20 questions)
	{"java-m11", 1, "A. BufferedReader"},
	{"java-m11", 2, "B. Try-with-resources"},
	{"java-m11", 3, "C. file.exists()"},
	{"java-m11", 4, "D. new FileWriter(\"file.txt\", true)"},
	{"java-m11", 5, "A. java.io"},
	{"java-m11", 6, "B. InputStream/OutputStream"},
	{"java-m11", 7, "C. -1"},
	{"java-m11", 8, "D. null"},
	{"java-m11", 9, "A. new FileWriter(path, true)"},
	{"java-m11", 10, "B. Creates only an object representing a path"},
	{"java-m11", 11, "C. file.createNewFile()"},
	{"java-m11", 12, "D. It batches writes in memory before hitting disk"},
	{"java-m11", 13, "A. The platform-specific line separator"},
	{"java-m11", 14, "B. Buffered data may never reach disk"},
	{"java-m11", 15, "C. Reverse declaration order"},
	{"java-m11", 16, "D. AutoCloseable"},
	{"java-m11", 17, "A. IOException"},
	{"java-m11", 18, "B. So the -1 end-of-stream sentinel can be represented"},
	{"java-m11", 19, "C. BufferedReader"},
	{"java-m11", 20, "D. The JVM working directory"},

	// java-m12 (20 questions)
	{"java-m12", 1, "B. start()"},
	{"java-m12", 2, "C. synchronized"},
	{"java-m12", 3, "D. Java supports multiple interface implementations but only single class inheritance"},
	{"java-m12", 4, "A. Runnable"},
	{"java-m12", 5, "B. Executors"},
	{"java-m12", 6, "C. Executes synchronously on the current thread"},
	{"java-m12", 7, "D. IllegalThreadStateException"},
	{"java-m12", 8, "A. It leaves the single inheritance slot free"},
	{"java-m12", 9, "B. Six"},
	{"java-m12", 10, "C. BLOCKED"},
	{"java-m12", 11, "D. wait()"},
	{"java-m12", 12, "A. It is a read-modify-write sequence, not atomic"},
	{"java-m12", 13, "B. Visibility of reads and writes across threads"},
	{"java-m12", 14, "C. AtomicInteger"},
	{"java-m12", 15, "D. Blocks the caller until the target thread finishes"},
	{"java-m12", 16, "A. Two threads each holding a lock the other needs"},
	{"java-m12", 17, "B. Always acquire locks in the same global order"},
	{"java-m12", 18, "C. Threads are expensive to create and consume ~1MB stack each"},
	{"java-m12", 19, "D. Callable"},
	{"java-m12", 20, "A. Its non-daemon threads keep the JVM alive"},

	// java-m13 (20 questions)
	{"java-m13", 1, "C. @FunctionalInterface"},
	{"java-m13", 2, "D. ::"},
	{"java-m13", 3, "A. reduce()"},
	{"java-m13", 4, "B. Optional.empty()"},
	{"java-m13", 5, "C. Intermediate"},
	{"java-m13", 6, "D. An interface with exactly one abstract method"},
	{"java-m13", 7, "A. @FunctionalInterface"},
	{"java-m13", 8, "B. final or effectively final"},
	{"java-m13", 9, "C. Predicate"},
	{"java-m13", 10, "D. Function"},
	{"java-m13", 11, "A. Consumer"},
	{"java-m13", 12, "B. Supplier"},
	{"java-m13", 13, "C. Source, intermediate operations, terminal operation"},
	{"java-m13", 14, "D. Nothing executes"},
	{"java-m13", 15, "A. No, it throws IllegalStateException"},
	{"java-m13", 16, "B. collect"},
	{"java-m13", 17, "C. An unbound instance method reference"},
	{"java-m13", 18, "D. A constructor reference"},
	{"java-m13", 19, "A. orElse always evaluates its argument; orElseGet is lazy"},
	{"java-m13", 20, "B. As a return type"},

	// java-m14 (20 questions)
	{"java-m14", 1, "C. ResultSet"},
	{"java-m14", 2, "D. They prevent SQL Injection and cache query execution plans"},
	{"java-m14", 3, "A. executeQuery()"},
	{"java-m14", 4, "B. jdbc:mysql://..."},
	{"java-m14", 5, "C. 1"},
	{"java-m14", 6, "D. A standard Java API for relational database access"},
	{"java-m14", 7, "A. Type 4"},
	{"java-m14", 8, "B. 1"},
	{"java-m14", 9, "C. PreparedStatement"},
	{"java-m14", 10, "D. The query structure is compiled before values are bound"},
	{"java-m14", 11, "A. executeUpdate()"},
	{"java-m14", 12, "B. The number of affected rows"},
	{"java-m14", 13, "C. Before the first row"},
	{"java-m14", 14, "D. Call rs.wasNull()"},
	{"java-m14", 15, "A. No, it dies when the statement closes"},
	{"java-m14", 16, "B. true"},
	{"java-m14", 17, "C. rollback()"},
	{"java-m14", 18, "D. RETURN_GENERATED_KEYS"},
	{"java-m14", 19, "A. addBatch() and executeBatch()"},
	{"java-m14", 20, "B. No, give each thread its own"},

	// java-m15 (20 questions)
	{"java-m15", 1, "D. O(log N)"},
	{"java-m15", 2, "A. Stack"},
	{"java-m15", 3, "B. Breadth First Search (BFS)"},
	{"java-m15", 4, "C. O(N^2)"},
	{"java-m15", 5, "D. Queue"},
	{"java-m15", 6, "A. O(log n)"},
	{"java-m15", 7, "B. The data must be sorted"},
	{"java-m15", 8, "C. To avoid integer overflow"},
	{"java-m15", 9, "D. In-order"},
	{"java-m15", 10, "A. It degenerates into a linked list with O(n) operations"},
	{"java-m15", 11, "B. TreeMap and TreeSet"},
	{"java-m15", 12, "C. Queue"},
	{"java-m15", 13, "D. Stack (or recursion)"},
	{"java-m15", 14, "A. BFS"},
	{"java-m15", 15, "B. Cycles would cause an infinite loop"},
	{"java-m15", 16, "C. Merge sort"},
	{"java-m15", 17, "D. O(n^2)"},
	{"java-m15", 18, "A. Equal elements keep their relative order"},
	{"java-m15", 19, "B. TimSort"},
	{"java-m15", 20, "C. ArrayDeque used as a stack"},

	// java-m16 (20 questions)
	{"java-m16", 1, "D. @RestController"},
	{"java-m16", 2, "A. Spring Initializr"},
	{"java-m16", 3, "B. @Autowired"},
	{"java-m16", 4, "C. Tomcat"},
	{"java-m16", 5, "D. JpaRepository"},
	{"java-m16", 6, "A. @Configuration, @EnableAutoConfiguration, @ComponentScan"},
	{"java-m16", 7, "B. In the root package above your components"},
	{"java-m16", 8, "C. @ResponseBody on every method"},
	{"java-m16", 9, "D. @PathVariable"},
	{"java-m16", 10, "A. @RequestParam"},
	{"java-m16", 11, "B. 201"},
	{"java-m16", 12, "C. 204"},
	{"java-m16", 13, "D. GET, PUT, DELETE"},
	{"java-m16", 14, "A. Constructor injection"},
	{"java-m16", 15, "B. Fields cannot be final and testing without the framework is hard"},
	{"java-m16", 16, "C. singleton"},
	{"java-m16", 17, "D. It should be stateless"},
	{"java-m16", 18, "A. @Qualifier"},
	{"java-m16", 19, "B. Unchecked exceptions only"},
	{"java-m16", 20, "C. The proxy is bypassed by self-invocation"},

	// java-m17 (20 questions)
	{"java-m17", 1, "A. @Id"},
	{"java-m17", 2, "B. @Entity"},
	{"java-m17", 3, "C. @Valid"},
	{"java-m17", 4, "D. @RestControllerAdvice"},
	{"java-m17", 5, "A. spring.jpa.hibernate.ddl-auto=update"},
	{"java-m17", 6, "B. JPA is the specification, Hibernate an implementation"},
	{"java-m17", 7, "C. validate"},
	{"java-m17", 8, "D. HikariCP"},
	{"java-m17", 9, "A. @Entity"},
	{"java-m17", 10, "B. STRING"},
	{"java-m17", 11, "C. Reordering the enum silently changes stored meanings"},
	{"java-m17", 12, "D. The owning side"},
	{"java-m17", 13, "A. EAGER"},
	{"java-m17", 14, "B. LAZY"},
	{"java-m17", 15, "C. Touching a lazy association on a detached entity"},
	{"java-m17", 16, "D. Hibernate flushing changed managed entities at commit"},
	{"java-m17", 17, "A. No, dirty checking handles it"},
	{"java-m17", 18, "B. One query per collection element instead of one overall"},
	{"java-m17", 19, "C. @Valid on the @RequestBody parameter"},
	{"java-m17", 20, "D. @RestControllerAdvice"},

	// java-m18 (20 questions)
	{"java-m18", 1, "B. BCryptPasswordEncoder"},
	{"java-m18", 2, "C. JSON Web Token"},
	{"java-m18", 3, "D. Authorization Header"},
	{"java-m18", 4, "A. csrf().disable()"},
	{"java-m18", 5, "B. Authorization"},
	{"java-m18", 6, "C. AuthN is who you are; AuthZ is what you may do"},
	{"java-m18", 7, "D. 401"},
	{"java-m18", 8, "A. 403"},
	{"java-m18", 9, "B. header.payload.signature"},
	{"java-m18", 10, "C. No, it is only Base64URL encoded and readable by anyone"},
	{"java-m18", 11, "D. Integrity"},
	{"java-m18", 12, "A. They cannot easily be revoked before expiry"},
	{"java-m18", 13, "B. Hashing is one-way; the system never needs the plaintext"},
	{"java-m18", 14, "C. They are too fast, enabling rapid brute force"},
	{"java-m18", 15, "D. Generates a unique random salt per password"},
	{"java-m18", 16, "A. encoder.matches(raw, hash)"},
	{"java-m18", 17, "B. Each call uses a new random salt"},
	{"java-m18", 18, "C. For a stateless API authenticated by an Authorization header"},
	{"java-m18", 19, "D. ROLE_ADMIN"},
	{"java-m18", 20, "A. Top to bottom, first match wins"},

	// java-m19 (20 questions)
	{"java-m19", 1, "D. mvn clean package"},
	{"java-m19", 2, "A. java -jar app.jar"},
	{"java-m19", 3, "B. docker build"},
	{"java-m19", 4, "C. target/"},
	{"java-m19", 5, "D. Passed via Environment Variables"},
	{"java-m19", 6, "A. An executable fat JAR with dependencies and an embedded server"},
	{"java-m19", 7, "B. no main manifest attribute"},
	{"java-m19", 8, "C. java -jar app.jar"},
	{"java-m19", 9, "D. Command-line arguments"},
	{"java-m19", 10, "A. SPRING_DATASOURCE_URL"},
	{"java-m19", 11, "B. Use DB_URL, or the H2 URL as a fallback"},
	{"java-m19", 12, "C. --spring.profiles.active=prod"},
	{"java-m19", 13, "D. 5000"},
	{"java-m19", 14, "A. Through the PORT environment variable"},
	{"java-m19", 15, "B. It is ephemeral and wiped on redeploy"},
	{"java-m19", 16, "C. A much smaller runtime image containing only the JRE and JAR"},
	{"java-m19", 17, "D. So the dependency layer stays cached when only source changes"},
	{"java-m19", 18, "A. A compromise would inherit full privileges"},
	{"java-m19", 19, "B. They are permanently visible in the image history"},
	{"java-m19", 20, "C. By the service name, e.g. jdbc:postgresql://db:5432/..."},

	// java-m1-assignment (6 questions)
	{"java-m1-assignment", 1, "A. Java Development Kit (JDK)"},
	{"java-m1-assignment", 2, "B. The compiled bytecode (.class) is platform-neutral and can run on any JVM."},
	{"java-m1-assignment", 3, "C. The JVM is platform-dependent; a specific version must be installed for each OS."},
	{"java-m1-assignment", 4, "D. javac App.java"},
	{"java-m1-assignment", 5, "A. java App"},
	{"java-m1-assignment", 6, "B. .class"},

	// java-m10-assignment (1 questions)
	{"java-m10-assignment", 2, "D. When you need the keys to be maintained in a sorted order."},

	// java-m11-assignment (1 questions)
	{"java-m11-assignment", 2, "A. It buffers input for efficient reading, reducing the number of costly system/disk read operations."},

	// java-m12-assignment (1 questions)
	{"java-m12-assignment", 2, "B. start() creates a new thread and executes run() asynchronously in it; calling run() directly runs the code synchronously in the current thread."},

	// java-m14-assignment (1 questions)
	{"java-m14-assignment", 2, "C. It manages the list of database drivers, matches connection requests with the appropriate driver, and establishes the connection."},

	// java-m16-assignment (1 questions)
	{"java-m16-assignment", 2, "D. Constructor injection allows the class to declare dependencies as final (immutable), enforces required dependencies, and simplifies unit testing."},

	// java-m17-assignment (1 questions)
	{"java-m17-assignment", 2, "A. JPA is the specification (guidelines/interface); Hibernate is a concrete provider (implementation) of the JPA specification."},

	// java-m18-assignment (2 questions)
	{"java-m18-assignment", 1, "B. The Authorization header (using Bearer scheme)."},
	{"java-m18-assignment", 2, "C. BCrypt is a slow, adaptive hashing algorithm that makes brute-force attacks much harder; SHA-256 is extremely fast and vulnerable to hardware-accelerated cracking."},

	// java-m19-assignment (1 questions)
	{"java-m19-assignment", 2, "D. It removes the target directory (compiled classes, packaged files) to ensure a fresh, full build."},

	// java-m2-assignment (1 questions)
	{"java-m2-assignment", 2, "C. Widening is done automatically when converting a smaller type to a larger type; narrowing must be done manually."},

	// java-m4-assignment (1 questions)
	{"java-m4-assignment", 2, "D. break terminates the loop entirely; continue skips the current iteration and moves to the next one."},

	// java-m7-assignment (1 questions)
	{"java-m7-assignment", 2, "A. The \"==\" operator compares memory references (addresses), not the actual contents."},

	// java-m8-assignment (1 questions)
	{"java-m8-assignment", 2, "B. An abstract class can have instance fields and constructors; an interface cannot have instance fields or constructors."},

	// java-m9-assignment (1 questions)
	{"java-m9-assignment", 2, "C. throw is used to explicitly throw a single exception instance; throws is used in method signatures to declare exceptions that might be thrown."},

	// networking-m1 (6 questions)
	{"networking-m1", 1, "Latency and bandwidth are independent — each keystroke still waits a full round trip regardless of capacity"},
	{"networking-m1", 2, "The MAC addresses change at every hop; the IP addresses stay the same end to end"},
	{"networking-m1", 3, "The logical topology — a switch forwards each frame only to the destination port, so each port becomes its own collision domain"},
	{"networking-m1", 4, "Data traffic is bursty, so statistical multiplexing lets idle moments carry other users' traffic"},
	{"networking-m1", 5, "The TCP window must hold at least 12.5 MB of unacknowledged data or the sender stalls and cannot fill the link"},
	{"networking-m1", 6, "Every peer is also uploading, so supply grows with demand rather than being fixed"},

	// networking-m1-assignment (6 questions)
	{"networking-m1-assignment", 1, "The TCP window is smaller than the bandwidth-delay product, so the sender stalls waiting for ACKs"},
	{"networking-m1-assignment", 2, "About 12.5 MB — bandwidth in bytes per second multiplied by the round-trip time"},
	{"networking-m1-assignment", 3, "A leaf cable failing in a star"},
	{"networking-m1-assignment", 4, "The source and destination IP addresses stay the same throughout; the MAC addresses are rewritten at every hop"},
	{"networking-m1-assignment", 5, "It takes roughly 1.18 seconds — latency is per round trip and no amount of bandwidth reduces it"},
	{"networking-m1-assignment", 6, "About 1000 — statistical multiplexing shares the idle time between users"},

	// os-m1 (6 questions)
	{"os-m1", 1, "To enforce, in hardware, that ordinary processes cannot execute privileged instructions or touch kernel memory"},
	{"os-m1", 2, "The driver needed to read the root filesystem may itself live on the root filesystem, so a temporary in-memory root breaks the circular dependency"},
	{"os-m1", 3, "A controlled entry into kernel mode at a fixed entry point, used to request privileged work"},
	{"os-m1", 4, "Monolithic kernels are faster because subsystems call each other directly; microkernels isolate failures at the cost of IPC overhead"},
	{"os-m1", 5, "The MMU detected an access the page permissions forbid and faulted; the kernel terminated the process"},
	{"os-m1", 6, "Exactly one — PID 1 — which then starts everything else"},

	// os-m1-assignment (6 questions)
	{"os-m1-assignment", 1, "100,000 unbuffered and 25 buffered — the buffer flushes only when full"},
	{"os-m1-assignment", 2, "strace, which prints every syscall with its arguments and return value"},
	{"os-m1-assignment", 3, "Firmware or hardware — nothing has got as far as the bootloader"},
	{"os-m1-assignment", 4, "The MMU refuses the access at execution time and raises a fault; the kernel then delivers SIGSEGV"},
	{"os-m1-assignment", 5, "Nothing is wrong — page cache is reclaimable, so look at the available column rather than free"},
	{"os-m1-assignment", 6, "No — modules still run in kernel mode with full privileges; only the loading is dynamic"},

	// seo-m1 (6 questions)
	{"seo-m1", 1, "Pages retrieved and read while the answer is being written"},
	{"seo-m1", 2, "Search first, read the results, then write the answer"},
	{"seo-m1", 3, "Industry coinages, not official programmes run by search companies"},
	{"seo-m1", 4, "Very little — answers vary by user and run, so you need repeat samples"},
	{"seo-m1", 5, "Retrieval usually searches a conventional index, so unindexed pages cannot be fetched"},
	{"seo-m1", 6, "You may block the retrieval fetchers that would otherwise cite your pages"},

	// seo-m2 (6 questions)
	{"seo-m2", 1, "The direct answer appears in the first sentences, before the background"},
	{"seo-m2", 2, "Because it may be retrieved and read on its own, with no surrounding context"},
	{"seo-m2", 3, "Structured content is the visible organisation of the page; structured data is machine-readable markup"},
	{"seo-m2", 4, "It must also be visible to a human on the page"},
	{"seo-m2", 5, "No — eligibility rules are set by the search engine and have been narrowed"},
	{"seo-m2", 6, "Serving different content to bots than to people is cloaking, against search guidelines"},

	// seo-m3 (6 questions)
	{"seo-m3", 1, "Because answers vary between runs, so only a rate over several runs is evidence"},
	{"seo-m3", 2, "The answer is coming from the model’s memory of training data rather than a live fetch"},
	{"seo-m3", 3, "\"Rosie’s Bakery is an independent bakery in Bedminster, Bristol, founded in 2016.\""},
	{"seo-m3", 4, "A crawler may never read the hours, so the business has no stated hours in text"},
	{"seo-m3", 5, "No — you can only change what live retrieval finds and what future training data may contain"},
	{"seo-m3", 6, "Consistent facts repeated across many independent, crawlable sources"},

	// seo-m4 (6 questions)
	{"seo-m4", 1, "Everything, including /admin/, because it obeys only its own group"},
	{"seo-m4", 2, "Whether your content is used for Gemini and Vertex AI training"},
	{"seo-m4", 3, "ChatGPT-User"},
	{"seo-m4", 4, "Allow crawling and add a noindex tag"},
	{"seo-m4", 5, "A community proposal that major providers have not committed to, with weak evidence of benefit"},
	{"seo-m4", 6, "It gives no visibility benefit and may reduce how well future models know you"},

	// seo-m5 (6 questions)
	{"seo-m5", 1, "A concept from the Search Quality Rater Guidelines describing Experience, Expertise, Authoritativeness and Trust"},
	{"seo-m5", 2, "Trust"},
	{"seo-m5", 3, "\"Your Money or Your Life\" — topics affecting health, safety or finances, judged to a higher standard"},
	{"seo-m5", 4, "It lists other official URLs for the same entity, such as its verified profiles"},
	{"seo-m5", 5, "Do not write it — Wikipedia has strict notability rules and forbids self-promotion; earn independent coverage first"},
	{"seo-m5", 6, "A banned practice — incentivised and fake reviews fall under the DMCC Act and are enforced by the CMA"},

	// seo-m6 (6 questions)
	{"seo-m6", 1, "Training data the model learned from, and live retrieval while it answers"},
	{"seo-m6", 2, "With rel=\"sponsored\" or rel=\"nofollow\", per Google’s link-spam guidance"},
	{"seo-m6", 3, "The most important information first, then supporting detail, then background"},
	{"seo-m6", 4, "Who, what, when, where and why"},
	{"seo-m6", 5, "Syndicated republication, valuable for reach, citation surface and being on record — not for PageRank"},
	{"seo-m6", 6, "No one can guarantee AI citations; you can improve the odds and spot-check citations over time"},

	// seo-m7 (6 questions)
	{"seo-m7", 1, "Content is judged on whether it is helpful and people-first; scaled content abuse is the violation"},
	{"seo-m7", 2, "It gives a clear question-and-answer structure that machines can parse, and forces real customer questions to be written down"},
	{"seo-m7", 3, "It must match content that is visible on the page"},
	{"seo-m7", 4, "LCP 2.5s or less, INP 200 ms or less, CLS 0.1 or less"},
	{"seo-m7", 5, "The field data, because assessment uses the 75th percentile of real-user data"},
	{"seo-m7", 6, "A page that no other page on the site links to"},

	// seo-m8 (6 questions)
	{"seo-m8", 1, "Assistants publish no rankings, and answers vary by user and by run"},
	{"seo-m8", 2, "Use a fixed prompt set, repeat each prompt several times, and calculate mention rate and citation share"},
	{"seo-m8", 3, "Googlebot, the same crawler as Search"},
	{"seo-m8", 4, "It controls use of your content for Gemini and Vertex AI training and grounding, not AI Overviews eligibility"},
	{"seo-m8", 5, "It must be crawlable and indexable in ordinary Google Search"},
	{"seo-m8", 6, "Refuse: fake reviews are illegal in the UK and breach platform rules; collect real reviews instead"},

	// seo-m1-assignment (6 questions)
	{"seo-m1-assignment", 1, "You have one observation of a possible mention; repeat the question several times in each assistant before concluding anything"},
	{"seo-m1-assignment", 2, "The pages cannot be fetched, so no rewrite could have helped; remove the block, wait for re-crawling, then re-sample over several weeks"},
	{"seo-m1-assignment", 3, "The assistant retrieved an out-of-date third-party page, such as a directory listing, that still carries the old hours"},
	{"seo-m1-assignment", 4, "Block the named training crawlers specifically, leave retrieval and search fetchers allowed, and re-check the user-agent names in the providers’ current documentation"},
	{"seo-m1-assignment", 5, "No one outside the assistant companies controls which sources an answer uses, and answers vary run to run, so the position cannot be guaranteed"},
	{"seo-m1-assignment", 6, "The assistants trust third-party pages for these questions, so both improving the roastery’s own pages and getting the directory and blog entries right matter"},

	// seo-m2-assignment (6 questions)
	{"seo-m2-assignment", 1, "Lifted out of the page it names no business, no service and no place, so it answers nothing"},
	{"seo-m2-assignment", 2, "Improve the single existing page instead; a near-duplicate splits signals and a bot-only variant would be cloaking"},
	{"seo-m2-assignment", 3, "Remove the four invented questions — marking up content a visitor cannot see can be treated as spam"},
	{"seo-m2-assignment", 4, "The markup makes a factual claim that contradicts the page, which is both misleading to customers and a mismatch the search engine may act on"},
	{"seo-m2-assignment", 5, "(a) — an assistant reads the visible text, so a clear standalone answer changes what it can quote; markup only labels facts"},
	{"seo-m2-assignment", 6, "Fetchers that do not run JavaScript will never see the markup, so on those fetches the page has none"},

	// seo-m3-assignment (6 questions)
	{"seo-m3-assignment", 1, "Correct the hours on the directory listing, then make the hours plain text on the clinic’s own site so there is a better source to cite."},
	{"seo-m3-assignment", 2, "The answer is shaped by what they fed into the chat, and one run of one brand-name prompt shows nothing about what a new customer would see."},
	{"seo-m3-assignment", 3, "The old name is remembered from training data, so it can only fade as sources are corrected and future models are trained."},
	{"seo-m3-assignment", 4, "The facts are not in visible page text, so a crawler may never read them and the assistant falls back on other sources."},
	{"seo-m3-assignment", 5, "It misdescribes how models work: a trained model cannot be edited or submitted to, so there is nothing to buy."},
	{"seo-m3-assignment", 6, "Treat it as invention caused by ambiguity, publish an unmistakable \"Our practice\" page naming the one surgery, and re-test monthly."},

	// seo-m4-assignment (6 questions)
	{"seo-m4-assignment", 1, "The site’s text is less likely to be used in future OpenAI model training, while it can still be found and cited in ChatGPT’s search-backed answers."},
	{"seo-m4-assignment", 2, "Everything, including /admin/ and /booking/, because a bot obeys only the group that names it."},
	{"seo-m4-assignment", 3, "Google-Extended controls use of the content for Gemini and Vertex AI training only; AI Overviews are built from Google Search, which Googlebot crawls."},
	{"seo-m4-assignment", 4, "Remove the Disallow so the page can be crawled, and add a noindex tag to the page itself."},
	{"seo-m4-assignment", 5, "Assistants can no longer open the site when a real customer asks about it or pastes a link, and almost no server load was saved."},
	{"seo-m4-assignment", 6, "Very little, because the search crawlers that might act on those pages are asked not to fetch them at all — and llms.txt is an unproven community proposal in any case."},

	// seo-m5-assignment (6 questions)
	{"seo-m5-assignment", 1, "Explain that Wikipedia requires significant coverage in independent reliable sources and forbids self-promotion, so the honest route is to earn genuine coverage first"},
	{"seo-m5-assignment", 2, "Explain that E-E-A-T comes from the Search Quality Rater Guidelines and there is no such score, then offer a plan of specific trust improvements with evidence of the work"},
	{"seo-m5-assignment", 3, "This is a YMYL topic where bad information could harm someone, so do not give dosing advice; write about the writer’s own experience, cite recognised health sources, and point readers to a clinician"},
	{"seo-m5-assignment", 4, "Claim the duplicate and use the platform’s process to mark it as closed or merge it, so only the canonical record remains"},
	{"seo-m5-assignment", 5, "Conditioning a reward on a positive review and hiding the incentive is a banned practice in the UK under the DMCC Act and can be enforced by the CMA; ask every customer for a review with no reward attached"},
	{"seo-m5-assignment", 6, "Remove the sameAs entry that nobody has verified, and either publish the opening hours visibly on the page or take them out of the markup so the facts match"},

	// seo-m6-assignment (6 questions)
	{"seo-m6-assignment", 1, "Those placements are mostly automatic syndication with nofollow or sponsored links; direct outreach to a small, relevant media list costs nothing and usually achieves more"},
	{"seo-m6-assignment", 2, "Explain that links in paid placements must be marked nofollow or sponsored under Google’s link-spam guidance, and buy it only if the reach itself is worth the fee"},
	{"seo-m6-assignment", 3, "Decline the undisclosed version; a paid post must be clearly labelled as paid and any links marked sponsored, so offer a properly disclosed collaboration or send a sample with no strings"},
	{"seo-m6-assignment", 4, "Send one follow-up on the same thread that adds something new, then stop and tell the client why"},
	{"seo-m6-assignment", 5, "No one can guarantee what an assistant says, because answers vary by user, region and date; offer a plan of earned coverage and a monthly spot check recorded the same way each time"},
	{"seo-m6-assignment", 6, "Do not call it national; describe it accurately as a survey of 38 of the business’s own customers, state the method and limits, and find a smaller, true claim worth reporting"},

	// seo-m7-assignment (6 questions)
	{"seo-m7-assignment", 1, "Either add the same question and answer as visible text on the page, or delete it from the markup"},
	{"seo-m7-assignment", 2, "Remove the award and customer claims, correct the date to 2019, and replace them with facts the owner can prove"},
	{"seo-m7-assignment", 3, "The page fails on LCP, because assessment uses the 75th percentile of real-user field data"},
	{"seo-m7-assignment", 4, "Decline: mass pages made mainly to manipulate rankings are scaled content abuse, and offer instead a small number of pages with real local detail"},
	{"seo-m7-assignment", 5, "It is an orphan page; add at least one descriptive internal link to it from a relevant page on the same topic"},
	{"seo-m7-assignment", 6, "Reserve the banner's height in CSS so the space is held before the banner loads"},

	// seo-m8-assignment (6 questions)
	{"seo-m8-assignment", 1, "Explain that assistants publish no rankings and answers vary per run, and report mention rate and citation share from a fixed prompt set instead"},
	{"seo-m8-assignment", 2, "Nothing changes for AI Overviews eligibility; Google-Extended controls use of the content for Gemini and Vertex AI training and grounding"},
	{"seo-m8-assignment", 3, "Decline: a single run proves nothing, because answers vary between runs and users, and suggest repeated sampling across several prompts and assistants first"},
	{"seo-m8-assignment", 4, "Nowhere reliable: Search Console does not break out AI Overview clicks per surface, so you state it as a blind spot"},
	{"seo-m8-assignment", 5, "Refuse: fake reviews are illegal in the UK and breach platform rules, and propose a way to collect real reviews instead"},
	{"seo-m8-assignment", 6, "The prompt wording changed, so the two months are not comparable; keep the prompt set fixed and re-run the original wording"},

	// sql-m1 (5 questions)
	{"sql-m1", 1, "Structured data with fixed, typed columns"},
	{"sql-m1", 2, "The second write overwrites the first, and one row is silently lost"},
	{"sql-m1", 3, "By storing the matching key value"},
	{"sql-m1", 4, "Faster reads, at the cost of updating every copy when the author changes"},
	{"sql-m1", 5, "The planner, which chooses the execution strategy"},

	// sql-m2 (5 questions)
	{"sql-m2", 1, "You are in the wrong database, the wrong schema, or the name was created quoted with capitals"},
	{"sql-m2", 2, "NULL means unknown, so the comparison is UNKNOWN and never TRUE"},
	{"sql-m2", 3, "NUMERIC, because it is exact"},
	{"sql-m2", 4, "TIMESTAMPTZ records an actual moment; TIMESTAMP records an ambiguous wall-clock reading"},
	{"sql-m2", 5, "Deletes the child rows too, and any rows cascading from them"},

	// sql-m3 (5 questions)
	{"sql-m3", 1, "TRUNCATE"},
	{"sql-m3", 2, "Fail, because the existing rows would violate the constraint"},
	{"sql-m3", 3, "It returns the inserted rows, so a generated id needs no second query"},
	{"sql-m3", 4, "WHERE is evaluated before SELECT, so the alias does not exist yet"},
	{"sql-m3", 5, "The engine may return a different, arbitrary ten rows each time"},

	// sql-m4 (5 questions)
	{"sql-m4", 1, "created_at >= '2024-01-01' AND created_at < '2024-02-01'"},
	{"sql-m4", 2, "IS DISTINCT FROM"},
	{"sql-m4", 3, "Everyone in dept 1, plus anyone in dept 2 earning over 70000"},
	{"sql-m4", 4, "A leading wildcard means the sort order gives no starting point"},
	{"sql-m4", 5, "TRUE, because the result cannot depend on the unknown"},

	// sql-m5 (5 questions)
	{"sql-m5", 1, "3, because integer divided by integer is an integer"},
	{"sql-m5", 2, "total / NULLIF(quantity, 0)"},
	{"sql-m5", 3, "NULL for the entire expression"},
	{"sql-m5", 4, "DATE_TRUNC('month', ts)"},
	{"sql-m5", 5, "At the first character — SQL strings are 1-indexed"},

	// sql-m6 (5 questions)
	{"sql-m6", 1, "5 and 4"},
	{"sql-m6", 2, "WHERE runs before the aggregation, so the value does not exist yet"},
	{"sql-m6", 3, "4.0, because AVG divides by the three non-null values"},
	{"sql-m6", 4, "In the GROUP BY, or wrapped in an aggregate"},
	{"sql-m6", 5, "They form a single group of their own"},

	// sql-m7 (5 questions)
	{"sql-m7", 1, "An employee with no department and a department with no employees both lack a partner"},
	{"sql-m7", 2, "NULL fails the comparison, so WHERE removes the NULL-padded rows"},
	{"sql-m7", 3, "LEFT JOIN from departments and use COUNT(e.id)"},
	{"sql-m7", 4, "LEFT JOIN orders and filter WHERE orders.id IS NULL"},
	{"sql-m7", 5, "The ON clause is missing, so it became a cross join"},

	// sql-m8 (5 questions)
	{"sql-m8", 1, "It references a column from the outer query, so it re-runs per outer row"},
	{"sql-m8", 2, "Comparing against NULL yields UNKNOWN, so the condition is never TRUE"},
	{"sql-m8", 3, "NOT EXISTS, because it stops at the first match and handles NULL correctly"},
	{"sql-m8", 4, "DENSE_RANK()"},
	{"sql-m8", 5, "It raises an error at runtime, not at parse time"},

	// sql-m9 (5 questions)
	{"sql-m9", 1, "No — a view stores the query, so every read costs the full join"},
	{"sql-m9", 2, "Freshness — the data is stale until you refresh it"},
	{"sql-m9", 3, "WHERE status = 'paid'"},
	{"sql-m9", 4, "Run ANALYZE to refresh the table statistics"},
	{"sql-m9", 5, "None — only the referenced primary key is indexed"},

	// sql-m10 (5 questions)
	{"sql-m10", 1, "Atomicity — an uncommitted transaction is rolled back entirely"},
	{"sql-m10", 2, "The write-ahead log is flushed to disk before COMMIT returns"},
	{"sql-m10", 3, "REPEATABLE READ"},
	{"sql-m10", 4, "A SAVEPOINT taken before the failing statement"},
	{"sql-m10", 5, "Acquiring locks in a consistent order everywhere"},

	// sql-m11 (5 questions)
	{"sql-m11", 1, "It can COMMIT and ROLLBACK inside itself"},
	{"sql-m11", 2, "Its result can be cached or indexed, and becomes silently wrong when the table changes"},
	{"sql-m11", 3, "A parameter named after a column makes WHERE id = id always true"},
	{"sql-m11", 4, "Each iteration creates an implicit savepoint, which is slow"},
	{"sql-m11", 5, "<> misses a change from NULL to a value"},

	// sql-m12 (5 questions)
	{"sql-m12", 1, "A UNIQUE constraint on the foreign key column"},
	{"sql-m12", 2, "The many side, because a column can hold only one value"},
	{"sql-m12", 3, "A student cannot be enrolled in the same course twice"},
	{"sql-m12", 4, "An invoice is a historical fact and must not change when the catalogue does"},
	{"sql-m12", 5, "The N+1 query problem"},

	// sql-m13 (5 questions)
	{"sql-m13", 1, "Insertion anomaly"},
	{"sql-m13", 2, "One fact stored in more than one place"},
	{"sql-m13", 3, "1NF, because the value is not atomic"},
	{"sql-m13", 4, "Only when the primary key is composite"},
	{"sql-m13", 5, "3NF — dept_name depends on dept_id, not on the employee"},

	// sql-m14 (5 questions)
	{"sql-m14", 1, "It is stored parsed, so it can be indexed and queried with operators"},
	{"sql-m14", 2, "A termination condition, without which it loops forever"},
	{"sql-m14", 3, "It keeps every input row and attaches the total to each"},
	{"sql-m14", 4, "A running total up to the current row, not the partition total"},
	{"sql-m14", 5, "Window functions are evaluated after WHERE, so wrap the query in a CTE"},

	// sql-m15 (5 questions)
	{"sql-m15", 1, "A transaction lives on one connection, and pool.query may use a different one each call"},
	{"sql-m15", 2, "A connection leaks per failed request until the pool drains and requests hang"},
	{"sql-m15", 3, "No — placeholders bind values, not identifiers; use an allow-list"},
	{"sql-m15", 4, "The query is planned before the value arrives, so input can never become code"},
	{"sql-m15", 5, "It cannot run inside one, and most migration tools wrap migrations in a transaction by default"},

	// sql-m16 (5 questions)
	{"sql-m16", 1, "So each line can be joined, aggregated and constrained as real data"},
	{"sql-m16", 2, "UPDATE products SET stock = stock - 1 WHERE id = $1 AND stock > 0"},
	{"sql-m16", 3, "Deleting a product must not erase what a customer bought"},
	{"sql-m16", 4, "A partial unique index: CREATE UNIQUE INDEX ON addresses (user_id) WHERE is_default"},
	{"sql-m16", 5, "Two modules claiming the same slot in one course"},

	// sql-m17 (5 questions)
	{"sql-m17", 1, "Run EXPLAIN ANALYZE — measure before changing anything"},
	{"sql-m17", 2, "COUNT(*) counts the NULL-padded row, reporting 1 where the answer is 0"},
	{"sql-m17", 3, "ROW_NUMBER() OVER (PARTITION BY ...) in a CTE, filtered to rn <= N"},
	{"sql-m17", 4, "The date minus a row number is constant within a run"},
	{"sql-m17", 5, "Sharding, because it complicates everything else"},

	// testing-m1 (6 questions)
	{"testing-m1", 1, "A. Test Planning"},
	{"testing-m1", 2, "B. To find defects and verify expected behavior"},
	{"testing-m1", 3, "D. Agile"},
	{"testing-m1", 4, "A. Fewer artefacts have been built on the wrong assumption"},
	{"testing-m1", 5, "B. Test Case Development"},
	{"testing-m1", 6, "C. Assessing quality risk and advocating for the user"},

	// testing-m2 (6 questions)
	{"testing-m2", 1, "D. Pesticide paradox"},
	{"testing-m2", 2, "A. Acceptance Testing"},
	{"testing-m2", 3, "D. Testing must be prioritised by risk"},
	{"testing-m2", 4, "A. Load testing"},
	{"testing-m2", 5, "B. The internal code structure"},
	{"testing-m2", 6, "C. Building it right versus building the right thing"},

	// testing-m3 (6 questions)
	{"testing-m3", 1, "C. 5, 6, 7, 11, 12, 13"},
	{"testing-m3", 2, "D. A high-level, static project or organizational policy document defining testing approaches"},
	{"testing-m3", 3, "D. One value below 18, one within, one above 60"},
	{"testing-m3", 4, "A. Decision table testing"},
	{"testing-m3", 5, "B. Systems whose behaviour depends on prior events"},
	{"testing-m3", 6, "C. A scenario is what to test; a case gives steps and expected results"},

	// testing-m4 (6 questions)
	{"testing-m4", 1, "B. Invalid/Rejected"},
	{"testing-m4", 2, "C. Low Severity, High Priority"},
	{"testing-m4", 3, "D. The technical impact of the defect"},
	{"testing-m4", 4, "A. Exact steps, environment, and expected versus actual results"},
	{"testing-m4", 5, "B. Retest it and run regression on related areas"},
	{"testing-m4", 6, "C. Jira"},

	// testing-m5 (6 questions)
	{"testing-m5", 1, "A. Product Owner"},
	{"testing-m5", 2, "B. 15 minutes"},
	{"testing-m5", 3, "D. From the start, to shape acceptance criteria"},
	{"testing-m5", 4, "A. To improve how the team works"},
	{"testing-m5", 5, "B. Tests written and passing for the story"},
	{"testing-m5", 6, "C. The team and stakeholders"},

	// testing-m6 (6 questions)
	{"testing-m6", 1, "D. POST"},
	{"testing-m6", 2, "A. Unauthorized (Authentication failed)"},
	{"testing-m6", 3, "D. PUT"},
	{"testing-m6", 4, "A. The resource was not found"},
	{"testing-m6", 5, "B. To run the same requests against dev, staging and prod"},
	{"testing-m6", 6, "C. The server side"},

	// testing-m7 (6 questions)
	{"testing-m7", 1, "C. LEFT JOIN"},
	{"testing-m7", 2, "D. UPDATE"},
	{"testing-m7", 3, "D. Confirming data stays accurate and consistent across operations"},
	{"testing-m7", 4, "A. HAVING"},
	{"testing-m7", 5, "B. Query the table and compare stored values to the input"},
	{"testing-m7", 6, "C. Only rows with matches in both tables"},

	// testing-m8 (6 questions)
	{"testing-m8", 1, "B. quit()"},
	{"testing-m8", 2, "C. It blocks execution for a fixed duration, slowing down tests unnecessarily"},
	{"testing-m8", 3, "D. A unique id attribute"},
	{"testing-m8", 4, "A. Waits for a specific condition up to a timeout"},
	{"testing-m8", 5, "B. Switch to the frame first"},
	{"testing-m8", 6, "C. Select"},

	// testing-m9 (6 questions)
	{"testing-m9", 1, "A. pom.xml"},
	{"testing-m9", 2, "B. Decoupling test code from webpage UI selectors, reducing maintenance costs"},
	{"testing-m9", 3, "D. @DataProvider"},
	{"testing-m9", 4, "A. Data-driven and keyword-driven approaches"},
	{"testing-m9", 5, "B. Results are visible and traceable after every run"},
	{"testing-m9", 6, "C. In the page class"},

	// testing-m10 (6 questions)
	{"testing-m10", 1, "D. Endurance (Soak) Testing"},
	{"testing-m10", 2, "A. Time taken for a request to travel from client to server and return the first byte"},
	{"testing-m10", 3, "D. The breaking point beyond normal load"},
	{"testing-m10", 4, "A. A sudden sharp jump in users"},
	{"testing-m10", 5, "B. It shows the slow experience the average hides"},
	{"testing-m10", 6, "C. Apache JMeter"},

	// testing-m11 (6 questions)
	{"testing-m11", 1, "C. ADB (Android Debug Bridge)"},
	{"testing-m11", 2, "D. It is cross-platform, letting you use the same API for Android and iOS tests"},
	{"testing-m11", 3, "D. Real hardware reveals issues emulators miss"},
	{"testing-m11", 4, "A. Interruptions like calls and network changes"},
	{"testing-m11", 5, "B. Any language with a WebDriver client"},
	{"testing-m11", 6, "C. Xcode with Appium Inspector"},

	// testing-m12 (6 questions)
	{"testing-m12", 1, "B. Open Web Application Security Project"},
	{"testing-m12", 2, "C. SQL Injection"},
	{"testing-m12", 3, "D. Injecting scripts that run in other users' browsers"},
	{"testing-m12", 4, "A. Users cannot reach resources they are not permitted to"},
	{"testing-m12", 5, "B. To slow down brute-force attacks"},
	{"testing-m12", 6, "C. Parameterised queries"},

	// testing-m13 (6 questions)
	{"testing-m13", 1, "A. git checkout -b"},
	{"testing-m13", 2, "B. Jenkinsfile"},
	{"testing-m13", 3, "D. Merging changes often with automated builds and tests"},
	{"testing-m13", 4, "A. Regressions are caught before merging"},
	{"testing-m13", 5, "B. git add ."},
	{"testing-m13", 6, "C. Quality feedback at every stage"},

	// testing-m14 (6 questions)
	{"testing-m14", 1, "D. Automatically updating selectors when DOM elements change, reducing maintenance"},
	{"testing-m14", 2, "A. It compares screenshots using machine learning to detect visual deviations, regardless of HTML changes"},
	{"testing-m14", 3, "D. They can miss business context and must be reviewed"},
	{"testing-m14", 4, "A. Clustering similar failures and suggesting likely causes"},
	{"testing-m14", 5, "B. Judging whether results are correct and meaningful"},
	{"testing-m14", 6, "C. Silently binding to the wrong element"},

	// testing-m15 (6 questions)
	{"testing-m15", 1, "B. Checkout and payment"},
	{"testing-m15", 2, "C. Balances stay consistent even when a step fails"},
	{"testing-m15", 3, "D. Status codes, response schema and error handling"},
	{"testing-m15", 4, "A. Page Object Model with data-driven tests"},
	{"testing-m15", 5, "B. Coverage, defects found, and open risks"},
	{"testing-m15", 6, "C. They prove the system handles invalid input safely"},

	// testing-m1-assignment (1 questions)
	{"testing-m1-assignment", 1, "C. Requirements Analysis -> Test Planning -> Test Case Development -> Environment Setup -> Test Execution -> Test Closure"},

	// testing-m10-assignment (1 questions)
	{"testing-m10-assignment", 1, "B. Throughput"},

	// testing-m11-assignment (1 questions)
	{"testing-m11-assignment", 1, "A. Hybrid App"},

	// testing-m12-assignment (1 questions)
	{"testing-m12-assignment", 1, "D. Cross-Site Scripting (XSS)"},

	// testing-m13-assignment (1 questions)
	{"testing-m13-assignment", 1, "C. git pull"},

	// testing-m14-assignment (1 questions)
	{"testing-m14-assignment", 1, "B. Applitools Eyes"},

	// testing-m15-assignment (1 questions)
	{"testing-m15-assignment", 1, "C. To combine POM, Data-Driven testing, custom logging, and visual HTML reporting into an extensible, reusable test engine."},

	// testing-m2-assignment (1 questions)
	{"testing-m2-assignment", 1, "B. Verification evaluates static documents (reviews/walkthroughs); Validation executes the active code to verify system behavior."},

	// testing-m3-assignment (1 questions)
	{"testing-m3-assignment", 1, "A. 9, 10, 11, 49, 50, 51"},

	// testing-m4-assignment (1 questions)
	{"testing-m4-assignment", 1, "D. Retesting (or Pending Retest)"},

	// testing-m5-assignment (1 questions)
	{"testing-m5-assignment", 1, "C. Sprint Retrospective"},

	// testing-m6-assignment (1 questions)
	{"testing-m6-assignment", 1, "B. pm.test(\"Status is 201\", () => { pm.response.to.have.status(201); });"},

	// testing-m7-assignment (1 questions)
	{"testing-m7-assignment", 1, "A. SELECT * FROM employees WHERE salary > 50000 ORDER BY last_name;"},

	// testing-m8-assignment (1 questions)
	{"testing-m8-assignment", 1, "D. WebDriverWait wait = new WebDriverWait(driver, Duration.ofSeconds(10)); wait.until(ExpectedConditions.elementToBeClickable(locator));"},

	// testing-m9-assignment (1 questions)
	{"testing-m9-assignment", 1, "C. @DataProvider"},
}

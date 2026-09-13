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

// quizAnswerKeys holds 806 answer keys across 143 modules.
var quizAnswerKeys = []quizKey{
	// dm-m1 (5 questions)
	{"dm-m1", 1, "It is a conversion problem, so more traffic will not help"},
	{"dm-m1", 2, "Customer interviews, support tickets and reviews"},
	{"dm-m1", 3, "Whether the split changes what you would actually do"},
	{"dm-m1", 4, "Search captures existing demand; social creates it"},
	{"dm-m1", 5, "Rented reach can be withdrawn by a platform without notice"},

	// dm-m2 (5 questions)
	{"dm-m2", 1, "Inconsistency rather than content quality"},
	{"dm-m2", 2, "The blank-page problem — you never decide what to post from nothing"},
	{"dm-m2", 3, "Keeping people on the platform"},
	{"dm-m2", 4, "It is demoted by platforms and erodes audience trust"},
	{"dm-m2", 5, "It escalates — the deletion becomes the story"},

	// dm-m3 (5 questions)
	{"dm-m3", 1, "So the ad copy and landing page can closely match the query"},
	{"dm-m3", 2, "In queries that should have been excluded by negative keywords"},
	{"dm-m3", 3, "The creative"},
	{"dm-m3", 4, "Gross margin"},
	{"dm-m3", 5, "Automated strategies need conversion volume to learn from"},

	// dm-m4 (5 questions)
	{"dm-m4", 1, "No algorithm decides whether your message is delivered to your list"},
	{"dm-m4", 2, "Solving one specific problem with immediate value"},
	{"dm-m4", 3, "Spam complaints that degrade deliverability for your genuine subscribers too"},
	{"dm-m4", 4, "Abandoned cart"},
	{"dm-m4", 5, "It teaches customers to abandon carts deliberately to get one"},

	// dm-m5 (5 questions)
	{"dm-m5", 1, "Wrong traffic — a targeting or message-match problem"},
	{"dm-m5", 2, "Stopping the test as soon as it looks significant"},
	{"dm-m5", 3, "Use qualitative evidence and judge changes on trend"},
	{"dm-m5", 4, "An unresponsive or broken element"},
	{"dm-m5", 5, "Mask sensitive inputs and disclose it — the recordings are personal data"},

	// frontend-m1 (4 questions)
	{"frontend-m1", 1, "B. HyperText Markup Language"},
	{"frontend-m1", 2, "B. CSS"},
	{"frontend-m1", 3, "C. JavaScript"},
	{"frontend-m1", 4, "C. Database"},

	// frontend-m2 (6 questions)
	{"frontend-m2", 1, "B. HyperText Markup Language"},
	{"frontend-m2", 2, "C. <h1>"},
	{"frontend-m2", 3, "B. <a>"},
	{"frontend-m2", 4, "C. <ul>"},
	{"frontend-m2", 5, "C. <br>"},
	{"frontend-m2", 6, "B. <input type=\"checkbox\">"},

	// frontend-m3 (5 questions)
	{"frontend-m3", 1, "B. Cascading Style Sheets"},
	{"frontend-m3", 2, "C. color"},
	{"frontend-m3", 3, "B. padding"},
	{"frontend-m3", 4, "B. Removes element from the page completely"},
	{"frontend-m3", 5, "C. ID (#main)"},

	// frontend-m4 (5 questions)
	{"frontend-m4", 1, "B. display: flex"},
	{"frontend-m4", 2, "B. Horizontal alignment (main axis)"},
	{"frontend-m4", 3, "B. grid-template-columns"},
	{"frontend-m4", 4, "B. Wraps items to next line when no space"},
	{"frontend-m4", 5, "B. Fraction"},

	// frontend-m5 (4 questions)
	{"frontend-m5", 1, "B. Design for mobile first, then scale up"},
	{"frontend-m5", 2, "B. @media"},
	{"frontend-m5", 3, "B. Controls how page scales on mobile devices"},
	{"frontend-m5", 4, "C. vw"},

	// frontend-m6 (5 questions)
	{"frontend-m6", 1, "C. var"},
	{"frontend-m6", 2, "C. \"object\""},
	{"frontend-m6", 3, "B. ==="},
	{"frontend-m6", 4, "A. push()"},
	{"frontend-m6", 5, "B. Block scoped"},

	// frontend-m7 (5 questions)
	{"frontend-m7", 1, "B. querySelectorAll()"},
	{"frontend-m7", 2, "B. e.preventDefault()"},
	{"frontend-m7", 3, "C. User's browser"},
	{"frontend-m7", 4, "B. elem.classList.add(\"name\")"},
	{"frontend-m7", 5, "B. Session storage clears when tab/browser is closed"},

	// frontend-m8 (5 questions)
	{"frontend-m8", 1, "C. It throws a TypeError"},
	{"frontend-m8", 2, "B. Arrow functions inherit \"this\" from the parent scope"},
	{"frontend-m8", 3, "C. Backticks ``"},
	{"frontend-m8", 4, "B. B = [...A]"},
	{"frontend-m8", 5, "B. extends"},

	// frontend-m9 (5 questions)
	{"frontend-m9", 1, "B. Page UI freezes while task executes"},
	{"frontend-m9", 2, "B. Pending, Resolved, Rejected"},
	{"frontend-m9", 3, "B. using try...catch blocks"},
	{"frontend-m9", 4, "B. response.json()"},
	{"frontend-m9", 5, "C. async"},

	// frontend-m10 (5 questions)
	{"frontend-m10", 1, "B. git init"},
	{"frontend-m10", 2, "A. git add index.html"},
	{"frontend-m10", 3, "B. GitHub is a hosting service for Git repositories"},
	{"frontend-m10", 4, "B. git checkout feature-login"},
	{"frontend-m10", 5, "C. git log"},

	// frontend-m11 (5 questions)
	{"frontend-m11", 1, "B. It updates only changed parts of the UI, improving performance"},
	{"frontend-m11", 2, "B. class"},
	{"frontend-m11", 3, "B. Inside a single parent container (e.g. <div> or Fragment)"},
	{"frontend-m11", 4, "B. Read-only (immutable) from inside the receiving component"},
	{"frontend-m11", 5, "B. npm create vite@latest"},

	// frontend-m12 (5 questions)
	{"frontend-m12", 1, "B. Props are read-only (passed down), State is local (managed inside component)"},
	{"frontend-m12", 2, "A. The current state and a function to update it"},
	{"frontend-m12", 3, "B. The form input value is driven by React state"},
	{"frontend-m12", 4, "B. To help React identify which items have changed, been added, or been removed"},
	{"frontend-m12", 5, "B. &&"},

	// frontend-m13 (5 questions)
	{"frontend-m13", 1, "B. Empty dependency array []"},
	{"frontend-m13", 2, "B. To create mutable references that persist across renders without triggering a re-render"},
	{"frontend-m13", 3, "B. useMemo memoizes computed values, useCallback memoizes function instances"},
	{"frontend-m13", 4, "B. prefix with the word \"use\" (e.g. useFetch)"},
	{"frontend-m13", 5, "B. Return a function inside the effect body"},

	// frontend-m14 (5 questions)
	{"frontend-m14", 1, "B. Page changes happen client-side in the browser without reloading the page"},
	{"frontend-m14", 2, "B. <Link>"},
	{"frontend-m14", 3, "A. useParams() hook"},
	{"frontend-m14", 4, "B. <Outlet />"},
	{"frontend-m14", 5, "A. useNavigate() hook"},

	// frontend-m15 (5 questions)
	{"frontend-m15", 1, "B. useEffect"},
	{"frontend-m15", 2, "A. Fetch doesn't throw errors on 404 or 500 status codes"},
	{"frontend-m15", 3, "B. POST"},
	{"frontend-m15", 4, "B. LocalStorage or HTTP-only cookies"},
	{"frontend-m15", 5, "B. Communicating request status to the user to improve UX"},

	// frontend-m16 (5 questions)
	{"frontend-m16", 1, "B. Passing props through multiple nested components that don't need them, just to reach a deep child component"},
	{"frontend-m16", 2, "B. Dispatching an action object"},
	{"frontend-m16", 3, "B. Zustand is lightweight and does not require Provider wrapper setups"},
	{"frontend-m16", 4, "B. React.lazy() & Suspense"},
	{"frontend-m16", 5, "B. Provider & Consumer"},

	// frontend-m17 (5 questions)
	{"frontend-m17", 1, "A. Transpiles, minifies, and tree-shakes code files into a compact build bundle"},
	{"frontend-m17", 2, "B. VITE_"},
	{"frontend-m17", 3, "B. A & CNAME"},
	{"frontend-m17", 4, "B. Between 50 to 160 characters"},
	{"frontend-m17", 5, "B. Vercel"},

	// frontend-assessment (4 questions)
	{"frontend-assessment", 1, "B. Margin, Border, Padding, Content"},
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
	{"genai-m2", 8, "Return 0.0 for the affected metrics instead of dividing by zero"},

	// genai-m3 (8 questions)
	{"genai-m3", 1, "System prompt, history, context and the response together"},
	{"genai-m3", 2, "At or near 0"},
	{"genai-m3", 3, "Attention cost grows quadratically with sequence length"},
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
	{"genai-m8", 6, "About 800ms"},
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
	{"genai-m9", 8, "Regression outside the training distribution"},

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
	{"genai-m12", 6, "The restriction cannot be enforced during retrieval at all"},
	{"genai-m12", 7, "One document type and one team, shipped in weeks and measurable"},
	{"genai-m12", 8, "Naming it first is what makes your other numbers credible"},

	// golang-m1 (1 questions)
	{"golang-m1", 1, "go run"},

	// golang-m2 (1 questions)
	{"golang-m2", 1, "%T"},

	// golang-m3 (1 questions)
	{"golang-m3", 1, "for"},

	// golang-m4 (1 questions)
	{"golang-m4", 1, "Using _ blank identifier"},

	// golang-m5 (1 questions)
	{"golang-m5", 1, "append"},

	// golang-m6 (1 questions)
	{"golang-m6", 1, "Capitalize the first letter"},

	// golang-m7 (1 questions)
	{"golang-m7", 1, "None (implicit)"},

	// golang-m8 (1 questions)
	{"golang-m8", 1, "LIFO (Last In First Out)"},

	// golang-m9 (1 questions)
	{"golang-m9", 1, "go.mod"},

	// golang-m10 (1 questions)
	{"golang-m10", 1, "encoding/json"},

	// golang-m11 (1 questions)
	{"golang-m11", 1, "go run -race"},

	// golang-m12 (1 questions)
	{"golang-m12", 1, "_test.go"},

	// golang-m13 (1 questions)
	{"golang-m13", 1, "net/http"},

	// golang-m14 (1 questions)
	{"golang-m14", 1, "To prevent SQL Injection"},

	// golang-m15 (1 questions)
	{"golang-m15", 1, "binding"},

	// golang-m16 (1 questions)
	{"golang-m16", 1, "bcrypt"},

	// golang-m17 (1 questions)
	{"golang-m17", 1, "Protocol Buffers"},

	// golang-m18 (1 questions)
	{"golang-m18", 1, "Generates a single self-contained binary"},

	// java-m1 (20 questions)
	{"java-m1", 1, "D. James Gosling"},
	{"java-m1", 2, "C. Oak"},
	{"java-m1", 3, "A. Write Once, Run Anywhere (WORA)"},
	{"java-m1", 4, "B. JRE and development tools like 'javac'"},
	{"java-m1", 5, "B. Java Virtual Machine (JVM)"},
	{"java-m1", 6, "C. .class"},
	{"java-m1", 7, "C. Java Bytecode is platform-independent, but the JVM is platform-dependent."},
	{"java-m1", 8, "A. Garbage Collection"},
	{"java-m1", 9, "A. public static void main(String[] args)"},
	{"java-m1", 10, "C. Welcome.java"},
	{"java-m1", 11, "B. Providing an all-in-one text editor, build automation tool, and debugger"},
	{"java-m1", 12, "A. javac Test.java"},
	{"java-m1", 13, "B. The method does not return any value when it finishes executing."},
	{"java-m1", 14, "A. Strong type checking and exception handling mechanisms"},
	{"java-m1", 15, "A. java Demo"},
	{"java-m1", 16, "C. PATH"},
	{"java-m1", 17, "D. System.out.println(\"Hello World\");"},
	{"java-m1", 18, "B. To enable the JVM to call the method without creating an instance of the class first"},
	{"java-m1", 19, "B. The JDK is a superset that includes the complete JRE plus development tools."},
	{"java-m1", 20, "A. They mark the beginning of a single-line text comment."},

	// java-m2 (20 questions)
	{"java-m2", 1, "D. _variable$5"},
	{"java-m2", 2, "A. String"},
	{"java-m2", 3, "C. Widening casting happens automatically; narrowing casting must be done manually."},
	{"java-m2", 4, "B. /** Documentation comment */"},
	{"java-m2", 5, "B. next() reads input up to the next whitespace delimiter, while nextLine() reads the entire line until a newline character."},
	{"java-m2", 6, "A. %f"},
	{"java-m2", 7, "B. They skip evaluating the second condition if the overall result is already determined by the first condition."},
	{"java-m2", 8, "B. a=7, b=12"},
	{"java-m2", 9, "B. 2.0"},
	{"java-m2", 10, "D. 9"},
	{"java-m2", 11, "C. Output: 1020"},
	{"java-m2", 12, "C. 30 Output"},
	{"java-m2", 13, "A. -1"},
	{"java-m2", 14, "B. 10"},
	{"java-m2", 15, "D. -128"},
	{"java-m2", 16, "D. 5.68"},
	{"java-m2", 17, "A. n1=20, n2=10"},
	{"java-m2", 18, "D. 30"},
	{"java-m2", 19, "A. true"},
	{"java-m2", 20, "A. B"},

	// java-m3 (20 questions)
	{"java-m3", 1, "C. double"},
	{"java-m3", 2, "C. The program falls through, executing subsequent case blocks sequentially until a break or the end of the switch is encountered."},
	{"java-m3", 3, "A. The inner 'if' condition is evaluated only if the outer 'if' condition evaluates to true."},
	{"java-m3", 4, "A. 3"},
	{"java-m3", 5, "C. Passed"},
	{"java-m3", 6, "C. Block 2"},
	{"java-m3", 7, "B. Set Go Done"},
	{"java-m3", 8, "B. High"},
	{"java-m3", 9, "B. Divisible"},
	{"java-m3", 10, "D. 10"},
	{"java-m3", 11, "D. Compilation Error"},
	{"java-m3", 12, "C. Off"},
	{"java-m3", 13, "A. 20"},
	{"java-m3", 14, "C. Allowed"},
	{"java-m3", 15, "B. Five"},
	{"java-m3", 16, "D. Default One"},
	{"java-m3", 17, "A. Warm"},
	{"java-m3", 18, "B. 6"},
	{"java-m3", 19, "B. 8"},
	{"java-m3", 20, "D. Point 3"},

	// java-m4 (20 questions)
	{"java-m4", 1, "C. do-while loop"},
	{"java-m4", 2, "B. continue"},
	{"java-m4", 3, "B. Infinite loop"},
	{"java-m4", 4, "C. O(N^2)"},
	{"java-m4", 5, "C. semicolon (;)"},
	{"java-m4", 6, "B. for"},
	{"java-m4", 7, "B. Modify the array structure while iterating"},
	{"java-m4", 8, "B. 5"},
	{"java-m4", 9, "B. Exits only the innermost loop"},
	{"java-m4", 10, "A. A labelled break"},
	{"java-m4", 11, "B. 02"},
	{"java-m4", 12, "B. Initialization"},
	{"java-m4", 13, "C. Infinite loop"},
	{"java-m4", 14, "B. Only inside that loop"},
	{"java-m4", 15, "C. do-while"},
	{"java-m4", 16, "B. 12"},
	{"java-m4", 17, "B. 123"},
	{"java-m4", 18, "C. return"},
	{"java-m4", 19, "A. Forgetting to update the loop variable"},
	{"java-m4", 20, "B. enhanced for (for-each)"},

	// java-m5 (20 questions)
	{"java-m5", 1, "C. void"},
	{"java-m5", 2, "B. StackOverflowError"},
	{"java-m5", 3, "B. No"},
	{"java-m5", 4, "B. Pass by value"},
	{"java-m5", 5, "C. Overriding resolution at runtime"},
	{"java-m5", 6, "A. Same name, different parameter lists"},
	{"java-m5", 7, "B. No, it is a compile error"},
	{"java-m5", 8, "B. A base case"},
	{"java-m5", 9, "B. By value (a copy)"},
	{"java-m5", 10, "B. The reference value"},
	{"java-m5", 11, "C. Nothing"},
	{"java-m5", 12, "B. static"},
	{"java-m5", 13, "B. No, it has no instance context"},
	{"java-m5", 14, "B. Only that method"},
	{"java-m5", 15, "A. Zero or more arguments of that type"},
	{"java-m5", 16, "C. Last"},
	{"java-m5", 17, "B. 1"},
	{"java-m5", 18, "B. Iteration"},
	{"java-m5", 19, "B. The local variable wins inside that scope"},
	{"java-m5", 20, "C. private"},

	// java-m6 (20 questions)
	{"java-m6", 1, "B. 2"},
	{"java-m6", 2, "B. length"},
	{"java-m6", 3, "C. ArrayIndexOutOfBoundsException"},
	{"java-m6", 4, "B. Arrays.sort()"},
	{"java-m6", 5, "B. No"},
	{"java-m6", 6, "B. 0"},
	{"java-m6", 7, "B. null"},
	{"java-m6", 8, "B. ArrayIndexOutOfBoundsException"},
	{"java-m6", 9, "C. data.length"},
	{"java-m6", 10, "B. No, it is fixed"},
	{"java-m6", 11, "A. Arrays.sort()"},
	{"java-m6", 12, "B. Returns a readable representation of the array"},
	{"java-m6", 13, "A. O(1)"},
	{"java-m6", 14, "A. int[][] grid = new int[3][4];"},
	{"java-m6", 15, "B. A 2D array whose rows have different lengths"},
	{"java-m6", 16, "A. The array must be sorted"},
	{"java-m6", 17, "A. Copies a range of elements between arrays"},
	{"java-m6", 18, "B. No, it copies references (shallow)"},
	{"java-m6", 19, "C. O(n) because elements must shift"},
	{"java-m6", 20, "B. ArrayList"},

	// java-m7 (20 questions)
	{"java-m7", 1, "C. String Constant Pool (SCP)"},
	{"java-m7", 2, "B. str1.equals(str2)"},
	{"java-m7", 3, "C. StringBuilder"},
	{"java-m7", 4, "B. \"bc\""},
	{"java-m7", 5, "A. For security, caching, and thread safety"},
	{"java-m7", 6, "A. For caching, thread safety and security"},
	{"java-m7", 7, "B. Their reference addresses"},
	{"java-m7", 8, "B. equals()"},
	{"java-m7", 9, "C. StringBuffer"},
	{"java-m7", 10, "B. StringBuilder"},
	{"java-m7", 11, "B. 'J'"},
	{"java-m7", 12, "A. \"hi\""},
	{"java-m7", 13, "B. \"42\""},
	{"java-m7", 14, "B. A String array of length 3"},
	{"java-m7", 15, "A. The String constant pool"},
	{"java-m7", 16, "B. A distinct object on the heap"},
	{"java-m7", 17, "B. equalsIgnoreCase()"},
	{"java-m7", 18, "A. \"el\""},
	{"java-m7", 19, "B. Each concatenation creates a new String object"},
	{"java-m7", 20, "B. 2"},

	// java-m8 (20 questions)
	{"java-m8", 1, "D. Abstraction"},
	{"java-m8", 2, "B. this"},
	{"java-m8", 3, "B. No"},
	{"java-m8", 4, "C. implements"},
	{"java-m8", 5, "B. Method Overloading"},
	{"java-m8", 6, "B. Encapsulation"},
	{"java-m8", 7, "B. To initialise a new object"},
	{"java-m8", 8, "C. Nothing, it has no return type"},
	{"java-m8", 9, "B. Java provides a default no-arg constructor"},
	{"java-m8", 10, "B. The current object instance"},
	{"java-m8", 11, "B. extends"},
	{"java-m8", 12, "A. One"},
	{"java-m8", 13, "B. Interfaces"},
	{"java-m8", 14, "B. Overloading"},
	{"java-m8", 15, "B. Method overriding"},
	{"java-m8", 16, "A. Yes"},
	{"java-m8", 17, "B. No"},
	{"java-m8", 18, "B. protected"},
	{"java-m8", 19, "A. Calls the superclass constructor"},
	{"java-m8", 20, "B. default and static methods"},

	// java-m9 (20 questions)
	{"java-m9", 1, "C. finally"},
	{"java-m9", 2, "B. throws"},
	{"java-m9", 3, "A. Throwable"},
	{"java-m9", 4, "B. Unchecked"},
	{"java-m9", 5, "B. Extend Exception"},
	{"java-m9", 6, "B. Throwable"},
	{"java-m9", 7, "B. IOException"},
	{"java-m9", 8, "C. NullPointerException"},
	{"java-m9", 9, "C. Almost always, whether or not an exception occurred"},
	{"java-m9", 10, "A. throw raises an exception; throws declares one"},
	{"java-m9", 11, "B. Most specific first"},
	{"java-m9", 12, "B. Declared AutoCloseable resources are closed"},
	{"java-m9", 13, "A. ArithmeticException"},
	{"java-m9", 14, "B. Infinity"},
	{"java-m9", 15, "C. RuntimeException"},
	{"java-m9", 16, "A. NumberFormatException"},
	{"java-m9", 17, "B. No, they signal unrecoverable JVM conditions"},
	{"java-m9", 18, "B. It silently swallows failures"},
	{"java-m9", 19, "B. Yes, if it has finally or is try-with-resources"},
	{"java-m9", 20, "A. The original cause of the failure"},

	// java-m10 (20 questions)
	{"java-m10", 1, "C. HashSet"},
	{"java-m10", 2, "B. HashMap"},
	{"java-m10", 3, "C. TreeSet"},
	{"java-m10", 4, "B. map.containsKey()"},
	{"java-m10", 5, "B. ConcurrentModificationException"},
	{"java-m10", 6, "B. Set"},
	{"java-m10", 7, "B. HashMap"},
	{"java-m10", 8, "C. TreeMap"},
	{"java-m10", 9, "C. LinkedHashMap"},
	{"java-m10", 10, "B. No, it is a separate hierarchy"},
	{"java-m10", 11, "A. O(1)"},
	{"java-m10", 12, "B. O(n)"},
	{"java-m10", 13, "B. equals() and hashCode()"},
	{"java-m10", 14, "B. Both are stored in the same bucket"},
	{"java-m10", 15, "A. ConcurrentModificationException"},
	{"java-m10", 16, "B. iterator.remove()"},
	{"java-m10", 17, "C. ConcurrentHashMap"},
	{"java-m10", 18, "B. A fixed-size list backed by the array"},
	{"java-m10", 19, "B. Comparable"},
	{"java-m10", 20, "B. equals() and hashCode()"},

	// java-m11 (20 questions)
	{"java-m11", 1, "B. BufferedReader"},
	{"java-m11", 2, "B. Try-with-resources"},
	{"java-m11", 3, "B. file.exists()"},
	{"java-m11", 4, "B. new FileWriter(\"file.txt\", true)"},
	{"java-m11", 5, "C. java.io"},
	{"java-m11", 6, "B. InputStream/OutputStream"},
	{"java-m11", 7, "C. -1"},
	{"java-m11", 8, "B. null"},
	{"java-m11", 9, "B. new FileWriter(path, true)"},
	{"java-m11", 10, "B. Creates only an object representing a path"},
	{"java-m11", 11, "B. file.createNewFile()"},
	{"java-m11", 12, "B. It batches writes in memory before hitting disk"},
	{"java-m11", 13, "B. The platform-specific line separator"},
	{"java-m11", 14, "B. Buffered data may never reach disk"},
	{"java-m11", 15, "B. Reverse declaration order"},
	{"java-m11", 16, "B. AutoCloseable"},
	{"java-m11", 17, "A. IOException"},
	{"java-m11", 18, "B. So the -1 end-of-stream sentinel can be represented"},
	{"java-m11", 19, "B. BufferedReader"},
	{"java-m11", 20, "B. The JVM working directory"},

	// java-m12 (20 questions)
	{"java-m12", 1, "B. start()"},
	{"java-m12", 2, "C. synchronized"},
	{"java-m12", 3, "B. Java supports multiple interface implementations but only single class inheritance"},
	{"java-m12", 4, "B. Runnable"},
	{"java-m12", 5, "B. Executors"},
	{"java-m12", 6, "B. Executes synchronously on the current thread"},
	{"java-m12", 7, "B. IllegalThreadStateException"},
	{"java-m12", 8, "B. It leaves the single inheritance slot free"},
	{"java-m12", 9, "C. Six"},
	{"java-m12", 10, "B. BLOCKED"},
	{"java-m12", 11, "B. wait()"},
	{"java-m12", 12, "A. It is a read-modify-write sequence, not atomic"},
	{"java-m12", 13, "B. Visibility of reads and writes across threads"},
	{"java-m12", 14, "B. AtomicInteger"},
	{"java-m12", 15, "B. Blocks the caller until the target thread finishes"},
	{"java-m12", 16, "B. Two threads each holding a lock the other needs"},
	{"java-m12", 17, "B. Always acquire locks in the same global order"},
	{"java-m12", 18, "A. Threads are expensive to create and consume ~1MB stack each"},
	{"java-m12", 19, "B. Callable"},
	{"java-m12", 20, "B. Its non-daemon threads keep the JVM alive"},

	// java-m13 (20 questions)
	{"java-m13", 1, "B. @FunctionalInterface"},
	{"java-m13", 2, "C. ::"},
	{"java-m13", 3, "C. reduce()"},
	{"java-m13", 4, "A. Optional.empty()"},
	{"java-m13", 5, "A. Intermediate"},
	{"java-m13", 6, "A. An interface with exactly one abstract method"},
	{"java-m13", 7, "B. @FunctionalInterface"},
	{"java-m13", 8, "C. final or effectively final"},
	{"java-m13", 9, "B. Predicate"},
	{"java-m13", 10, "A. Function"},
	{"java-m13", 11, "C. Consumer"},
	{"java-m13", 12, "B. Supplier"},
	{"java-m13", 13, "A. Source, intermediate operations, terminal operation"},
	{"java-m13", 14, "B. Nothing executes"},
	{"java-m13", 15, "B. No, it throws IllegalStateException"},
	{"java-m13", 16, "C. collect"},
	{"java-m13", 17, "B. An unbound instance method reference"},
	{"java-m13", 18, "A. A constructor reference"},
	{"java-m13", 19, "B. orElse always evaluates its argument; orElseGet is lazy"},
	{"java-m13", 20, "B. As a return type"},

	// java-m14 (20 questions)
	{"java-m14", 1, "C. ResultSet"},
	{"java-m14", 2, "B. They prevent SQL Injection and cache query execution plans"},
	{"java-m14", 3, "B. executeQuery()"},
	{"java-m14", 4, "B. jdbc:mysql://..."},
	{"java-m14", 5, "B. 1"},
	{"java-m14", 6, "B. A standard Java API for relational database access"},
	{"java-m14", 7, "D. Type 4"},
	{"java-m14", 8, "B. 1"},
	{"java-m14", 9, "B. PreparedStatement"},
	{"java-m14", 10, "B. The query structure is compiled before values are bound"},
	{"java-m14", 11, "B. executeUpdate()"},
	{"java-m14", 12, "B. The number of affected rows"},
	{"java-m14", 13, "B. Before the first row"},
	{"java-m14", 14, "B. Call rs.wasNull()"},
	{"java-m14", 15, "B. No, it dies when the statement closes"},
	{"java-m14", 16, "B. true"},
	{"java-m14", 17, "B. rollback()"},
	{"java-m14", 18, "A. RETURN_GENERATED_KEYS"},
	{"java-m14", 19, "A. addBatch() and executeBatch()"},
	{"java-m14", 20, "B. No, give each thread its own"},

	// java-m15 (20 questions)
	{"java-m15", 1, "B. O(log N)"},
	{"java-m15", 2, "B. Stack"},
	{"java-m15", 3, "B. Breadth First Search (BFS)"},
	{"java-m15", 4, "C. O(N^2)"},
	{"java-m15", 5, "B. Queue"},
	{"java-m15", 6, "B. O(log n)"},
	{"java-m15", 7, "A. The data must be sorted"},
	{"java-m15", 8, "B. To avoid integer overflow"},
	{"java-m15", 9, "B. In-order"},
	{"java-m15", 10, "B. It degenerates into a linked list with O(n) operations"},
	{"java-m15", 11, "B. TreeMap and TreeSet"},
	{"java-m15", 12, "B. Queue"},
	{"java-m15", 13, "B. Stack (or recursion)"},
	{"java-m15", 14, "B. BFS"},
	{"java-m15", 15, "B. Cycles would cause an infinite loop"},
	{"java-m15", 16, "B. Merge sort"},
	{"java-m15", 17, "B. O(n^2)"},
	{"java-m15", 18, "B. Equal elements keep their relative order"},
	{"java-m15", 19, "B. TimSort"},
	{"java-m15", 20, "B. ArrayDeque used as a stack"},

	// java-m16 (20 questions)
	{"java-m16", 1, "B. @RestController"},
	{"java-m16", 2, "B. Spring Initializr"},
	{"java-m16", 3, "B. @Autowired"},
	{"java-m16", 4, "C. Tomcat"},
	{"java-m16", 5, "B. JpaRepository"},
	{"java-m16", 6, "A. @Configuration, @EnableAutoConfiguration, @ComponentScan"},
	{"java-m16", 7, "B. In the root package above your components"},
	{"java-m16", 8, "A. @ResponseBody on every method"},
	{"java-m16", 9, "B. @PathVariable"},
	{"java-m16", 10, "B. @RequestParam"},
	{"java-m16", 11, "B. 201"},
	{"java-m16", 12, "C. 204"},
	{"java-m16", 13, "A. GET, PUT, DELETE"},
	{"java-m16", 14, "B. Constructor injection"},
	{"java-m16", 15, "B. Fields cannot be final and testing without the framework is hard"},
	{"java-m16", 16, "B. singleton"},
	{"java-m16", 17, "A. It should be stateless"},
	{"java-m16", 18, "B. @Qualifier"},
	{"java-m16", 19, "B. Unchecked exceptions only"},
	{"java-m16", 20, "A. The proxy is bypassed by self-invocation"},

	// java-m17 (20 questions)
	{"java-m17", 1, "B. @Id"},
	{"java-m17", 2, "B. @Entity"},
	{"java-m17", 3, "B. @Valid"},
	{"java-m17", 4, "C. @RestControllerAdvice"},
	{"java-m17", 5, "B. spring.jpa.hibernate.ddl-auto=update"},
	{"java-m17", 6, "A. JPA is the specification, Hibernate an implementation"},
	{"java-m17", 7, "C. validate"},
	{"java-m17", 8, "B. HikariCP"},
	{"java-m17", 9, "B. @Entity"},
	{"java-m17", 10, "B. STRING"},
	{"java-m17", 11, "B. Reordering the enum silently changes stored meanings"},
	{"java-m17", 12, "A. The owning side"},
	{"java-m17", 13, "B. EAGER"},
	{"java-m17", 14, "B. LAZY"},
	{"java-m17", 15, "A. Touching a lazy association on a detached entity"},
	{"java-m17", 16, "B. Hibernate flushing changed managed entities at commit"},
	{"java-m17", 17, "B. No, dirty checking handles it"},
	{"java-m17", 18, "A. One query per collection element instead of one overall"},
	{"java-m17", 19, "B. @Valid on the @RequestBody parameter"},
	{"java-m17", 20, "B. @RestControllerAdvice"},

	// java-m18 (20 questions)
	{"java-m18", 1, "B. BCryptPasswordEncoder"},
	{"java-m18", 2, "B. JSON Web Token"},
	{"java-m18", 3, "B. Authorization Header"},
	{"java-m18", 4, "B. csrf().disable()"},
	{"java-m18", 5, "B. Authorization"},
	{"java-m18", 6, "A. AuthN is who you are; AuthZ is what you may do"},
	{"java-m18", 7, "B. 401"},
	{"java-m18", 8, "B. 403"},
	{"java-m18", 9, "A. header.payload.signature"},
	{"java-m18", 10, "B. No, it is only Base64URL encoded and readable by anyone"},
	{"java-m18", 11, "B. Integrity"},
	{"java-m18", 12, "B. They cannot easily be revoked before expiry"},
	{"java-m18", 13, "A. Hashing is one-way; the system never needs the plaintext"},
	{"java-m18", 14, "B. They are too fast, enabling rapid brute force"},
	{"java-m18", 15, "B. Generates a unique random salt per password"},
	{"java-m18", 16, "B. encoder.matches(raw, hash)"},
	{"java-m18", 17, "B. Each call uses a new random salt"},
	{"java-m18", 18, "B. For a stateless API authenticated by an Authorization header"},
	{"java-m18", 19, "B. ROLE_ADMIN"},
	{"java-m18", 20, "A. Top to bottom, first match wins"},

	// java-m19 (20 questions)
	{"java-m19", 1, "B. mvn clean package"},
	{"java-m19", 2, "B. java -jar app.jar"},
	{"java-m19", 3, "A. docker build"},
	{"java-m19", 4, "B. target/"},
	{"java-m19", 5, "C. Passed via Environment Variables"},
	{"java-m19", 6, "B. An executable fat JAR with dependencies and an embedded server"},
	{"java-m19", 7, "B. no main manifest attribute"},
	{"java-m19", 8, "A. java -jar app.jar"},
	{"java-m19", 9, "B. Command-line arguments"},
	{"java-m19", 10, "A. SPRING_DATASOURCE_URL"},
	{"java-m19", 11, "B. Use DB_URL, or the H2 URL as a fallback"},
	{"java-m19", 12, "A. --spring.profiles.active=prod"},
	{"java-m19", 13, "B. 5000"},
	{"java-m19", 14, "B. Through the PORT environment variable"},
	{"java-m19", 15, "B. It is ephemeral and wiped on redeploy"},
	{"java-m19", 16, "B. A much smaller runtime image containing only the JRE and JAR"},
	{"java-m19", 17, "B. So the dependency layer stays cached when only source changes"},
	{"java-m19", 18, "B. A compromise would inherit full privileges"},
	{"java-m19", 19, "B. They are permanently visible in the image history"},
	{"java-m19", 20, "B. By the service name, e.g. jdbc:postgresql://db:5432/..."},

	// java-m1-assignment (6 questions)
	{"java-m1-assignment", 1, "C. Java Development Kit (JDK)"},
	{"java-m1-assignment", 2, "B. The compiled bytecode (.class) is platform-neutral and can run on any JVM."},
	{"java-m1-assignment", 3, "B. The JVM is platform-dependent; a specific version must be installed for each OS."},
	{"java-m1-assignment", 4, "B. javac App.java"},
	{"java-m1-assignment", 5, "A. java App"},
	{"java-m1-assignment", 6, "B. .class"},

	// java-m10-assignment (1 questions)
	{"java-m10-assignment", 1, "B. When you need the keys to be maintained in a sorted order."},

	// java-m11-assignment (1 questions)
	{"java-m11-assignment", 1, "C. It buffers input for efficient reading, reducing the number of costly system/disk read operations."},

	// java-m12-assignment (1 questions)
	{"java-m12-assignment", 1, "B. start() creates a new thread and executes run() asynchronously in it; calling run() directly runs the code synchronously in the current thread."},

	// java-m14-assignment (1 questions)
	{"java-m14-assignment", 1, "C. It manages the list of database drivers, matches connection requests with the appropriate driver, and establishes the connection."},

	// java-m16-assignment (1 questions)
	{"java-m16-assignment", 1, "B. Constructor injection allows the class to declare dependencies as final (immutable), enforces required dependencies, and simplifies unit testing."},

	// java-m17-assignment (1 questions)
	{"java-m17-assignment", 1, "B. JPA is the specification (guidelines/interface); Hibernate is a concrete provider (implementation) of the JPA specification."},

	// java-m18-assignment (2 questions)
	{"java-m18-assignment", 1, "A. The Authorization header (using Bearer scheme)."},
	{"java-m18-assignment", 2, "B. BCrypt is a slow, adaptive hashing algorithm that makes brute-force attacks much harder; SHA-256 is extremely fast and vulnerable to hardware-accelerated cracking."},

	// java-m19-assignment (1 questions)
	{"java-m19-assignment", 1, "B. It removes the target directory (compiled classes, packaged files) to ensure a fresh, full build."},

	// java-m2-assignment (1 questions)
	{"java-m2-assignment", 1, "A. Widening is done automatically when converting a smaller type to a larger type; narrowing must be done manually."},

	// java-m4-assignment (1 questions)
	{"java-m4-assignment", 1, "A. break terminates the loop entirely; continue skips the current iteration and moves to the next one."},

	// java-m7-assignment (1 questions)
	{"java-m7-assignment", 1, "B. The \"==\" operator compares memory references (addresses), not the actual contents."},

	// java-m8-assignment (1 questions)
	{"java-m8-assignment", 1, "C. An abstract class can have instance fields and constructors; an interface cannot have instance fields or constructors."},

	// java-m9-assignment (1 questions)
	{"java-m9-assignment", 1, "B. throw is used to explicitly throw a single exception instance; throws is used in method signatures to declare exceptions that might be thrown."},

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

	// seo-m1 (5 questions)
	{"seo-m1", 1, "Crawling, indexing, serving"},
	{"seo-m1", 2, "Whether the page is indexed at all"},
	{"seo-m1", 3, "Crawling, not indexing"},
	{"seo-m1", 4, "Google cannot read the noindex tag on a page it is forbidden to crawl"},
	{"seo-m1", 5, "Large sites with tens of thousands of URLs"},

	// seo-m2 (5 questions)
	{"seo-m2", 1, "Look at what currently ranks on page one"},
	{"seo-m2", 2, "Commercial"},
	{"seo-m2", 3, "Lower competition and more specific intent make them realistically rankable"},
	{"seo-m2", 4, "Multiple pages targeting the same intent and competing with each other"},
	{"seo-m2", 5, "Business value"},

	// seo-m3 (5 questions)
	{"seo-m3", 1, "No, but it affects click-through rate"},
	{"seo-m3", 2, "Exactly one"},
	{"seo-m3", 3, "Google judged your title unhelpful, stuffed or mismatched"},
	{"seo-m3", 4, "Accessibility for screen reader users"},
	{"seo-m3", 5, "It distributes authority and aids discovery, entirely under your control"},

	// seo-m4 (5 questions)
	{"seo-m4", 1, "CLS — Cumulative Layout Shift"},
	{"seo-m4", 2, "Field data from real users"},
	{"seo-m4", 3, "No, but it can qualify a page for rich results"},
	{"seo-m4", 4, "When a page has permanently moved and the old URL should no longer be used"},
	{"seo-m4", 5, "Leaving a site-wide noindex tag in place at launch"},

	// seo-m5 (5 questions)
	{"seo-m5", 1, "A third-party vendor estimate that Google does not use"},
	{"seo-m5", 2, "Topical relevance and editorial placement"},
	{"seo-m5", 3, "Not to pass ranking signal through the link"},
	{"seo-m5", 4, "Whether you would still publish the piece if the link were nofollow"},
	{"seo-m5", 5, "Only for a manual action or documented negative SEO — most sites never need it"},

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

	// testing-m1 (2 questions)
	{"testing-m1", 1, "B. Test Planning"},
	{"testing-m1", 2, "B. To find defects and verify expected behavior"},

	// testing-m2 (2 questions)
	{"testing-m2", 1, "B. Testing is context dependent"},
	{"testing-m2", 2, "C. Acceptance Testing"},

	// testing-m3 (2 questions)
	{"testing-m3", 1, "A. 5, 6, 7, 11, 12, 13"},
	{"testing-m3", 2, "B. A high-level, static project or organizational policy document defining testing approaches"},

	// testing-m4 (2 questions)
	{"testing-m4", 1, "B. Invalid/Rejected"},
	{"testing-m4", 2, "B. Low Severity, High Priority"},

	// testing-m5 (2 questions)
	{"testing-m5", 1, "C. Product Owner"},
	{"testing-m5", 2, "B. 15 minutes"},

	// testing-m6 (2 questions)
	{"testing-m6", 1, "B. POST"},
	{"testing-m6", 2, "B. Unauthorized (Authentication failed)"},

	// testing-m7 (2 questions)
	{"testing-m7", 1, "B. LEFT JOIN"},
	{"testing-m7", 2, "C. UPDATE"},

	// testing-m8 (2 questions)
	{"testing-m8", 1, "B. quit()"},
	{"testing-m8", 2, "B. blocks execution for a fixed duration, slowing down tests unnecessarily"},

	// testing-m9 (2 questions)
	{"testing-m9", 1, "B. pom.xml"},
	{"testing-m9", 2, "B. Decoupling test code from webpage UI selectors, reducing maintenance costs"},

	// testing-m10 (2 questions)
	{"testing-m10", 1, "C. Endurance (Soak) Testing"},
	{"testing-m10", 2, "B. Time taken for a request to travel from client to server and return the first byte"},

	// testing-m11 (2 questions)
	{"testing-m11", 1, "B. ADB (Android Debug Bridge)"},
	{"testing-m11", 2, "B. It is cross-platform, letting you use the same API for Android and iOS tests"},

	// testing-m12 (2 questions)
	{"testing-m12", 1, "A. Open Web Application Security Project"},
	{"testing-m12", 2, "B. SQL Injection"},

	// testing-m13 (2 questions)
	{"testing-m13", 1, "B. git checkout -b"},
	{"testing-m13", 2, "B. Jenkinsfile"},

	// testing-m14 (2 questions)
	{"testing-m14", 1, "B. Automatically updating selectors when DOM elements change, reducing maintenance"},
	{"testing-m14", 2, "B. It compares screenshots using machine learning to detect visual deviations, regardless of HTML changes"},

	// testing-m1-assignment (1 questions)
	{"testing-m1-assignment", 1, "B. Requirements Analysis -> Test Planning -> Test Case Development -> Environment Setup -> Test Execution -> Test Closure"},

	// testing-m10-assignment (1 questions)
	{"testing-m10-assignment", 1, "B. Throughput"},

	// testing-m11-assignment (1 questions)
	{"testing-m11-assignment", 1, "C. Hybrid App"},

	// testing-m12-assignment (1 questions)
	{"testing-m12-assignment", 1, "B. Cross-Site Scripting (XSS)"},

	// testing-m13-assignment (1 questions)
	{"testing-m13-assignment", 1, "B. git pull"},

	// testing-m14-assignment (1 questions)
	{"testing-m14-assignment", 1, "B. Applitools Eyes"},

	// testing-m15-assignment (1 questions)
	{"testing-m15-assignment", 1, "B. To combine POM, Data-Driven testing, custom logging, and visual HTML reporting into an extensible, reusable test engine."},

	// testing-m2-assignment (1 questions)
	{"testing-m2-assignment", 1, "B. Verification evaluates static documents (reviews/walkthroughs); Validation executes the active code to verify system behavior."},

	// testing-m3-assignment (1 questions)
	{"testing-m3-assignment", 1, "B. 9, 10, 11, 49, 50, 51"},

	// testing-m4-assignment (1 questions)
	{"testing-m4-assignment", 1, "C. Retesting (or Pending Retest)"},

	// testing-m5-assignment (1 questions)
	{"testing-m5-assignment", 1, "B. Sprint Retrospective"},

	// testing-m6-assignment (1 questions)
	{"testing-m6-assignment", 1, "B. pm.test(\"Status is 201\", () => { pm.response.to.have.status(201); });"},

	// testing-m7-assignment (1 questions)
	{"testing-m7-assignment", 1, "B. SELECT * FROM employees WHERE salary > 50000 ORDER BY last_name;"},

	// testing-m8-assignment (1 questions)
	{"testing-m8-assignment", 1, "B. WebDriverWait wait = new WebDriverWait(driver, Duration.ofSeconds(10)); wait.until(ExpectedConditions.elementToBeClickable(locator));"},

	// testing-m9-assignment (1 questions)
	{"testing-m9-assignment", 1, "B. @DataProvider"},
}

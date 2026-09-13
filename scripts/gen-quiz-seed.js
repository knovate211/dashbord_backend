#!/usr/bin/env node
/**
 * Regenerates services/user-service/internal/repository/quiz_seed_data.go from
 * the course content data files in skillofied-app.
 *
 * The server grades quiz submissions against the quiz_keys table, so those keys
 * must stay in sync with the questions the client renders. Run this whenever
 * quiz questions are added, removed, or their correct answers change.
 *
 *   node scripts/gen-quiz-seed.js
 *
 * Module IDs are namespaced by course ("java-m1", "sql-m1", "frontend-m1")
 * because all three courses number their modules from m1, while quiz_keys is
 * keyed on (module_id, question_id).
 */
const fs = require('fs');
const path = require('path');

const BACKEND_ROOT = path.resolve(__dirname, '..');
const REPO_ROOT = path.resolve(BACKEND_ROOT, '..');
const APP_ROOT = path.join(REPO_ROOT, 'skillofied-app');
const COURSES = path.join(APP_ROOT, 'src/components/courses');
const DEST = path.join(BACKEND_ROOT, 'services/user-service/internal/repository/quiz_seed_data.go');

let esbuild;
try {
  esbuild = require(path.join(APP_ROOT, 'node_modules/esbuild'));
} catch {
  console.error(`Could not load esbuild from ${APP_ROOT}/node_modules.`);
  console.error('Run "npm install" in skillofied-app first.');
  process.exit(1);
}

/** Transpile + evaluate a TS data module so we read the real exported values. */
function loadModule(entry) {
  const result = esbuild.buildSync({
    entryPoints: [entry],
    bundle: true,
    write: false,
    format: 'cjs',
    platform: 'node',
    external: ['react', 'react-dom'],
    loader: { '.ts': 'ts', '.tsx': 'tsx' },
  });
  const mod = { exports: {} };
  new Function('module', 'exports', 'require', result.outputFiles[0].text)(mod, mod.exports, require);
  return mod.exports;
}

/**
 * Extract an array literal assigned to `name` from source text.
 * Starts scanning after the '=' so the "[]" in a type annotation such as
 * `QuizQuestion[]` is not mistaken for the literal.
 */
function extractArrayLiteral(src, name) {
  const start = src.indexOf(`const ${name}`);
  if (start < 0) return null;

  const assign = src.indexOf('=', start);
  const open = src.indexOf('[', assign);
  if (open < 0) return null;

  let depth = 0;
  let quote = null;
  for (let i = open; i < src.length; i++) {
    const c = src[i];
    if (quote) {
      if (c === quote && src[i - 1] !== '\\') quote = null;
      continue;
    }
    if (c === "'" || c === '"' || c === '`') { quote = c; continue; }
    if (c === '[') depth++;
    else if (c === ']') {
      depth--;
      if (depth === 0) return src.slice(open, i + 1);
    }
  }
  return null;
}

const keys = {};

// ---------- Java ----------
const java = loadModule(path.join(COURSES, 'modules/JavaCourse/JavaCourseData.ts'));
for (const [modId, mod] of Object.entries(java.JAVA_COURSE_DATA)) {
  if (mod.quiz?.length) {
    keys[`java-${modId}`] = mod.quiz.map((q) => ({ id: q.id, correctAnswer: q.correctAnswer }));
  }
  if (mod.assignment?.prompts?.length) {
    const mcqPrompts = mod.assignment.prompts.filter(p => typeof p === 'object' && p.kind === 'mcq');
    if (mcqPrompts.length > 0) {
      keys[`java-${modId}-assignment`] = mcqPrompts.map((q, idx) => ({ id: idx + 1, correctAnswer: q.correctAnswer }));
    }
  }
}
// ---------- Testing ----------
const testing = loadModule(path.join(COURSES, 'modules/TestingCourse/TestingCourseData.ts'));
for (const [modId, mod] of Object.entries(testing.TESTING_COURSE_DATA)) {
  if (mod.quiz?.length) {
    keys[`testing-${modId}`] = mod.quiz.map((q) => ({ id: q.id, correctAnswer: q.correctAnswer }));
  }
  if (mod.assignment?.prompts?.length) {
    const mcqPrompts = mod.assignment.prompts.filter(p => typeof p === 'object' && p.kind === 'mcq');
    if (mcqPrompts.length > 0) {
      keys[`testing-${modId}-assignment`] = mcqPrompts.map((q, idx) => ({ id: idx + 1, correctAnswer: q.correctAnswer }));
    }
  }
}

// ---------- Golang / Fullstack ----------
// Same shape as Java and Testing: a Record<moduleId, ModuleData> export whose
// module ids are namespaced with the course prefix by the renderer.
const RECORD_COURSES = [
  ['golang', 'modules/GolangCourse/GolangCourseData.ts', 'GOLANG_COURSE_DATA'],
  ['fullstack', 'modules/FullStackCourse/FullstackCourseData.ts', 'FULLSTACK_COURSE_DATA'],
  ['genai', 'modules/GenAICourse/GenAICourseData.ts', 'GENAI_COURSE_DATA'],
  ['os', 'modules/OsCourse/OsCourseData.ts', 'OS_COURSE_DATA'],
  ['networking', 'modules/NetworkingCourse/NetworkingCourseData.ts', 'NETWORKING_COURSE_DATA'],
];

for (const [prefix, file, exportName] of RECORD_COURSES) {
  const mod = loadModule(path.join(COURSES, file));
  const data = mod[exportName];
  if (!data) {
    console.error(`  ! ${exportName} not found in ${file}`);
    process.exitCode = 1;
    continue;
  }
  for (const [modId, m] of Object.entries(data)) {
    if (m.quiz?.length) {
      keys[`${prefix}-${modId}`] = m.quiz.map((q) => ({ id: q.id, correctAnswer: q.correctAnswer }));
    }
    if (m.assignment?.prompts?.length) {
      const mcqPrompts = m.assignment.prompts.filter(p => typeof p === 'object' && p.kind === 'mcq');
      if (mcqPrompts.length > 0) {
        keys[`${prefix}-${modId}-assignment`] = mcqPrompts.map((q, idx) => ({ id: idx + 1, correctAnswer: q.correctAnswer }));
      }
    }
  }
}

// ---------- SQL ----------
const sql = loadModule(path.join(COURSES, 'modules/SqlCourse/SqlCourseData.ts'));
for (const [key, questions] of Object.entries(sql.sqlQuizzes)) {
  if (!questions?.length) continue;
  keys[`sql-${key.replace(/-quiz$/, '')}`] = questions.map((q) => ({ id: q.id, correctAnswer: q.correctAnswer }));
}

// ---------- Frontend (quizzes are inline in each module component) ----------
const feDir = path.join(COURSES, 'modules/FrontendCourse');
for (const file of fs.readdirSync(feDir)) {
  const m = /^Module(\d+)\.tsx$/.exec(file);
  if (!m) continue;

  const literal = extractArrayLiteral(fs.readFileSync(path.join(feDir, file), 'utf8'), 'quizQuestions');
  if (!literal) continue;

  let questions;
  try {
    questions = new Function(`return ${literal};`)();
  } catch (err) {
    console.error(`  ! failed to parse quizQuestions in ${file}: ${err.message}`);
    process.exitCode = 1;
    continue;
  }
  if (!questions.length) continue;
  keys[`frontend-m${m[1]}`] = questions.map((q) => ({ id: q.id, correctAnswer: q.correctAnswer }));
}

// ---------- Marketing courses (SEO, Digital Marketing) ----------
// These export a single content object holding lessons, quizzes and assignments.
const MARKETING = [
  ['seo', 'modules/MarketingCourses/SeoCourseData.ts', 'seoContent'],
  ['dm', 'modules/MarketingCourses/DigitalMarketingCourseData.ts', 'digitalMarketingContent'],
];

for (const [, file, exportName] of MARKETING) {
  const mod = loadModule(path.join(COURSES, file));
  const content = mod[exportName];
  if (!content) {
    console.error(`  ! ${exportName} not found in ${file}`);
    process.exitCode = 1;
    continue;
  }
  for (const [quizId, questions] of Object.entries(content.quizzes)) {
    if (!questions?.length) continue;
    // Quiz ids already carry the course prefix ("seo-m1-quiz"), and the
    // renderer submits the syllabus module id directly, so no extra prefix.
    const moduleId = quizId.replace(/-quiz$/, '');
    keys[moduleId] = questions.map((q) => ({ id: q.id, correctAnswer: q.correctAnswer }));
  }
}

// The frontend final assessment is graded the same way, under its own id.
const assessmentLiteral = extractArrayLiteral(
  fs.readFileSync(path.join(feDir, 'FinalAssessment.tsx'), 'utf8'),
  'theoryQuestions'
);
if (assessmentLiteral) {
  const questions = new Function(`return ${assessmentLiteral};`)();
  if (questions.length) {
    keys['frontend-assessment'] = questions.map((q) => ({ id: q.id, correctAnswer: q.correctAnswer }));
  }
}

// ---------- validate ----------
const sorted = Object.entries(keys).sort(([a], [b]) => {
  // "java-m12" -> ["java", 12]; ids without a module number (e.g.
  // "frontend-assessment") sort last within their course.
  const parse = (s) => {
    const m = /^(.*)-m(\d+)$/.exec(s);
    return m ? [m[1], Number(m[2])] : [s.split('-')[0], Number.MAX_SAFE_INTEGER];
  };
  const [ca, na] = parse(a);
  const [cb, nb] = parse(b);
  return ca.localeCompare(cb) || na - nb || a.localeCompare(b);
});

let total = 0;
let invalid = 0;
for (const [moduleId, questions] of sorted) {
  const ids = questions.map((q) => q.id);
  if (ids.length !== new Set(ids).size) {
    console.error(`  ! ${moduleId} has duplicate question ids`);
    invalid++;
  }
  for (const q of questions) {
    if (!q.correctAnswer) {
      console.error(`  ! ${moduleId} question ${q.id} has no correctAnswer`);
      invalid++;
    }
  }
  total += questions.length;
}

if (invalid > 0) {
  console.error(`\nAborted: ${invalid} problem(s) found. Seed not written.`);
  process.exit(1);
}

// ---------- emit ----------
const goEsc = (s) => s.replace(/\\/g, '\\\\').replace(/"/g, '\\"');

const body = sorted
  .map(([moduleId, questions]) =>
    [`\t// ${moduleId} (${questions.length} questions)`]
      .concat(questions.map((q) => `\t{"${moduleId}", ${q.id}, "${goEsc(q.correctAnswer)}"},`))
      .join('\n')
  )
  .join('\n\n');

fs.writeFileSync(
  DEST,
  `package repository

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
\tmoduleID   string
\tquestionID int
\tcorrectAns string
}

// quizAnswerKeys holds ${total} answer keys across ${sorted.length} modules.
var quizAnswerKeys = []quizKey{
${body}
}
`
);

console.log(`Wrote ${total} answer keys across ${sorted.length} modules`);
console.log(`  -> ${path.relative(process.cwd(), DEST)}`);

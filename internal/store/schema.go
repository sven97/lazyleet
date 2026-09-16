package store

// migrations are applied in order. Never edit or reorder an existing entry once
// released; append a new one instead. schema_migrations records the highest
// applied index.
var migrations = []string{
	// 1: initial schema
	`
CREATE TABLE problems (
    frontend_id INTEGER PRIMARY KEY,
    question_id INTEGER NOT NULL,
    slug        TEXT    NOT NULL UNIQUE,
    title       TEXT    NOT NULL,
    difficulty  TEXT    NOT NULL,               -- Easy | Medium | Hard
    ac_rate     REAL    NOT NULL DEFAULT 0,
    paid_only   INTEGER NOT NULL DEFAULT 0,     -- 0 | 1
    status      TEXT    NOT NULL DEFAULT '',    -- '' | ac | notac
    topic_tags  TEXT    NOT NULL DEFAULT '[]',  -- JSON array of slugs
    updated_at  INTEGER NOT NULL                -- unix seconds
);
CREATE INDEX idx_problems_slug ON problems(slug);

CREATE TABLE problem_detail (
    slug              TEXT    PRIMARY KEY,
    question_id       INTEGER NOT NULL,
    content_html      TEXT    NOT NULL DEFAULT '',
    meta_data         TEXT    NOT NULL DEFAULT '{}',   -- JSON
    example_testcases TEXT    NOT NULL DEFAULT '',
    sample_testcase   TEXT    NOT NULL DEFAULT '',
    code_snippets     TEXT    NOT NULL DEFAULT '{}',   -- JSON: lang -> code
    hints             TEXT    NOT NULL DEFAULT '[]',   -- JSON array
    similar           TEXT    NOT NULL DEFAULT '[]',   -- JSON array
    fetched_at        INTEGER NOT NULL
);

CREATE TABLE study_plans (
    slug       TEXT    PRIMARY KEY,
    name       TEXT    NOT NULL,
    source     TEXT    NOT NULL,               -- leetcode | bundled
    problems   TEXT    NOT NULL DEFAULT '[]',  -- JSON array of slugs, ordered
    fetched_at INTEGER NOT NULL
);

CREATE TABLE submissions (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    slug          TEXT    NOT NULL,
    lang          TEXT    NOT NULL,
    submission_id TEXT    NOT NULL DEFAULT '', -- LeetCode id for remote run/submit
    kind          TEXT    NOT NULL,            -- local | run | submit
    verdict       TEXT    NOT NULL DEFAULT '',
    runtime_ms    INTEGER,
    memory_kb     INTEGER,
    passed        INTEGER,
    total         INTEGER,
    detail        TEXT    NOT NULL DEFAULT '{}', -- JSON
    code          TEXT    NOT NULL DEFAULT '',
    created_at    INTEGER NOT NULL
);
CREATE INDEX idx_submissions_slug ON submissions(slug, created_at);

CREATE TABLE workspace_state (
    slug        TEXT    PRIMARY KEY,
    lang        TEXT    NOT NULL,
    dir         TEXT    NOT NULL,
    last_opened INTEGER NOT NULL,
    created_at  INTEGER NOT NULL
);
`,
	// 2: cache the worked example cases (inputs + scraped expected outputs) so
	// they survive a cache round-trip; exampleTestcases alone is inputs-only.
	`ALTER TABLE problem_detail ADD COLUMN example_cases TEXT NOT NULL DEFAULT '[]';`,
	// 3: a small key/value scratch table for cache bookkeeping that has no
	// natural home on another row — e.g. when the signed-in user's solve status
	// was last refreshed (distinct from the full-catalog sync recorded by
	// problems.updated_at).
	`CREATE TABLE meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);`,
	// 4: preserve official plan groups and problem metadata for offline debug output.
	`ALTER TABLE study_plans ADD COLUMN questions TEXT NOT NULL DEFAULT '[]';`,
	// 5: distinguish "hints fetched but genuinely empty" from "cached before
	// hints support existed" — the hints column alone can't tell those apart,
	// since both end up as '[]'.
	`ALTER TABLE problem_detail ADD COLUMN hints_fetched INTEGER NOT NULL DEFAULT 0;`,
}

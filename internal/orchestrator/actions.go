package orchestrator

import "fmt"

// MaxReviewRounds is the safety limit for consecutive code-review iterations
// on a single story. Prevents infinite loops when reviews keep finding issues.
const MaxReviewRounds = 3

// MaxInvocationsPerStory is the absolute ceiling of Claude calls for a single story
// across all phases (create + dev + review rounds + judge calls don't count).
const MaxInvocationsPerStory = 8

// MaxConsecutiveBlocked stops the autopilot after N stories are blocked in a row.
const MaxConsecutiveBlocked = 2

type Action struct {
	Prompt       string
	Command      string
	WorkflowKey  string // maps to BMAD workflow in workflowRegistry
	AllowedTools string // override default allowed tools (empty = use executor default)
}

func PlanPrimaryActions(status, storyNumber string) ([]Action, error) {
	switch normalizeStatus(status) {
	case "backlog":
		return []Action{
			createStoryAction(storyNumber),
			devStoryAction(storyNumber),
		}, nil
	case "ready-for-dev", "in-progress":
		return []Action{
			devStoryAction(storyNumber),
		}, nil
	case "review":
		return nil, nil
	case "done":
		return []Action{
			validateStoryAction(storyNumber),
		}, nil
	case "validated":
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported story status %q", status)
	}
}

func ReviewAction(storyNumber string) Action {
	return newAction(
		"code-review",
		fmt.Sprintf(`Execute the code-review workflow for story %s in #yolo mode.
Follow the workflow engine (workflow.xml) to process the workflow configuration and instructions.

PERSONA: You are a grey-hat security researcher AND a senior dev reviewer.
You think like an attacker but fix like a defender. You hunt BOTH security vulns AND classic bugs.

PHASE 1 — RECON (map the attack surface):
- Read the story file and ALL changed files.
- Identify every entry point: routes, form handlers, API endpoints, file uploads, WebSocket channels.
- Identify trust boundaries: user input → controller → service → database → response.
- List all data flows where untrusted data crosses a trust boundary.

PHASE 2 — OFFENSIVE (try to break it, OWASP Top 10 methodology):
For each entry point, systematically attempt:
- **Injection** (SQL/DQL, XSS stored+reflected, SSTI, command injection, LDAP, header injection)
- **Broken Auth** (session fixation, credential stuffing vectors, missing brute-force protection)
- **Broken Access Control** (IDOR — change entity IDs in URLs, privilege escalation between roles, missing Voter checks, forced browsing to admin routes)
- **Security Misconfiguration** (verbose errors leaking stack traces, debug mode, default credentials, missing security headers, CORS misconfiguration)
- **Cryptographic Failures** (plaintext PII, weak hashing, hardcoded secrets, missing encryption at rest)
- **Insecure Design** (race conditions via concurrent requests, mass assignment via unprotected form fields, business logic bypass — negative amounts, duplicate submissions, replay attacks)
- **SSRF** (user-controlled URLs fetched server-side without validation)
- **Data Exposure** (API over-fetching — returning fields the role should not see, PII in logs, error responses with internal paths)

For EACH finding, write a PROOF OF CONCEPT:
- The exact malicious input, HTTP request, or scenario.
- The expected vulnerable behavior vs the expected secure behavior.
- NOT "this could be vulnerable" — PROVE IT or dismiss it.

PHASE 3 — DEFENSIVE (fix and harden):
- Fix every CRITICAL and HIGH finding. Write a security test that proves the fix.
- For MEDIUM findings: fix if straightforward, otherwise document with the exact exploit scenario.
- IGNORE cosmetic issues (naming, formatting, style) unless they cause an actual bug.

PHASE 4 — VALIDATION (verify story claims):
- Verify tasks marked [x] are actually implemented (file:line evidence).
- Verify each Acceptance Criterion has a working implementation AND a test.
- Missing AC implementation = HIGH finding. False [x] = CRITICAL finding.

TEST EXECUTION RULES:
- ONLY run tests for files changed in this story — NEVER the full test suite.
- Use targeted test commands with EXPLICIT file paths or --filter.
- Write NEW security test cases for vulnerabilities you find and fix.
- Running "php bin/phpunit" without file arguments is FORBIDDEN (2900+ tests, 20+ min, $5-12 wasted).
- Pre-existing test failures are NOT your problem — ignore them.

COMMIT RULES:
- ALL commit messages MUST start with "review(%s): " followed by a description.
- Describe the VULNERABILITY you fixed, not that you reviewed.
  Example: "review(%s): sanitize review content against stored XSS via Twig escape"
  Example: "review(%s): add HostVoter check to prevent IDOR on message endpoint"
  Example: "review(%s): add rate limiting on password reset to prevent brute-force"
- Do NOT use generic messages like "code-review completed".
- If no real issues found, do NOT create an empty commit.

STATUS UPDATE:
- After review, update sprint-status.yaml for this story:
  - If no CRITICAL/HIGH issues remain: set status to "done"
  - If CRITICAL/HIGH issues could not be fixed: set status to "blocked"
- Commit the status update separately: "review(%s): update status to [new-status]"
- Then push all commits.`, storyNumber, storyNumber, storyNumber, storyNumber, storyNumber, storyNumber),
	)
}

// ShouldContinueReview returns true if the review loop should keep running.
// Stops when status reaches "done" or "blocked".
func ShouldContinueReview(status string) bool {
	s := normalizeStatus(status)
	return s != "done" && s != "blocked" && s != "validated"
}

func createStoryAction(storyNumber string) Action {
	return newAction(
		"create-story",
		fmt.Sprintf(`Execute the create-story workflow for story %s in #yolo mode.
Follow the workflow engine (workflow.xml) to process the workflow configuration and instructions.
Auto-complete all steps autonomously as an expert Scrum Master.

TEST-FIRST MANDATE:
- Design the story with TESTABILITY as the primary constraint.
- The "Test Strategy" section MUST be written BEFORE the "Tasks / Subtasks" section.
- Each acceptance criterion must have a corresponding test scenario (unit or functional).
- Tasks must be ordered: test infrastructure first, then implementation, then integration tests.
- If a task cannot be tested in isolation, split it until it can.
- Specify exactly WHAT to test and HOW (test class, method pattern, assertions).

COMMIT RULES:
- ALL commit messages MUST start with "create(%s): " followed by a description.
  Example: "create(%s): define story spec with 6 acceptance criteria and test-first task breakdown"
- Do NOT use generic messages like "create-story completed".
- Describe what the story spec contains.

STATUS UPDATE:
- Update sprint-status.yaml: set this story's status to "ready-for-dev"
- Commit the status update separately: "create(%s): update status to ready-for-dev"`, storyNumber, storyNumber, storyNumber, storyNumber),
	)
}

func devStoryAction(storyNumber string) Action {
	return newAction(
		"dev-story",
		fmt.Sprintf(`Execute the dev-story workflow for story %s in #yolo mode.
Read the story file, implement ALL tasks and subtasks IN ORDER.
Write tests for each task. Mark tasks [x] only when tests pass.
Follow the workflow engine (workflow.xml) to process the workflow configuration and instructions.

TEST EXECUTION RULES:
- ONLY run YOUR tests — the tests you wrote or modified for this story.
- Use targeted test commands with EXPLICIT file paths or --filter, NEVER the full test suite.
- Running "php bin/phpunit" without file arguments is ABSOLUTELY FORBIDDEN (2900+ tests, 20+ min, $5-12 wasted).
- Do NOT use "composer test" either (Composer 300s process timeout + runs full suite).
- Before running tests, perform IMPACT ANALYSIS: list modified files → identify their test files + dependent tests → run ONLY those.
- Pre-existing test failures are NOT your problem — ignore them.

COMMIT RULES:
- ALL commit messages MUST start with "dev(%s): " followed by a description.
- Commit after each logical unit of work (not one giant commit at the end).
  Example: "dev(%s): implement BaseLayout with header, main and footer landmarks"
  Example: "dev(%s): add responsive navigation with glassmorphism effect"
- Do NOT use generic messages like "dev-story completed".
- Describe WHAT you implemented.

STATUS UPDATE:
- When all tasks are done, update sprint-status.yaml: set this story's status to "review"
- Commit the status update separately: "dev(%s): update status to review"`, storyNumber, storyNumber, storyNumber, storyNumber, storyNumber),
	)
}

// chromeMCPTools lists the MCP Chrome DevTools tools for browser-based validation.
const chromeMCPTools = "mcp__chrome-devtools__navigate_page,mcp__chrome-devtools__take_screenshot,mcp__chrome-devtools__click,mcp__chrome-devtools__fill,mcp__chrome-devtools__fill_form,mcp__chrome-devtools__evaluate_script,mcp__chrome-devtools__press_key,mcp__chrome-devtools__type_text,mcp__chrome-devtools__hover,mcp__chrome-devtools__wait_for,mcp__chrome-devtools__select_page,mcp__chrome-devtools__list_pages,mcp__chrome-devtools__new_page,mcp__chrome-devtools__close_page,mcp__chrome-devtools__get_console_message,mcp__chrome-devtools__list_console_messages,mcp__chrome-devtools__list_network_requests,mcp__chrome-devtools__get_network_request,mcp__chrome-devtools__handle_dialog,mcp__chrome-devtools__upload_file,mcp__chrome-devtools__drag,mcp__chrome-devtools__emulate,mcp__chrome-devtools__resize_page,mcp__chrome-devtools__take_snapshot,mcp__chrome-devtools__lighthouse_audit"

func validateStoryAction(storyNumber string) Action {
	return Action{
		Prompt: fmt.Sprintf(`Execute the validate-story workflow for story %s in #yolo mode.
Follow the workflow engine (workflow.xml) to process the workflow configuration and instructions.

CRITICAL CONTEXT — FULLY AUTONOMOUS VALIDATION (NO HUMAN):
You are running in AUTOPILOT mode. There is NO human to test in the browser.
YOU are both the security auditor AND the browser tester.
Use the MCP Chrome DevTools tools for ALL browser-based testing.

BROWSER TESTING VIA MCP CHROME DEVTOOLS:
- Navigate to pages: use mcp__chrome-devtools__navigate_page
- Take screenshots to verify visual state: use mcp__chrome-devtools__take_screenshot
- Click buttons/links: use mcp__chrome-devtools__click
- Fill forms: use mcp__chrome-devtools__fill or mcp__chrome-devtools__fill_form
- Check console errors: use mcp__chrome-devtools__list_console_messages
- Verify network requests: use mcp__chrome-devtools__list_network_requests
- Test responsive: use mcp__chrome-devtools__resize_page (mobile: 375x667, tablet: 768x1024, desktop: 1920x1080)
- Test dark mode: use mcp__chrome-devtools__evaluate_script to toggle data-theme attribute
- Test i18n: navigate with /en/ and /fr/ locale prefixes
- Wait for elements: use mcp__chrome-devtools__wait_for before interacting
- Keyboard interactions: use mcp__chrome-devtools__press_key

PHASE EXECUTION:
1. PHASE 1 (Autonomous Deep Scan): Execute ALL automated checks — code analysis, curl attacks, PHPStan, PHPUnit, DB queries, pentest. Standard workflow.
2. PHASE 2 (Report): Compile findings report. Do NOT wait for human — proceed immediately to Phase 3.
3. PHASE 3 (Fix + Browser Test):
   - First: fix ALL findings from Phase 1 (CRITIQUE, MAJEUR, MINEUR — zero unresolved).
   - Then: execute ALL browser tests yourself via MCP Chrome DevTools.
   - For EACH Acceptance Criterion: navigate to the relevant page, interact with UI elements, take screenshots, verify expected behavior.
   - Test responsive, dark mode, translations, empty states, error states.
   - Record PASS/FAIL for each AC based on your browser testing.
4. PHASE 4 (Verdict): Compile verdict based on your own findings + browser test results. Update story status.

TEST EXECUTION RULES:
- ONLY run tests for files changed in this story — NEVER the full test suite.
- Use targeted test commands with EXPLICIT file paths or --filter.
- Running "php bin/phpunit" without file arguments is FORBIDDEN.
- NEVER use "composer test".
- Pre-existing test failures are NOT your problem.

COMMIT RULES:
- ALL commit messages MUST start with "validate(%s): " followed by a description.
  Example: "validate(%s): fix stored XSS in review content via Twig escape"
  Example: "validate(%s): add missing CSRF check on profile update form"
- Do NOT use generic messages like "validation completed".
- If no real issues found, do NOT create an empty commit.

STATUS UPDATE:
- After validation, update sprint-status.yaml for this story:
  - If ALL findings fixed + ALL ACs pass: set status to "validated"
  - If unfixable issues remain: set status to "blocked"
- Commit the status update separately: "validate(%s): update status to [new-status]"
- Then push all commits.`, storyNumber, storyNumber, storyNumber, storyNumber, storyNumber),
		Command:      fmt.Sprintf("claude -p [validate-story] --dangerously-skip-permissions --append-system-prompt [BMAD context]"),
		WorkflowKey:  "validate-story",
		AllowedTools: "Bash,Read,Write,Edit,Glob,Grep,Agent,Skill," + chromeMCPTools,
	}
}

func newAction(workflowKey, prompt string) Action {
	return Action{
		Prompt:      prompt,
		Command:     fmt.Sprintf("claude -p [%s] --dangerously-skip-permissions --append-system-prompt [BMAD context]", workflowKey),
		WorkflowKey: workflowKey,
	}
}

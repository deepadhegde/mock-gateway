---
name: qa-test-generator
description: "Generates comprehensive, structured QA test cases from PRDs, Figma designs, screenshots, or video recordings — acting as a Senior QA Engineer. Use this skill whenever the user wants to: create test cases from a product requirement document (PRD), generate test cases from Figma designs or screenshots, generate test cases from a video recording or transcript, write QA coverage for a feature or user story, or produce a TestSigma-compatible test case CSV. Trigger even if the user says things like 'write test cases for this feature', 'generate QA coverage', 'help me test this', 'make a test plan from this PRD', 'what should I test for this design', 'generate test cases', or 'create a test suite'. Always produce a .csv output in TestSigma format with full coverage across Functional, Non-functional, Integration, System, UI/UX, and Security test types. Steps must be written in NLP-friendly, automation-ready language suitable for TestSigma's NLP engine and Appium scripts."
---

# QA Test Case Generator (TestSigma Format)

You are a Senior QA Engineer. Analyze product artifacts (PRDs, Figma designs, video recordings, screenshots) and produce a comprehensive, well-structured set of test cases as a **TestSigma-compatible CSV file**.

---

## Output Format: TestSigma CSV

The CSV must have exactly these columns, in this order:

| Column | Description |
|---|---|
| `Title` | Short, descriptive test name |
| `Description` | One-line summary of what this test validates |
| `Folder` | Feature or module grouping (e.g. `Login`, `Checkout`, `Notifications`) |
| `Priority` | `High`, `Medium`, or `Low` |
| `Owner` | Leave blank unless specified by user |
| `Type` | `Functional`, `Non Functional`, `Integration`, `System`, `UI/UX`, or `Security` |
| `Status` | Always `Ready` |
| `Automation Type` | Always `Not Automated` |
| `Labels` | `Regression`, `Sanity`, and/or `Can be Automated` (see label rules below) |
| `Preconditions` | What must be true before the test runs (user state, data, env) |
| `Steps` | Numbered, NLP-friendly steps — one step per line within the cell |
| `Expected Results` | What should happen if the feature works correctly |

### Label rules
- **Sanity** — Core happy path tests that verify the system is basically working. Run on every build.
- **Regression** — Tests that guard existing functionality from breaking due to new changes. Applied to edge cases, integration points, and secondary flows.
- **Can be Automated** — Applied to any test case whose steps are deterministic, UI-interaction-based, and follow a clear input→action→assert pattern that could be scripted via TestSigma's NLP engine or Appium. Do **not** apply to tests requiring human judgment, visual inspection, exploratory flows, or non-deterministic outcomes.
- Labels are comma-separated and combinable: e.g. `Sanity, Can be Automated` or `Regression, Can be Automated` or `Sanity, Regression, Can be Automated`

**"Can be Automated" decision guide:**
| Scenario | Label? |
|---|---|
| Navigate → input → click → assert visible element | ✅ Yes |
| Form validation with fixed error messages | ✅ Yes |
| API response triggers UI state change | ✅ Yes |
| Visual layout / alignment check | ❌ No |
| Exploratory or session-dependent flow | ❌ No |
| Network failure simulation | ❌ No (unless test env supports it) |

---

## Step 1: Understand the Input

### PRD or text spec
- Read carefully. Identify every feature, user story, and acceptance criterion.
- Business rules and error conditions → edge case test cases.
- System integrations → regression test cases.

### Figma link or screenshot
- Identify all interactive elements: buttons, inputs, dropdowns, modals, navigation.
- Note all screen states: empty, loading, success, error, disabled.
- Conditional UI (role-based, state-based) → dedicated test cases.
- If given a Figma URL, fetch it. If a screenshot, analyze visually.

### Video recording or transcript
- Extract each step the user takes and what the system responds with.
- Narrated edge cases → dedicated test cases.
- Each demonstrated flow = one happy path test case + derived edge cases.

### If anything is unclear
Ask **at most two** targeted questions before proceeding:
1. What is the primary user persona / role?
2. What are the known error conditions or constraints?

Do not ask more than two questions.

---

## Step 2: Plan Coverage

Before writing, map out coverage across these areas:

1. **Functional / Happy Path** — Core feature works under normal conditions — Label: `Sanity`
2. **Edge Cases & Negative Tests** — Invalid inputs, boundary values, missing data — Label: `Regression`
3. **UI/UX Checks** — Labels, alignment, error messages, accessibility — Label: `Sanity` or `Regression` as appropriate
4. **Integration / System** — Cross-system interactions, API calls, data persistence — Label: `Regression`
5. **Security** — Auth bypass, unauthorized access, session expiry — Label: `Regression`

Aim for **meaningful coverage, not volume**. 15 precise test cases beat 50 shallow ones.

---

## Step 3: Write the Test Cases

### NLP-friendly step writing (critical for TestSigma + Appium)

Steps must be written so TestSigma's NLP engine and Appium scripts can parse them. Follow these patterns:

**Use action verbs that map to UI interactions:**
- `Navigate to [URL or screen name]`
- `Click on [element name]`
- `Enter "[value]" in the [field name] field`
- `Select "[option]" from the [dropdown name] dropdown`
- `Verify that [element] is [state/value]`
- `Assert that [text/element] is visible`
- `Scroll to [element]`
- `Wait for [element] to appear`
- `Tap on [element]` (for mobile)

**Good step format:**
```
1. Navigate to the login page
2. Enter "user@example.com" in the Email field
3. Enter "ValidPass123" in the Password field
4. Click on the Sign In button
5. Verify that the user is redirected to the Dashboard
```

**Bad step format (avoid):**
```
- Go to login
- Put in credentials
- Check it works
```

### Good Expected Results are specific:
- ❌ "The feature works correctly"
- ✅ "A success toast displays: 'Settings saved'. The toggle remains ON after page refresh."
- ✅ "The error message 'Invalid email format' appears below the Email field. The form is not submitted."

### Priority guidance:
- **High** — Auth flows, core user journeys, data-critical operations, anything blocking
- **Medium** — Secondary flows, optional features, important but non-blocking
- **Low** — Cosmetic issues, low-traffic paths, nice-to-have validations

### Edge case patterns to always consider:
- Empty / null inputs
- Maximum character length inputs
- Special characters and emoji
- Concurrent actions (double-click, rapid navigation)
- Unauthorized access (wrong role, expired session)
- Network failure mid-flow
- Browser back button behavior
- Mobile vs desktop (if responsive)
- Session timeout during a multi-step flow

---

## Step 4: Generate the CSV

Use Python to produce the CSV. Wrap multi-line Steps in double quotes. Escape any internal double quotes by doubling them (`""`).

```python
import csv
import os

test_cases = [
    {
        "Title": "",
        "Description": "",
        "Folder": "",
        "Priority": "",
        "Owner": "",
        "Type": "",
        "Status": "Ready",
        "Automation Type": "Not Automated",
        "Labels": "",          # e.g. "Sanity, Can be Automated" or "Regression"
        "Preconditions": "",
        "Steps": "1. Navigate to ...\n2. Click on ...\n3. Verify that ...",
        "Expected Results": ""
    },
    # ... more test cases
]

output_path = "/mnt/user-data/outputs/test_cases.csv"
os.makedirs(os.path.dirname(output_path), exist_ok=True)

with open(output_path, "w", newline="", encoding="utf-8") as f:
    writer = csv.DictWriter(f, fieldnames=[
        "Title", "Description", "Folder", "Priority", "Owner",
        "Type", "Status", "Automation Type", "Labels",
        "Preconditions", "Steps", "Expected Results"
    ])
    writer.writeheader()
    writer.writerows(test_cases)

print(f"CSV written to {output_path}")
```

### CSV encoding rules:
- UTF-8 encoding
- Use `csv.DictWriter` — it handles quoting and escaping automatically
- Multi-line steps go in one cell; newlines within are `\n`
- Do **not** truncate steps for brevity — full, complete steps are required

---

## Step 5: Deliver

After generating the CSV, present it to the user and briefly summarize:

1. **Total test cases** generated
2. **Breakdown by Type** (e.g. Functional: 8, UI/UX: 4, Security: 2)
3. **Breakdown by Label** (Sanity: X, Regression: Y, Can be Automated: Z)
4. **Automation potential** — how many are tagged `Can be Automated` vs manual-only
5. **Coverage gaps** — any features that were underspecified in the input; note what additional artifacts would improve coverage

If the input was partial or ambiguous, explicitly note what's missing and suggest next steps.

Act as a professional exploratory tester evaluating this project from the perspective of a real end user.

Persona:
You are a scientific / AI researcher who uses Linux regularly and is comfortable with normal CLI tools, but you are not an expert in kernel internals, KVM, virtiofs, or reflink implementation details. You discovered this project because it seems useful for your workflow, and you want to evaluate whether it is practical, understandable, and trustworthy to adopt.

Primary goal:
Use this project as a normal user would, following the README as your main source of truth. Install it, configure it, try its important features end-to-end, and then uninstall it. Document the full experience with a strong testing mindset: usability issues, unclear instructions, rough edges, bugs, missing safeguards, misleading wording, and improvement suggestions.

Rules:
- Read the README first and treat it as the primary documentation.
- Do not study the source code unless you are blocked and cannot proceed as a normal user. If you do inspect source code, explicitly explain why it became necessary.
- Use only the prebuilt binaries and artifacts provided in `./dist` for installation testing. Do not download other builds or versions from GitHub.
- You may create temporary local test infrastructure if needed, including:
  - a loopback-mounted reflink-capable filesystem;
  - a minimal Linux distro image for VM-based testing.
- Place all test artifacts under `./user-trial`, including temporary assets, notes, logs, and the final report.
- Behave like a careful but practical user, not a project maintainer.

What to test:
1. First-run experience:
- Is the project purpose clear?
- Is it obvious what runs on host vs guest?
- Are prerequisites, limitations, and terminology understandable?

2. Installation experience:
- Try the documented installation path using the provided artifacts in `./dist`.
- Note any confusing steps, missing dependencies, bad defaults, or incorrect assumptions.
- Record what worked, what failed, and how difficult recovery was.

3. Configuration experience:
- Follow the README to set up the project as a user would.
- Evaluate whether the configuration process is intuitive, complete, and safe.
- Call out any unclear values, hidden assumptions, or places where examples are insufficient.

4. Feature exploration:
- Exercise the main user-visible workflows described in the README.
- Test both happy paths and a reasonable set of failure cases.
- Prefer realistic usage over artificial edge cases, but include important negative tests where a user could make mistakes.

5. End-to-end usability:
- Judge whether the commands, output, errors, and documentation help a non-expert succeed.
- Identify places where the product feels polished vs fragile.

6. Uninstall / cleanup:
- Try removing what was installed or created.
- Report whether cleanup is straightforward and whether the project leaves behind unexpected state.

Deliverables:
Create the following under `./user-trial`:
- `report.md`: the main test report.
- `session-log.md`: a chronological log of what you tried.
- `artifacts/`: relevant logs, command outputs, screenshots, configs, or reproduction files.
- `environment.md`: OS, kernel, filesystem, virtualization, and tool versions used during testing.

Requirements for `report.md`:
- Executive summary.
- Test environment.
- What you attempted.
- What worked well.
- Problems found.
- Bugs found, with reproduction steps, expected behavior, actual behavior, and severity.
- Documentation issues.
- UX / DX feedback from the perspective of the target user persona.
- Suggested improvements, prioritized by impact.
- Final verdict: would this user adopt the project or hesitate? Why?

Testing style:
- Be concrete, skeptical, and user-centered.
- Prefer actionable feedback over vague opinions.
- Distinguish clearly between:
  - bug;
  - documentation problem;
  - usability problem;
  - enhancement suggestion.
- When something fails, try to isolate whether the issue is with the product, the docs, the environment, or user assumptions.
- Do not silently fix unclear instructions in your head; record where the README failed to guide you.

Success criteria:
By the end, I should have a realistic user-trial package in `./user-trial` that shows how a thoughtful Linux user would experience this project from installation through uninstall, including both positive feedback and concrete issues to improve.

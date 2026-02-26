Please work on all the APPROVED items in the ROADMAP.md document, committing each as you go.

Follow red/green TDD as described in AGENTS.md: write a failing test first, then implement.
Ensure all tests pass and add regression tests as needed.

Follow these guidelines:

1. Read ROADMAP.md to identify all APPROVED work items
2. For each approved item:
   - Read the relevant source files before making changes
   - Write failing tests first (red), then implement (green), then refactor
   - Run `go build ./...` to verify the build compiles
   - Run `make test` to verify unit tests pass
   - Run `make godog-test` and note the new pass/fail scenario counts
   - Commit with a descriptive message including scenario count change, e.g.:
     `Fix OPF-012 nav property check: 41 → 38 failing scenarios`
3. After all items are implemented:
   - Run `make test-all` to verify both unit and BDD tests pass
   - Run `go vet ./...` to verify no static analysis issues
4. Update ROADMAP.md:
   - Move completed items from APPROVED to COMPLETED with implementation notes
   - Update the godog scenario counts and progress history in the STATUS section
   - Commit the ROADMAP.md update
5. Push all commits to the current branch

# Phase Transition Checklist

## Before Completing Current Phase

### 1. Documentation
- [ ] Update `agent-plan.md` with completed tasks
- [ ] Create learning document in `.agent/learnings/` for this phase
- [ ] Update `cleanup.md` with any technical debt or issues found
- [ ] Document any regrets in `regrets.md`
- [ ] Note any user commendations in `rewards.md` (only if explicitly given)

### 2. Testing
- [ ] Run all unit tests: `make test`
- [ ] Run BDD tests: `python run_bdd_tests.py`
- [ ] Document test results (before/after metrics)
- [ ] Note any failing tests that are blocked by missing features

### 3. Code Review
- [ ] Ensure all new code has appropriate documentation
- [ ] Verify error handling is consistent
- [ ] Check for any hardcoded values that should be configurable
- [ ] Confirm thread safety for concurrent operations

### 4. Clean Up
- [ ] Remove any debug print statements
- [ ] Delete temporary files or test artifacts
- [ ] Ensure all TODO comments are tracked in cleanup.md
- [ ] Verify no sensitive information in commits

## Documenting Phase Completion

### 1. Create Phase Completion Document
Use template: `.agent/learnings/PHASE-TEMPLATE.md`

Required sections:
- [ ] Summary of accomplishments
- [ ] Metrics (tests passing, coverage, etc.)
- [ ] Discoveries and learnings
- [ ] Technical debt introduced
- [ ] Blockers encountered
- [ ] **CRITICAL: Next phase plan with specific steps**

### 2. Update Agent Plan
In `.agent/agent-plan.md`:
- [ ] Mark completed items with ✅
- [ ] Update "Current Status" section
- [ ] Add clear "🎯 NEXT ACTION" section with:
  - Why this is next
  - Implementation steps
  - Success criteria
  - Commands to run

### 3. Define Next Phase Entry Point
- [ ] **First file to create/modify**
- [ ] **First test to run**
- [ ] **Success criteria for first step**
- [ ] **Estimated time for first deliverable**

## Starting Next Phase

### 1. Review Context
- [ ] Read previous phase completion document
- [ ] Review "NEXT ACTION" in agent-plan.md
- [ ] Check cleanup.md for relevant issues
- [ ] Note any blockers from previous phase

### 2. Set Up
- [ ] Create new branch if using version control
- [ ] Set up any new directories needed
- [ ] Create placeholder files for new components
- [ ] Write failing tests first (TDD approach)

### 3. Initial Implementation
- [ ] Start with simplest possible implementation
- [ ] Get one test passing
- [ ] Commit early and often
- [ ] Document as you go

## Phase Transition Communication

### When Reporting Phase Completion
Include:
1. **What was accomplished** (bullet points)
2. **Current test status** (X/Y passing)
3. **Next immediate action** (specific file and function)
4. **Estimated time to next milestone**

### Example:
```
Phase Complete: Documentation

✅ Accomplishments:
- Documented all 47 public functions
- Added 8 architecture diagrams
- Achieved 100% documentation coverage

📊 Test Status: 22/39 scenarios passing
- distributed-storage: 21/21 ✅
- cluster-management: 1/18 ⚠️ (17 need health monitoring)

🎯 Next Action: Create internal/coordinator/health_monitor.go
- Add HealthMonitor struct
- Implement Start() method with 5s check interval
- Test with: python run_bdd_tests.py --scenario "Node health monitoring"
- Estimated: 2 hours to first working health check

Ready to continue? The next step is clearly defined.
```

## Common Phase Types

### Feature Implementation Phase
1. Start with failing tests
2. Implement minimal working version
3. Add error handling
4. Optimize if needed
5. Document thoroughly

### Bug Fix Phase
1. Reproduce the issue with a test
2. Fix the bug
3. Verify all tests still pass
4. Document the fix in learnings
5. Add regression test

### Refactoring Phase
1. Ensure comprehensive test coverage first
2. Make incremental changes
3. Run tests after each change
4. Update documentation
5. Verify no functionality lost

### Testing Phase
1. Identify untested code paths
2. Write tests for edge cases
3. Add integration tests
4. Run coverage reports
5. Document testing patterns

### Documentation Phase
1. Start with package overviews
2. Document public APIs
3. Add usage examples
4. Create architecture diagrams
5. Write operational guides

## Red Flags to Watch For

### Signs Phase Is Too Large
- [ ] More than 10 files need modification
- [ ] Estimated time exceeds 8 hours
- [ ] Multiple unrelated changes required
- [ ] Can't define clear success criteria

### Signs Phase Is Complete
- [ ] All success criteria met
- [ ] Tests passing (or failures understood)
- [ ] Documentation updated
- [ ] Next steps clearly defined
- [ ] No critical blockers remaining

## Emergency Procedures

### If Stuck
1. Document the blocker in cleanup.md
2. Try alternative approach from phase plan
3. Create minimal reproduction case
4. Ask for clarification on requirements
5. Consider deferring to later phase

### If Breaking Changes Required
1. Document impact in regrets.md
2. Update all affected tests
3. Ensure backward compatibility if possible
4. Create migration guide if needed
5. Update all documentation

### If Time Exceeds Estimate
1. Document why in learnings
2. Identify what was underestimated
3. Break remaining work into smaller phases
4. Update estimates for similar future work
5. Complete current phase at logical stopping point

## Quality Gates

### Before Marking Phase Complete
- [ ] Code compiles without warnings
- [ ] Tests pass (or failures documented)
- [ ] Documentation is accurate
- [ ] No security vulnerabilities introduced
- [ ] Performance hasn't degraded
- [ ] Error messages are helpful
- [ ] Logging is appropriate
- [ ] Configuration is documented

### Ready for Next Phase When
- [ ] Current phase objectives met (or explicitly deferred)
- [ ] Next phase plan is specific and actionable
- [ ] Dependencies are identified
- [ ] Success criteria are measurable
- [ ] First task can be started immediately
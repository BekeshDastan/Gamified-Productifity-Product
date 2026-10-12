.PHONY: dev lint test check-changed build migrate seed seed-demo test-integration test-e2e backup restore-verify

dev lint test check-changed build migrate seed seed-demo test-integration test-e2e backup restore-verify:
	node scripts/tasks.mjs $@
